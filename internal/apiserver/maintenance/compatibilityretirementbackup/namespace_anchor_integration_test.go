//go:build integration

package compatibilityretirementbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This bounded fixture creates only its own small source namespaces and uses
// the existing registered nativeStartSourceRS helper for one independent
// destination process (bridge plus loopback, not network-none). It tests no
// production approval, privilege policy, writer fence or restore-scale budget.
// The root-owned source instances, accounts, topology and parameters are kept.
func nativeNamespaceArchive(t *testing.T, db *sql.DB, mdb *mongo.Database, dir, operation, run string, approvedAnchor *identitymeta.MongoNamespaceAnchor) *Archive {
	t.Helper()
	ctx := context.Background()
	ids, heads := nativeIdentities(t, db, mdb)
	defs, e := readSQLCatalog(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	cols, mdefs, e := mongoCatalog(ctx, mdb)
	if e != nil {
		t.Fatal(e)
	}
	subtype, mig, ok := cols["schema_migrations"].Lookup("info", "uuid").BinaryOK()
	if !ok || subtype != 4 {
		t.Fatal("native migration UUID")
	}
	var build bson.Raw
	if approvedAnchor == nil || approvedAnchor.Validate() != nil || approvedAnchor.Database != mdb.Name() {
		t.Fatal("native namespace approval invalid")
	}
	observed, e := identitymeta.ObserveMongoNamespaceAnchor(ctx, mdb, approvedAnchor.EndpointSHA256)
	if e != nil || !identitymeta.MatchMongoNamespaceAnchors(approvedAnchor, observed) {
		t.Fatal("native namespace approval mismatch")
	}
	if mdb.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build) != nil {
		t.Fatal("native metadata")
	}
	// Original inventory projection retains its target-owned constraint rows.
	// Recovery separately derives its exact target-exclusion schema baseline.
	originalNonTarget := map[string]any{}
	for key, value := range defs {
		if !strings.HasPrefix(key, "table:") || !targetSQLName(strings.TrimPrefix(key, "table:")) {
			originalNonTarget[key] = value
		}
	}
	bindings := map[string]Binding{"mysql": {IdentityHash: ids["mysql"], AnchorHash: ids["mysql"], Version: heads["mysql"], IdentityMatch: true, HeadMatch: true, CatalogHash: jsonSHA(defs), NonTargetHash: jsonSHA(originalNonTarget), MetadataComplete: true, ErrorCategory: "none"}, "mongodb": {IdentityHash: ids["mongodb"], AnchorHash: approvedAnchor.Hash, NamespaceAnchor: approvedAnchor.Clone(), GenerationHash: parts("mongodb_migration_generation_v1", hex.EncodeToString(mig)), Version: heads["mongodb"], IdentityMatch: true, HeadMatch: true, CatalogHash: jsonSHA(mdefs), NonTargetHash: targetMongoNonTarget(mdefs), MetadataComplete: true, ErrorCategory: "none"}}
	r := inventory{Format: 2, Kind: "readonly_compatibility_inventory", SourceSHA: strings.Repeat("a", 40), OperationID: operation, RunID: run, RequestHash: strings.Repeat("b", 64), TargetHash: jsonSHA([4][3]string{{"mysql", "domain_event_outbox", "base_table"}, {"mysql", "ai_bridge_commands", "base_table"}, {"mysql", "ai_messaging_legacy_commands", "base_table"}, {"mongodb", "domain_event_outbox", "collection"}}), Complete: true, Diagnostic: true, Bindings: bindings, Protocol: "mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2", ErrorCategory: "none", BoundaryHash: strings.Repeat("c", 64)}
	var sources [4][]byte
	for i := 0; i < 3; i++ {
		structure, e := sqlStructure(ctx, db, i)
		if e != nil {
			t.Fatal(e)
		}
		normalized := increment.ReplaceAllString(structure.DDL, "")
		name := targetNames[i]
		schema := jsonSHA([][]*string{{&name, &normalized}})
		identity := parts("mysql-object-v1", name, schema)
		projections := []string{}
		for _, c := range structure.Columns {
			projections = append(projections, "CAST("+quote(*c[0])+" AS BINARY)")
		}
		rows, e := readSQL(ctx, db, "SELECT "+strings.Join(projections, ",")+" FROM "+quote(name)+" ORDER BY "+quote(*structure.Columns[0][0]))
		if e != nil {
			t.Fatal(e)
		}
		h := sha256.New()
		frame(h, []byte(jsonSHA(structure.Columns)), false)
		var size uint64
		body := bytes.Buffer{}
		for _, row := range rows {
			enc := make([]*string, len(row))
			for j, v := range row {
				if v != nil {
					encoded := base64.StdEncoding.EncodeToString([]byte(*v))
					enc[j] = &encoded
					size += uint64(len(*v))
					frame(h, []byte(*v), false)
				} else {
					frame(h, nil, true)
				}
			}
			raw, e := json.Marshal(enc)
			if e != nil {
				t.Fatal("native row frame")
			}
			body.Write(raw)
			body.WriteByte('\n')
		}
		kind := "ascii_string"
		if i == 0 {
			kind = "uint64"
		}
		boundary := retirement.SourceBoundary{Database: "mysql", Name: name, Kind: "base_table", Present: true, Empty: len(rows) == 0, PKType: kind, SchemaHash: schema, IdentityHash: identity}
		if len(rows) > 0 {
			boundary.UpperToken = base64.StdEncoding.EncodeToString([]byte(*rows[len(rows)-1][0]))
		}
		header, e := json.Marshal(sqlHeader{retirement.SQLSourceProtocol, structure.Columns, boundary})
		if e != nil {
			t.Fatal("native source header")
		}
		sources[i] = append(append(header, '\n'), body.Bytes()...)
		r.Targets = append(r.Targets, SourceSnapshot{Database: "mysql", Name: name, Kind: "base_table", Present: true, Complete: true, Records: uint64(len(rows)), SchemaHash: schema, DataHash: hex.EncodeToString(h.Sum(nil)), IdentityHash: identity, Bytes: size, SourceFile: sourceNames[i], ErrorCategory: "none", Boundary: boundary, Passes: 2})
	}
	ordered, e := ReadOrderedMongoSchema(ctx, mdb)
	if e != nil {
		t.Fatal(e)
	}
	canonical, e := mongoTargetCanonical(ordered.data)
	if e != nil {
		t.Fatal(e)
	}
	typ, uid, ok := bson.Raw(ordered.data.Collection).Lookup("info", "uuid").BinaryOK()
	if !ok || typ != 4 {
		t.Fatal("native target UUID")
	}
	schema := jsonSHA(canonical)
	identity := parts("mongodb-object-v1", hex.EncodeToString(uid))
	boundary := retirement.SourceBoundary{Database: "mongodb", Name: targetNames[3], Kind: "collection", Present: true, Empty: true, SchemaHash: schema, IdentityHash: identity}
	cur, e := mdb.Collection(targetNames[3]).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}))
	if e != nil {
		t.Fatal("native BSON read")
	}
	h := sha256.New()
	var size, count uint64
	for cur.Next(ctx) {
		raw := append(bson.Raw(nil), cur.Current...)
		id := raw.Lookup("_id")
		boundary.PKType = pkKind(id)
		token, e := bson.Marshal(bson.D{{Key: "_id", Value: id}})
		if e != nil {
			t.Fatal("native token")
		}
		boundary.UpperToken = base64.StdEncoding.EncodeToString(token)
		boundary.Empty = false
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
		sources[3] = append(sources[3], length[:]...)
		sources[3] = append(sources[3], raw...)
		frame(h, raw, false)
		size += uint64(len(raw))
		count++
	}
	readErr, closeErr := cur.Err(), cur.Close(ctx)
	if readErr != nil || closeErr != nil {
		t.Fatal("native BSON close")
	}
	r.Targets = append(r.Targets, SourceSnapshot{Database: "mongodb", Name: targetNames[3], Kind: "collection", Present: true, Complete: true, Records: count, SchemaHash: schema, DataHash: hex.EncodeToString(h.Sum(nil)), IdentityHash: identity, Bytes: size, SourceFile: sourceNames[3], ErrorCategory: "none", Boundary: boundary, Passes: 2})
	// The synthetic local approval is sealed only after two actual complete
	// reads match every source's server-byte digest, including empty targets.
	// Capture independently performs its own full source-copy and DB checks.
	for pass := 0; pass < 2; pass++ {
		for i := 0; i < 3; i++ {
			structure, err := sqlStructure(ctx, db, i)
			if err != nil || verifySQLContent(ctx, db, structure, r.Targets[i], i) != nil {
				t.Fatal("native SQL inventory content mismatch")
			}
		}
		if verifyMongoContent(ctx, mdb, r.Targets[3]) != nil {
			t.Fatal("native Mongo inventory content mismatch")
		}
	}
	report, e := json.Marshal(r)
	if e != nil {
		t.Fatal("native report")
	}
	grants, e := readSQL(ctx, db, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil {
		t.Fatal(e)
	}
	sqlRaw, e := json.Marshal(sqlMetadata{Schema: defs, GrantsHash: jsonSHA(grants)})
	if e != nil {
		t.Fatal("native SQL metadata")
	}
	mongoRaw, e := json.Marshal(mongoMetadata{Schema: mdefs, Version: build.Lookup("version").StringValue()})
	if e != nil {
		t.Fatal("native Mongo metadata")
	}
	approval := Approval{InventorySHA256: sha(report), SQLMetadataSHA256: sha(sqlRaw), MongoMetadataSHA256: sha(mongoRaw), OrderedMongoSchemaSHA256: ordered.SHA256(), SourceSHA: r.SourceSHA, OperationID: r.OperationID, RunID: r.RunID, RequestHash: r.RequestHash}
	in := Inputs{Inventory: bytes.NewReader(report), SQLMetadata: bytes.NewReader(sqlRaw), MongoMetadata: bytes.NewReader(mongoRaw)}
	for i := range sources {
		in.Sources[i] = bytes.NewReader(sources[i])
	}
	out := filepath.Join(dir, "archive")
	if os.Mkdir(out, 0700) != nil {
		t.Fatal("native archive directory")
	}
	tx, e := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		t.Fatal("native RRRO")
	}
	a, e := Capture(ctx, BorrowedSources{SQL: tx, Mongo: mdb}, approval, in, out)
	ce := tx.Rollback()
	if e != nil || ce != nil {
		t.Fatal("native archive capture failed")
	}
	return a
}

