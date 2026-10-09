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
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This local fixture synthesizes approval policy only. Every schema/content/
// instance/head and actual DROP/result proof below comes from native reads.
// It never represents a production approval or Linux root-window budget proof.
func nativeTargetArchive(t *testing.T, db *sql.DB, mdb *mongo.Database, dir, operation, run string) *Archive {
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
	var config, hello, build bson.Raw
	if mdb.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&config) != nil || mdb.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil || mdb.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build) != nil {
		t.Fatal("native metadata")
	}
	set, ok := hello.Lookup("setName").StringValueOK()
	rid, rok := config.Lookup("config", "settings", "replicaSetId").ObjectIDOK()
	if !ok || !rok {
		t.Fatal("native anchor")
	}
	// Original inventory projection retains its target-owned constraint rows.
	// Recovery separately derives its exact target-exclusion schema baseline.
	originalNonTarget := map[string]any{}
	for key, value := range defs {
		if !strings.HasPrefix(key, "table:") || !targetSQLName(strings.TrimPrefix(key, "table:")) {
			originalNonTarget[key] = value
		}
	}
	bindings := map[string]Binding{"mysql": {IdentityHash: ids["mysql"], AnchorHash: ids["mysql"], Version: heads["mysql"], IdentityMatch: true, HeadMatch: true, CatalogHash: jsonSHA(defs), NonTargetHash: jsonSHA(originalNonTarget), MetadataComplete: true, ErrorCategory: "none"}, "mongodb": {IdentityHash: ids["mongodb"], AnchorHash: parts("mongodb_database_anchor_v1", rid.Hex(), set, mdb.Name()), GenerationHash: parts("mongodb_migration_generation_v1", hex.EncodeToString(mig)), Version: heads["mongodb"], IdentityMatch: true, HeadMatch: true, CatalogHash: jsonSHA(mdefs), NonTargetHash: targetMongoNonTarget(mdefs), MetadataComplete: true, ErrorCategory: "none"}}
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
	if cur.Err() != nil || cur.Close(ctx) != nil {
		t.Fatal("native BSON close")
	}
	r.Targets = append(r.Targets, SourceSnapshot{Database: "mongodb", Name: targetNames[3], Kind: "collection", Present: true, Complete: true, Records: count, SchemaHash: schema, DataHash: hex.EncodeToString(h.Sum(nil)), IdentityHash: identity, Bytes: size, SourceFile: sourceNames[3], ErrorCategory: "none", Boundary: boundary, Passes: 2})
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
func TestTargetRecoveryNative(t *testing.T) {
	if os.Getenv("QS_TARGET_RECOVERY_NATIVE") != "1" {
		t.Skip("owned target recovery native fixture not requested")
	}
	nativeRoot(t, "owned-mysql.json", "34306")
	nativeRoot(t, "owned-mongo.json", "33317")
	env := nativeEnv(t)
	dir := nativePrivateDir(t)
	admin := nativeSQL(t, env, "")
	name := "qs_target_recovery_" + primitive.NewObjectID().Hex()
	nativeExec(t, admin, "CREATE DATABASE "+quote(name)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	db := nativeSQL(t, env, name)
	client := nativeMongo(t, env)
	mdb := client.Database(name)
	t.Cleanup(func() {
		if _, e := admin.ExecContext(context.Background(), "DROP DATABASE "+quote(name)); e != nil {
			nativeRetainCleanup(t, dir)
			t.Error("owned target recovery SQL cleanup failed")
		}
		if mdb.Drop(context.Background()) != nil {
			nativeRetainCleanup(t, dir)
			t.Error("owned target recovery Mongo cleanup failed")
		}
		rows, e := readSQL(context.Background(), admin, "SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME=?", name)
		if e != nil || len(rows) != 0 {
			nativeRetainCleanup(t, dir)
			t.Error("owned SQL namespace remaining")
		}
		names, e := client.ListDatabaseNames(context.Background(), bson.D{{Key: "name", Value: name}})
		if e != nil || len(names) != 0 {
			nativeRetainCleanup(t, dir)
			t.Error("owned Mongo namespace remaining")
		}
	})
	nativeSourceSchemas(t, db, mdb)
	nativeTargetInstallOperations(t, db)
	nativeTargetInsertEvents(t, db)
	id := "11111111-1111-4111-8111-111111111111"
	nativeExec(t, db, "INSERT INTO ai_bridge_requests(request_id,request_hash,payload) VALUES(?,?,?)", id, strings.Repeat("a", 64), `{"owner":"owned local"}`)
	nativeExec(t, db, "INSERT INTO ai_bridge_commands VALUES(?,?,?,?,?,?,?,?)", id, id, "start", `{"private":"local fixture"}`, strings.Repeat("b", 64), true, 3, "2026-10-08 11:12:13.123456")
	nativeExec(t, db, "INSERT INTO ai_messaging_legacy_commands VALUES(?,?,?,?,?,?,?,?,?,?)", id, id, "start", []byte{0, 255}, sha([]byte{0, 255}), 3, "2026-10-08 11:12:13.123456", "2026-10-08T11:12:13.123456Z", strings.Repeat("c", 64), "2026-10-08 11:12:14.123456")
	if _, e := mdb.Collection("domain_event_outbox").InsertOne(context.Background(), bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "event_id", Value: id}, {Key: "status", Value: "published"}, {Key: "null", Value: nil}, {Key: "time", Value: time.UnixMilli(1728000123123)}, {Key: "binary", Value: primitive.Binary{Subtype: 0, Data: []byte{0, 255}}}}); e != nil {
		t.Fatal("native target BSON setup")
	}
	a := nativeTargetArchive(t, db, mdb, dir, "123-1", "789-1")
	conn, e := db.Conn(context.Background())
	if e != nil {
		t.Fatal("borrowed conn")
	}
	defer func() {
		if conn.Close() != nil {
			t.Error("host connection release")
		}
	}()
	defs, e := readSQLCatalog(context.Background(), conn)
	if e != nil {
		t.Fatal(e)
	}
	non, e := targetSQLNonTarget(defs, name)
	if e != nil {
		t.Fatal(e)
	}
	_, mdefs, e := mongoCatalog(context.Background(), mdb)
	if e != nil {
		t.Fatal(e)
	}
	r := TargetRecoveryRequest{SourceSHA: a.data.Approval.SourceSHA, OperationID: a.data.Approval.OperationID, OriginalRunID: a.data.Approval.RunID, ActualRunID: "990-1", ManifestSHA256: strings.Repeat("d", 64), ArchiveSHA256: a.digest, SQLNonTargetSHA256: non, MongoNonTargetSHA256: targetMongoNonTarget(mdefs), SQLHead: 99, MongoHead: 38}
	journal := filepath.Join(dir, "recovery")
	if os.Mkdir(journal, 0700) != nil {
		t.Fatal("owned journal")
	}
	p, e := PrepareTargetRecovery(context.Background(), a, TargetRecoveryBorrowed{conn, mdb}, r, journal, nil)
	if e != nil {
		t.Fatal(e)
	}
	// A single fixed private test deadline, never each call's now+600. This is not
	// evidence for the Linux public window, whole writer fence, or production.
	end := time.Now().Add(time.Minute)
	p.budget = func(ctx context.Context) (context.Context, context.CancelFunc, error) {
		q, c := context.WithDeadline(ctx, end)
		return q, c, nil
	}
	originalSQL := nativeTargetOriginalPoints(t, conn)
	originalProtected := nativeTargetProtectedPoints(t, conn, mdb)
	for _, i := range []int{0, 3} {
		if e = executeTargetDrop(context.Background(), p, i); e != nil {
			t.Fatal(e)
		}
	}
	v, e := RecoverTargets(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	s := v.Summary()
	if s.NativeDropProofs != 2 || s.ProductionAuthorityIntegrated || s.WholeWriterFenceProven || s.DropReady {
		t.Fatal("native receipt overclaimed")
	}
	if _, e = RecoverTargets(context.Background(), p); e != nil {
		t.Fatal("exact repeat readback failed")
	}

	if !reflect.DeepEqual(originalSQL, nativeTargetOriginalPoints(t, conn)) || !reflect.DeepEqual(originalProtected, nativeTargetProtectedPoints(t, conn, mdb)) {
		t.Fatal("mixed restore changed actual target or protected bytes")
	}
	before := nativeTargetProtectedPoints(t, conn, mdb)
	// First round is mixed: SQL DCE+Mongo are restored; AI SQL targets were only
	// read. All preserved facts are compared before the complete second round.
	firstSQL := nativeTargetOriginalPoints(t, conn)
	if e = conn.PingContext(context.Background()); e != nil {
		t.Fatal("borrowed conn closed")
	}
	second := filepath.Join(dir, "second")
	if os.Mkdir(second, 0700) != nil {
		t.Fatal("second owned archive directory")
	}
	a2 := nativeTargetArchive(t, db, mdb, second, "124-1", "790-1")
	if a2.data.Inventory.Targets[3].IdentityHash == a.data.Inventory.Targets[3].IdentityHash {
		t.Fatal("restored Mongo UUID was falsely reused")
	}
	r2 := r
	r2.OperationID = a2.data.Approval.OperationID
	r2.OriginalRunID = a2.data.Approval.RunID
	r2.ActualRunID = "991-1"
	r2.ManifestSHA256 = strings.Repeat("e", 64)
	r2.ArchiveSHA256 = a2.digest
	j2 := filepath.Join(second, "recovery")
	if os.Mkdir(j2, 0700) != nil {
		t.Fatal("second owned journal")
	}
	p2, e := PrepareTargetRecovery(context.Background(), a2, TargetRecoveryBorrowed{conn, mdb}, r2, j2, nil)
	if e != nil {
		t.Fatal(e)
	}
	p2.budget = func(ctx context.Context) (context.Context, context.CancelFunc, error) {
		q, c := context.WithDeadline(ctx, end)
		return q, c, nil
	}
	for i := 0; i < 4; i++ {
		if e = executeTargetDrop(context.Background(), p2, i); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 4; i++ {
		present, e := p2.checkTarget(context.Background(), i)
		if e != nil || present {
			t.Fatal("four actual targets not all absent")
		}
	}
	v2, e := RecoverTargets(context.Background(), p2)
	if e != nil {
		t.Fatal(e)
	}
	if v2.Summary().NativeDropProofs != 4 {
		t.Fatal("four native DROP proofs missing")
	}
	if _, e = RecoverTargets(context.Background(), p2); e != nil {
		t.Fatal("restored exact repeat readback")
	}
	if !reflect.DeepEqual(firstSQL, nativeTargetOriginalPoints(t, conn)) {
		t.Fatal("SQL original bytes NULL attempts or times changed")
	}
	if !reflect.DeepEqual(before, nativeTargetProtectedPoints(t, conn, mdb)) {
		t.Fatal("fixture protected MQ evidence retired PK or head changed")
	}
	// Root-owned proof is one-shot. A fresh third archive binds the second newly
	// restored target UUID; old source identities are never adopted by JSON.
	third := filepath.Join(dir, "third")
	if os.Mkdir(third, 0700) != nil {
		t.Fatal("third owned archive directory")
	}
	a3 := nativeTargetArchive(t, db, mdb, third, "125-1", "791-1")
	if a3.data.Inventory.Targets[3].IdentityHash == a2.data.Inventory.Targets[3].IdentityHash {
		t.Fatal("new actual UUID not observed")
	}
	r3 := r2
	r3.OperationID = a3.data.Approval.OperationID
	r3.OriginalRunID = a3.data.Approval.RunID
	r3.ActualRunID = "992-1"
	r3.ManifestSHA256 = strings.Repeat("f", 64)
	r3.ArchiveSHA256 = a3.digest
	j3 := filepath.Join(third, "recovery")
	if os.Mkdir(j3, 0700) != nil {
		t.Fatal("third journal")
	}
	p3, e := PrepareTargetRecovery(context.Background(), a3, TargetRecoveryBorrowed{conn, mdb}, r3, j3, nil)
	if e != nil {
		t.Fatal(e)
	}
	p3.budget = func(ctx context.Context) (context.Context, context.CancelFunc, error) {
		q, c := context.WithDeadline(ctx, end)
		return q, c, nil
	}
	if e = executeTargetDrop(context.Background(), p3, 1); e != nil {
		t.Fatal(e)
	}
	nativeExec(t, db, "DELETE FROM ai_bridge_requests WHERE request_id=?", id)
	if _, e = RecoverTargets(context.Background(), p3); e != ErrRecoveryUnknown {
		t.Fatal("missing FK parent did not block actual load")
	}
	if _, e = RecoverTargets(context.Background(), p3); e == nil {
		t.Fatal("partial retry accepted")
	}
	if !reflect.DeepEqual(before, nativeTargetProtectedPoints(t, conn, mdb)) {
		t.Fatal("failed restore touched protected fixture facts")
	}
	if conn.PingContext(context.Background()) != nil {
		t.Fatal("borrowed connection lost")
	}

}

// Actual published migration resources supply the protected operations shape.
// Its fixture marker is only a preserved local fact, not closure authorization.
func nativeTargetInstallOperations(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, name := range []string{"000091_ai_messaging.up.sql", "000097_ai_command_retirement.up.sql"} {
		raw, e := os.ReadFile(filepath.Join("../../../..", "internal/pkg/migration/migrations/mysql", name))
		if e != nil {
			t.Fatal("actual operations migration missing")
		}
		lines := []string{}
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				lines = append(lines, line)
			}
		}
		for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
			if strings.TrimSpace(statement) != "" {
				nativeExec(t, db, statement)
			}
		}
	}
	marker := `{"version":1,"ownership_verified":true,"responsibility_closed":true,"business_terminal":true,"fixture_only":true}`
	nativeExec(t, db, "INSERT INTO ai_messaging_operations(command_id,kind,body_sha256,organization_id,subject_id,resource_id,aggregate_key,aggregate_sequence,created_at,retired,retirement_evidence,retired_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", "33333333-3333-4333-8333-333333333333", 1, nil, uint64(7), "owned-subject", "44444444-4444-4444-8444-444444444444", "owned-request", nil, nil, true, marker, "2026-10-08 11:12:13.123456")
}
func nativeTargetInsertEvents(t *testing.T, db *sql.DB) {
	t.Helper()
	columns := "(id,event_id,event_type,aggregate_type,aggregate_id,org_id,topic_name,payload_json,status,attempt_count,retry_disposition,next_attempt_at,last_error,last_error_kind,manual_replay_request_id,created_at,updated_at,published_at)"
	statement := "INSERT INTO domain_event_outbox " + columns + " VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
	nativeExec(t, db, statement, uint64(639678084915671598), "owned-event-high", "evaluation.outcome.committed", "Evaluation", "639678084915671598", nil, "qs-evaluation", `{"number":639678084915671598,"text":"original  bytes"}`, "published", uint64(3), nil, "2026-10-08 11:12:13.123", nil, nil, nil, "2026-10-08 11:12:13.123", "2026-10-08 11:12:13.123", nil)
	nativeExec(t, db, statement, uint64(639678084915671599), "owned-event-next", "evaluation.requested", "Evaluation", "639678084915671598", int64(7), "qs-evaluation", `{"text":"second"}`, "published", uint64(7), "terminal", "2026-10-08 11:12:14.321", "", "fixture", nil, "2026-10-08 11:12:14.321", "2026-10-08 11:12:14.321", "2026-10-08 11:12:14.321")
}
func nativeTargetOriginalPoints(t *testing.T, conn *sql.Conn) map[string][][]*string {
	t.Helper()
	queries := map[string]string{
		"dce":    "SELECT CAST(id AS CHAR),HEX(CAST(payload_json AS BINARY)),CAST(org_id AS CHAR),CAST(attempt_count AS CHAR),CAST(next_attempt_at AS CHAR),CAST(published_at AS CHAR),last_error,retry_disposition FROM domain_event_outbox ORDER BY id",
		"bridge": "SELECT command_id,HEX(CAST(payload AS BINARY)),CAST(delivered AS CHAR),CAST(attempts AS CHAR),CAST(available_at AS CHAR) FROM ai_bridge_commands ORDER BY command_id",
		"legacy": "SELECT command_id,HEX(source_payload),source_payload_hash,CAST(source_attempts AS CHAR),CAST(source_available_at AS CHAR),source_original_time,messaging_body_sha256,CAST(transferred_at AS CHAR) FROM ai_messaging_legacy_commands ORDER BY command_id",
	}
	out := map[string][][]*string{}
	for name, query := range queries {
		rows, e := readSQL(context.Background(), conn, query)
		if e != nil {
			t.Fatal("actual target point read failed")
		}
		out[name] = rows
	}
	return out
}
func nativeTargetProtectedPoints(t *testing.T, conn *sql.Conn, mdb *mongo.Database) map[string]string {
	t.Helper()
	queries := map[string]string{
		"mq":       "SELECT CAST(id AS CHAR),HEX(protected_fact) FROM rm_outbox ORDER BY id",
		"evidence": "SELECT CAST(id AS CHAR),HEX(CAST(proof AS BINARY)) FROM evidence_kept ORDER BY id",
		"retired":  "SELECT command_id,HEX(CAST(retirement_evidence AS BINARY)),CAST(retired AS CHAR),body_sha256,CAST(aggregate_sequence AS CHAR),CAST(created_at AS CHAR),CAST(retired_at AS CHAR) FROM ai_messaging_operations ORDER BY command_id",
		"head":     "SELECT CAST(version AS CHAR),CAST(dirty AS CHAR) FROM schema_migrations",
	}
	out := map[string]string{}
	for name, query := range queries {
		rows, e := readSQL(context.Background(), conn, query)
		if e != nil {
			t.Fatal("actual protected point read failed")
		}
		out[name] = jsonSHA(rows)
	}
	var raw bson.Raw
	if mdb.Collection("rm_outbox").FindOne(context.Background(), bson.D{{Key: "_id", Value: "protected"}}).Decode(&raw) != nil {
		t.Fatal("actual Mongo MQ point read failed")
	}
	out["mongo_mq"] = sha(raw)
	if mdb.Collection("schema_migrations").FindOne(context.Background(), bson.D{}).Decode(&raw) != nil {
		t.Fatal("actual Mongo head point read failed")
	}
	out["mongo_head"] = sha(raw)
	for _, name := range []string{"answersheets", "interpret_report_artifacts", "interpretation_runs", "report_generations"} {
		ctx := context.Background()
		cur, e := mdb.Collection(name).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if e != nil {
			t.Fatal("actual retained Mongo facts read failed")
		}
		var rows []bson.Raw
		readErr := cur.All(ctx, &rows)
		closeErr := cur.Close(ctx)
		if readErr != nil || closeErr != nil {
			t.Fatal("actual retained Mongo facts cursor failed")
		}
		hashes := make([]string, len(rows))
		for i, row := range rows {
			hashes[i] = sha(row)
		}
		out["mongo_kept/"+name] = jsonSHA(hashes)
	}
	return out
}