func nativeNamespaceOriginalDigest(t *testing.T, db *sql.DB, mdb *mongo.Database) string {
	t.Helper()
	ctx := context.Background()
	defs, err := readSQLCatalog(ctx, db)
	if err != nil {
		t.Fatal("native original SQL catalog read failed")
	}
	_, mdefs, err := mongoCatalog(ctx, mdb)
	if err != nil {
		t.Fatal("native original Mongo catalog read failed")
	}
	h := sha256.New()
	frame(h, []byte(jsonSHA(defs)), false)
	frame(h, []byte(jsonSHA(mdefs)), false)
	// These are all SQL tables this local fixture creates, not an org-filtered
	// subset. CAST AS BINARY retains NULL, empty and actual JSON server bytes.
	for _, name := range []string{"ai_bridge_commands", "ai_bridge_requests", "ai_messaging_legacy_commands", "domain_event_outbox", "evidence_kept", "rm_outbox", "schema_migrations"} {
		columns, err := readSQL(ctx, db, "SELECT COLUMN_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ordinal_position", name)
		if err != nil || len(columns) == 0 {
			t.Fatal("native original SQL columns read failed")
		}
		projection := make([]string, 0, len(columns))
		for _, column := range columns {
			projection = append(projection, "CAST("+quote(cell(column, 0))+" AS BINARY)")
		}
		rows, err := readSQL(ctx, db, "SELECT "+strings.Join(projection, ",")+" FROM "+quote(name)+" ORDER BY "+quote(cell(columns[0], 0)))
		if err != nil {
			t.Fatal("native original SQL full row read failed")
		}
		frame(h, []byte(name), false)
		for _, row := range rows {
			for _, value := range row {
				if value == nil {
					frame(h, nil, true)
				} else {
					frame(h, []byte(*value), false)
				}
			}
		}
	}
	names, err := mdb.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		t.Fatal("native original Mongo names read failed")
	}
	sort.Strings(names)
	for _, name := range names {
		frame(h, []byte(name), false)
		cur, err := mdb.Collection(name).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}))
		if err != nil {
			t.Fatal("native original BSON read failed")
		}
		for cur.Next(ctx) {
			frame(h, cur.Current, false)
		}
		readErr, closeErr := cur.Err(), cur.Close(ctx)
		if readErr != nil || closeErr != nil {
			t.Fatal("native original BSON read incomplete")
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestBackupNamespaceNativeCaptureAndOriginalBoundRestore(t *testing.T) {
	if os.Getenv("QS_BACKUP_NAMESPACE_NATIVE") != "1" {
		t.Skip("owned namespace-anchor native fixture not requested")
	}
	nativeRoot(t, "owned-mysql.json", "34306")
	mongoRoot := nativeRoot(t, "owned-mongo.json", "33317")
	env := nativeEnv(t)
	dir := nativePrivateDir(t)
	admin := nativeSQL(t, env, "")
	name := "qs_backup_namespace_" + primitive.NewObjectID().Hex()
	nativeExec(t, admin, "CREATE DATABASE "+quote(name)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	db := nativeSQL(t, env, name)
	originalClient := nativeMongo(t, env)
	original := originalClient.Database(name)
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+quote(name)); err != nil {
			nativeRetainCleanup(t, dir)
			t.Error("owned namespace source SQL cleanup failed")
		}
		if original.Drop(context.Background()) != nil {
			nativeRetainCleanup(t, dir)
			t.Error("owned namespace source Mongo cleanup failed")
		}
		rows, err := readSQL(context.Background(), admin, "SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME=?", name)
		names, mongoErr := originalClient.ListDatabaseNames(context.Background(), bson.D{{Key: "name", Value: name}})
		if err != nil || len(rows) != 0 || mongoErr != nil || len(names) != 0 {
			nativeRetainCleanup(t, dir)
			t.Error("owned namespace source remaining")
		} else {
			t.Log("owned_source_sql_namespace_remaining=0 owned_source_mongo_namespace_remaining=0")
		}
	})
	nativeSourceSchemas(t, db, original)
	nativeTargetInsertEvents(t, db)
	if _, err := original.Collection("domain_event_outbox").InsertOne(context.Background(), bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "event_id", Value: "namespace-local-original"}, {Key: "status", Value: "published"}, {Key: "binary", Value: primitive.Binary{Subtype: 0, Data: []byte{0, 255}}}, {Key: "time", Value: time.UnixMilli(1728000123123)}}); err != nil {
		t.Fatal("native original event setup failed")
	}
	if _, err := original.Collection("answersheets").InsertOne(context.Background(), bson.D{{Key: "_id", Value: "kept"}, {Key: "native_fixture_only", Value: true}}); err != nil {
		t.Fatal("native kept collection setup failed")
	}
	port, err := strconv.Atoi(env["MONGODB_PORT"])
	if err != nil {
		t.Fatal("native approved endpoint unavailable")
	}
	endpoint, err := identitymeta.MongoEndpointSHA256(env["MONGODB_HOST"], port, name)
	if err != nil {
		t.Fatal("native approved endpoint invalid")
	}
	approved, err := identitymeta.ObserveMongoNamespaceAnchor(context.Background(), original, endpoint)
	if err != nil || approved.Validate() != nil || approved.Kind != identitymeta.MongoNamespaceAnchorKind {
		t.Fatal("native approved kept UUID observation failed")
	}
	// The local approval comes from real metadata before Capture. Nothing in
	// this test rewrites a UUID, turns a partial catalog into complete, or
	// promotes the synthetic local approval into production execution rights.
	before := nativeNamespaceOriginalDigest(t, db, original)
	a := nativeNamespaceArchive(t, db, original, dir, "891-1", "892-1", approved.Clone())
	if !identitymeta.MatchMongoNamespaceAnchors(approved, a.data.Inventory.Bindings["mongodb"].NamespaceAnchor) || a.data.Inventory.Bindings["mongodb"].AnchorHash != approved.Hash {
		t.Fatal("native captured approved anchor changed")
	}
	if nativeNamespaceOriginalDigest(t, db, original) != before {
		t.Fatal("native Capture changed original source")
	}
	isolatedEnv := make(map[string]string, len(env))
	for key, value := range env {
		isolatedEnv[key] = value
	}
	owned := nativeStartSourceRS(t, dir, mongoRoot, isolatedEnv)
	isolatedClient := nativeMongo(t, isolatedEnv)
	restoredName := "qs_backup_namespace_restore_" + primitive.NewObjectID().Hex()
	refusedName := "qs_backup_namespace_refused_" + primitive.NewObjectID().Hex()
	isolated := isolatedClient.Database(restoredName)
	refused := isolatedClient.Database(refusedName)
	t.Cleanup(func() {
		for _, target := range []*mongo.Database{isolated, refused} {
			if target.Drop(context.Background()) != nil {
				nativeRetainCleanup(t, dir)
				t.Error("owned namespace destination cleanup failed")
			}
			names, err := isolatedClient.ListDatabaseNames(context.Background(), bson.D{{Key: "name", Value: target.Name()}})
			if err != nil || len(names) != 0 {
				nativeRetainCleanup(t, dir)
				t.Error("owned namespace destination remaining")
			}
		}
	})
	proof, err := RestoreMongoWithOriginal(context.Background(), original, isolated, a)
	if err != nil || proof == nil {
		t.Fatal("native original-bound restore failed")
	}
	v := proof.Summary()
	if !v.ContentEqual || !v.SchemaEqual || v.DropReady || v.ProductionBoundRestoreBudgetProven || v.Database != "mongodb" || v.ArchiveSHA256 != a.digest || v.TargetCount != 1 || v.Isolation != "host_runtime_inspection_required" || len(v.Targets) != 1 || v.Targets[0].SourceRecords != 1 || v.Targets[0].RestoredRecords != 1 {
		t.Fatal("native original-bound restore observation invalid")
	}
	actualAfter, err := identitymeta.ObserveMongoNamespaceAnchor(context.Background(), original, endpoint)
	if err != nil || !identitymeta.MatchMongoNamespaceAnchors(approved, actualAfter) || nativeNamespaceOriginalDigest(t, db, original) != before || owned.check(context.Background(), true) != nil {
		t.Fatal("native original UUID/content or destination ownership changed")
	}
	// Replace only this test's own kept collection. An equal name/document
	// cannot stand in for the previously approved native collection UUID.
	if original.Collection("answersheets").Drop(context.Background()) != nil {
		t.Fatal("native UUID replacement failed")
	}
	if _, err := original.Collection("answersheets").InsertOne(context.Background(), bson.D{{Key: "_id", Value: "kept"}, {Key: "native_fixture_only", Value: true}}); err != nil {
		t.Fatal("native UUID replacement recreate failed")
	}
	drifted, err := identitymeta.ObserveMongoNamespaceAnchor(context.Background(), original, endpoint)
	if err != nil || identitymeta.MatchMongoNamespaceAnchors(approved, drifted) || drifted.Collections[0].UUID == approved.Collections[0].UUID {
		t.Fatal("native actual UUID drift not demonstrated")
	}
	if _, err := RestoreMongoWithOriginal(context.Background(), original, refused, a); !errors.Is(err, ErrIdentity) {
		t.Fatal("native approved original UUID drift not refused")
	}
	names, err := refused.ListCollectionNames(context.Background(), bson.D{})
	if err != nil || len(names) != 0 {
		t.Fatal("native refused restore changed destination")
	}
	// The borrowed clients remain usable; this helper neither closes them nor
	// starts/commits/ends a host session. Cleanup uses only registered own DBs.
	if originalClient.Ping(context.Background(), nil) != nil || isolatedClient.Ping(context.Background(), nil) != nil || db.PingContext(context.Background()) != nil {
		t.Fatal("native borrowed source lifecycle changed")
	}
}
