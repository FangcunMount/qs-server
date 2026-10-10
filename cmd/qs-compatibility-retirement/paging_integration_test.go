//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Compare the actual original BSON stream, fixed upper, EOF and both-pass hash
// at the two fixed page sizes. Timings describe only this owned local fixture.
func TestOwnedMongoPageSizePreservesSourceAndBothEOF(t *testing.T) {
	localInventoryGuard(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, e := mongoOpen(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := client.Disconnect(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	name := "qs_retirement_inventory_test_page_size_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	db := client.Database(name)
	defer func() {
		if e := db.Drop(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	col := db.Collection("domain_event_outbox")
	const rows = 10000
	for start := 1; start <= rows; start += 1000 {
		docs := make([]any, 0, 1000)
		for n := start; n < start+1000 && n <= rows; n++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int64(n)}, {Key: "payload", Value: bytes.Repeat([]byte{byte(n)}, 1536)}, {Key: "status", Value: "published"}})
		}
		if _, e := col.InsertMany(ctx, docs); e != nil {
			t.Fatal(e)
		}
	}
	token, kind, empty, e := mongoUpper(ctx, col, productionLimits())
	if e != nil || empty || kind != "long" {
		t.Fatal("native fixed upper was not proved", e)
	}
	bound := targetBoundary{Database: "mongodb", Present: true, PKType: kind, UpperToken: token}
	if _, e := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int64(rows + 1)}, {Key: "payload", Value: "OUTSIDE_APPROVED_UPPER"}}); e != nil {
		t.Fatal(e)
	}
	var original snapshot
	var originalBytes []byte
	for _, pageSize := range []int{1000, 10000} {
		limits := productionLimits()
		limits.PageSize = pageSize
		dir := privateTestDir(t)
		file, e := os.OpenFile(filepath.Join(dir, "source.bsonframes"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if e != nil {
			t.Fatal(e)
		}
		started := time.Now()
		first, e := mongoPagedPass(ctx, col, bound, limits, dir, 1, file)
		if e != nil {
			_ = file.Close()
			t.Fatal(e)
		}
		if e = file.Sync(); e != nil {
			_ = file.Close()
			t.Fatal(e)
		}
		if e = file.Close(); e != nil {
			t.Fatal(e)
		}
		second, e := mongoPagedPass(ctx, col, bound, limits, dir, 2, nil)
		elapsed := time.Since(started)
		if e != nil || first.Records != rows || second.Records != rows || first.Pages != uint64(rows/pageSize+1) || second.Pages != first.Pages || first.Bytes != second.Bytes || first.DataHash != second.DataHash {
			t.Fatal("actual fixed-upper stream or complete EOF changed", e)
		}
		raw, e := os.ReadFile(file.Name())
		if e != nil {
			t.Fatal(e)
		}
		if pageSize == 1000 {
			original, originalBytes = first, raw
		} else if first.Records != original.Records || first.Bytes != original.Bytes || first.DataHash != original.DataHash || !bytes.Equal(raw, originalBytes) {
			t.Fatal("page size changed original source bytes or hash")
		}
		t.Logf("owned_fixture_page_size=%d records=%d raw_bytes=%d total_pages=%d two_pass_elapsed_ms=%d", pageSize, first.Records, first.Bytes, first.Pages+second.Pages, elapsed.Milliseconds())
	}
}

func localInventoryGuard(t *testing.T) {
	t.Helper()
	if os.Getenv("QS_RETIREMENT_LOCAL_INTEGRATION") != "1" {
		t.Skip("owned local databases not requested")
	}
	if os.Getenv("MYSQL_HOST") != "127.0.0.1" || os.Getenv("MONGODB_HOST") != "127.0.0.1" || os.Getenv("MYSQL_DATABASE") != "qs_retirement_inventory_test" || os.Getenv("MONGODB_DBNAME") != "qs_retirement_inventory_test" {
		t.Fatal("non-isolated target rejected")
	}
}
func privateTestDir(t *testing.T) string {
	t.Helper()
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	return dir
}
func TestOwnedProductionScaleTwoPassInventoryAndFixedUpper(t *testing.T) {
	localInventoryGuard(t)
	sourceSHA = strings.Repeat("a", 40)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	admin, e := mysqlOpen(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := admin.Close(); e != nil {
			t.Error(e)
		}
	}()
	name := "qs_retirement_inventory_test_paged_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, e = admin.ExecContext(ctx, "CREATE DATABASE "+quote(name)); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e = admin.ExecContext(context.Background(), "DROP DATABASE "+quote(name)); e != nil {
			t.Error(e)
		}
	}()
	t.Setenv("MYSQL_DATABASE", name)
	t.Setenv("MONGODB_DBNAME", name)
	db, e := mysqlOpen(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := db.Close(); e != nil {
			t.Error(e)
		}
	}()
	client, e := mongoOpen(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := client.Disconnect(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	mdb := client.Database(name)
	defer func() {
		if e = mdb.Drop(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	for _, statement := range []string{"CREATE TABLE schema_migrations(version BIGINT NOT NULL,dirty BOOL NOT NULL)", "INSERT INTO schema_migrations VALUES(98,FALSE)", "CREATE TABLE kept_fact(id BIGINT PRIMARY KEY,note TEXT)", "CREATE TABLE domain_event_outbox(id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,event_type VARCHAR(128),status VARCHAR(32),payload_json LONGTEXT)", "CREATE TABLE ai_bridge_commands(command_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,kind VARCHAR(32),delivered BOOL,payload JSON)", "CREATE TABLE ai_messaging_legacy_commands(command_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,source_kind VARCHAR(32),source_payload MEDIUMBLOB)"} {
		if _, e = db.ExecContext(ctx, statement); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = mdb.Collection("schema_migrations").InsertOne(ctx, bson.D{{Key: "version", Value: int64(37)}, {Key: "dirty", Value: false}}); e != nil {
		t.Fatal(e)
	}
	rows := 2*productionLimits().PageSize + 5
	if raw := os.Getenv("QS_RETIREMENT_SCALE_ROWS"); raw != "" {
		rows, e = strconv.Atoi(raw)
		if e != nil || rows < 2*productionLimits().PageSize+5 || rows > 700000 {
			t.Fatal("test size rejected")
		}
	}
	col := mdb.Collection("domain_event_outbox")
	for start := 1; start <= rows; start += 1000 {
		var placeholders []string
		var args []any
		var mongoRows []any
		for n := start; n < start+1000 && n <= rows; n++ {
			placeholders = append(placeholders, "(?,?,?,?)")
			args = append(args, n, "evaluation.outcome.committed", "published", `{"sentinel":"PRIVATE_BULK_BODY"}`)
			mongoRows = append(mongoRows, bson.D{{Key: "_id", Value: int64(n)}, {Key: "status", Value: "published"}, {Key: "event_type", Value: "answersheet.submitted"}, {Key: "payload", Value: bson.D{{Key: "sentinel", Value: "PRIVATE_BULK_BODY"}}}})
		}
		if _, e = db.ExecContext(ctx, "INSERT INTO domain_event_outbox VALUES "+strings.Join(placeholders, ","), args...); e != nil {
			t.Fatal(e)
		}
		if _, e = col.InsertMany(ctx, mongoRows); e != nil {
			t.Fatal(e)
		}
	}
	// A UUID-key table spans page boundaries; the second UUID table remains
	// present-empty and must not collapse to absent.
	uuidRows := productionLimits().PageSize + 5
	for start := 0; start < uuidRows; start += 500 {
		var args []any
		var placeholders []string
		for n := start; n < start+500 && n < uuidRows; n++ {
			placeholders = append(placeholders, "(?,?,?,?)")
			args = append(args, fmt.Sprintf("%036d", n), "request", true, `{"sentinel":"PRIVATE_COMMAND_BODY"}`)
		}
		if _, e = db.ExecContext(ctx, "INSERT INTO ai_bridge_commands VALUES "+strings.Join(placeholders, ","), args...); e != nil {
			t.Fatal(e)
		}
	}
	var uuid, selected string
	if e = db.QueryRowContext(ctx, "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &selected); e != nil {
		t.Fatal(e)
	}
	collections, _, e := mongoSchemas(ctx, mdb)
	if e != nil {
		t.Fatal(e)
	}
	_, uuidBytes := collections["schema_migrations"].Lookup("info", "uuid").Binary()
	stable, _ := json.Marshal(bson.D{})
	r := v2Request("readonly_inventory_boundary_request")
	r.Identities = map[string]string{"mysql": hashParts("mysql_database_identity_v1", uuid, selected), "mongodb": hashParts("mongodb_database_identity_v1", string(stable), name, hex.EncodeToString(uuidBytes))}
	dir := privateTestDir(t)
	boundsDir := filepath.Join(dir, "bounds-456-1")
	if e = os.Mkdir(boundsDir, 0700); e != nil {
		t.Fatal(e)
	}
	requestPath := filepath.Join(dir, "boundary-request.json")
	raw, _ := json.Marshal(r)
	if e = os.WriteFile(requestPath, raw, 0600); e != nil {
		t.Fatal(e)
	}
	discovered, e := run(requestPath, digestRaw(raw), "123-1", "456-1", boundsDir)
	if e != nil || !discovered.Complete || discovered.DropReady || !discovered.DiagnosticOnly {
		t.Fatalf("bounds failed: %v", e)
	}
	if !discovered.Targets[2].Boundary.Present || !discovered.Targets[2].Boundary.Empty || discovered.Targets[2].SourceFile != "" {
		t.Fatal("present empty collapsed or body copied")
	}
	boundaryRaw, e := os.ReadFile(filepath.Join(boundsDir, "boundary.private.json"))
	if e != nil {
		t.Fatal(e)
	}
	r.Kind = "readonly_inventory_request"
	r.BoundaryRunID = "456-1"
	r.BoundaryReportHash = digestRaw(boundaryRaw)
	for _, s := range discovered.Targets {
		r.Boundaries = append(r.Boundaries, *s.Boundary)
	}
	// New rows after explicit upper approval are next-cycle work, not silently
	// included in the old snapshot or mistaken for a schema change.
	if _, e = db.ExecContext(ctx, "INSERT INTO domain_event_outbox VALUES(?,?,?,?)", rows+1, "evaluation.outcome.committed", "published", `{"new":"NEXT_CYCLE"}`); e != nil {
		t.Fatal(e)
	}
	if _, e = col.InsertOne(ctx, bson.D{{Key: "_id", Value: int64(rows + 1)}, {Key: "status", Value: "published"}}); e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(r)
	requestPath = filepath.Join(dir, "inventory-request.json")
	if e = os.WriteFile(requestPath, raw, 0600); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(dir, "inventory-789-1")
	if e = os.Mkdir(out, 0700); e != nil {
		t.Fatal(e)
	}
	result, e := run(requestPath, digestRaw(raw), "123-1", "789-1", out)
	if e != nil || !result.Complete || result.DropReady {
		t.Fatalf("paged inventory failed: %v", e)
	}
	for i, s := range result.Targets {
		expected := uint64(0)
		switch i {
		case 0, 3:
			expected = uint64(rows)
		case 1:
			expected = uint64(uuidRows)
		}
		if s.Records != expected || s.Passes != 2 || !s.Complete || s.Boundary == nil {
			t.Fatalf("target %d coverage failed", i)
		}
		if i == 0 || i == 3 {
			if !s.NextCycleRequired || s.Pages < 6 {
				t.Fatal("fixed upper or page boundary not exercised")
			}
		}
		if _, e = os.Stat(filepath.Join(out, s.SourceFile+".asset.json")); e != nil {
			t.Fatal("source copy unregistered")
		}
	}
	public, _ := json.Marshal(safeSummary(result))
	if strings.Contains(string(public), "PRIVATE_") || strings.Contains(string(public), "NEXT_CYCLE") {
		t.Fatal("private source leaked")
	}
	if _, e = run(requestPath, digestRaw(raw), "123-1", "789-1", out); e == nil {
		t.Fatal("existing source/checkpoint overwritten")
	}
	// Failed/time-limited scans retain private page checkpoints and cannot be
	// converted into complete inventory. No automatic continuation exists.
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
			t.Error(e)
		}
	}()
	columns, e := scanSQL(ctx, tx, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='domain_event_outbox' ORDER BY ORDINAL_POSITION")
	if e != nil {
		t.Fatal(e)
	}
	tiny := productionLimits()
	tiny.PageSize = 3
	tiny.MaxPages = 1
	bounded := privateTestDir(t)
	if _, e = sqlPagedPass(ctx, tx, "domain_event_outbox", "id", columns, r.Boundaries[0], tiny, bounded, 1, nil); e == nil || e.Error() != "target_page_bound_exceeded" {
		t.Fatal("page cap accepted")
	}
	if _, e = os.Stat(filepath.Join(bounded, "mysql-domain_event_outbox-pass-1-page-000001.checkpoint.json")); e != nil {
		t.Fatal("interruption lost real cursor")
	}
	tiny.MaxPages = 1001
	tiny.MaxRecords = 2
	if _, e = sqlPagedPass(ctx, tx, "domain_event_outbox", "id", columns, r.Boundaries[0], tiny, privateTestDir(t), 1, nil); e == nil || e.Error() != "target_record_bound_exceeded" {
		t.Fatal("row cap accepted")
	}
	tiny.MaxRecords = 1000000
	tiny.MaxBytes = 1
	if _, e = sqlPagedPass(ctx, tx, "domain_event_outbox", "id", columns, r.Boundaries[0], tiny, privateTestDir(t), 1, nil); e == nil || e.Error() != "target_byte_bound_exceeded" {
		t.Fatal("byte cap accepted")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, e = sqlPagedPass(cancelled, tx, "domain_event_outbox", "id", columns, r.Boundaries[0], productionLimits(), privateTestDir(t), 1, nil); e == nil {
		t.Fatal("timeout accepted")
	}
	// Pin an RR view, then insert above its upper through a separate caller.
	// Only the post-RR live observer is allowed to conclude next-cycle work.
	var pinned uint64
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM domain_event_outbox").Scan(&pinned); e != nil {
		t.Fatal(e)
	}
	_, beforeLiveDefs, e := mysqlCatalog(ctx, tx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = admin.ExecContext(ctx, "INSERT INTO "+quote(name)+".domain_event_outbox VALUES(?,?,?,?)", rows+2, "evaluation.outcome.committed", "published", `{"new":"DURING_RR"}`); e != nil {
		t.Fatal(e)
	}
	var invisible bool
	if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM domain_event_outbox WHERE id > ?)", rows+1).Scan(&invisible); e != nil || invisible {
		t.Fatal("native RR fixture did not isolate concurrent insert")
	}
	if e = tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	liveBound := r.Boundaries[0]
	liveBound.UpperToken = base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(rows + 1)))
	liveTargets := []snapshot{{Database: "mysql", Name: "domain_event_outbox", Present: true, Boundary: &liveBound}}
	if e = mysqlObserveAfterUpper(ctx, db, r, digest(beforeLiveDefs), liveTargets); e != nil || !liveTargets[0].NextCycleRequired {
		t.Fatal("post-RR live observer missed concurrent insert")
	}
	// Exact PK bounds must also cover an inbound FK from another schema.
	outside := name + "_outside"
	if _, e = admin.ExecContext(ctx, "CREATE DATABASE "+quote(outside)); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e = admin.ExecContext(context.Background(), "DROP DATABASE "+quote(outside)); e != nil {
			t.Error(e)
		}
	}()
	if _, e = admin.ExecContext(ctx, "CREATE TABLE "+quote(outside)+".inbound(id BIGINT UNSIGNED PRIMARY KEY,FOREIGN KEY(id) REFERENCES "+quote(name)+".domain_event_outbox(id))"); e != nil {
		t.Fatal(e)
	}
	metadataTx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := metadataTx.Rollback(); e != nil {
			t.Error(e)
		}
	}()
	kinds, defs, e := mysqlCatalog(ctx, metadataTx)
	if e != nil || kinds["domain_event_outbox"] != "BASE TABLE" || len(defs["inbound_constraints"].([][]*string)) != 1 {
		t.Fatal("cross-schema inbound FK omitted")
	}
}

func TestOwnedMongoBSONCursorTypeProofAndInterruptedPages(t *testing.T) {
	localInventoryGuard(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client, e := mongoOpen(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := client.Disconnect(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	name := "qs_retirement_inventory_test_cursor_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	db := client.Database(name)
	defer func() {
		if e := db.Drop(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	for _, tc := range []struct {
		kind string
		ids  []any
	}{{"string", []any{"1", "10", "2", "z"}}, {"objectId", []any{primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()}}, {"int", []any{int32(-10), int32(2), int32(10), int32(20)}}, {"long", []any{int64(-10), int64(2), int64(10), int64(9223372036854775807)}}} {
		col := db.Collection(tc.kind)
		var docs []any
		for _, id := range tc.ids {
			docs = append(docs, bson.D{{Key: "_id", Value: id}, {Key: "status", Value: "published"}})
		}
		if _, e = col.InsertMany(ctx, docs); e != nil {
			t.Fatal(e)
		}
		token, kind, empty, e := mongoUpper(ctx, col, productionLimits())
		if e != nil || empty || kind != tc.kind {
			t.Fatalf("type %s rejected: %v", tc.kind, e)
		}
		b := targetBoundary{Database: "mongodb", Present: true, PKType: kind, UpperToken: token}
		limits := productionLimits()
		limits.PageSize = 2
		result, e := mongoPagedPass(ctx, col, b, limits, privateTestDir(t), 1, nil)
		if e != nil || result.Records != 4 || result.Pages != 3 {
			t.Fatalf("typed boundary %s incomplete: %v", tc.kind, e)
		}
		limited := limits
		limited.MaxPages = 1
		dir := privateTestDir(t)
		if _, e = mongoPagedPass(ctx, col, b, limited, dir, 1, nil); e == nil || e.Error() != "target_page_bound_exceeded" {
			t.Fatal("BSON checkpoint page cap accepted")
		}
		raw, e := os.ReadFile(filepath.Join(dir, "mongodb-domain_event_outbox-pass-1-page-000001.checkpoint.json"))
		if e != nil {
			t.Fatal(e)
		}
		var checkpoint map[string]any
		if e = json.Unmarshal(raw, &checkpoint); e != nil {
			t.Fatal(e)
		}
		cursorRaw, e := base64.StdEncoding.DecodeString(checkpoint["cursor_token"].(string))
		if e != nil {
			t.Fatal(e)
		}
		_, cursorKind, e := decodeMongoToken(cursorRaw)
		if e != nil || cursorKind != kind {
			t.Fatal("checkpoint BSON type coerced")
		}
		// This cursor is evidence of progress, not permission to resume a source
		// file. A new scan requires an independent fresh run directory.
		if _, e = mongoPagedPass(ctx, col, b, limited, dir, 1, nil); e == nil {
			t.Fatal("checkpoint overwritten")
		}
	}
	mixed := db.Collection("mixed")
	if _, e = mixed.InsertMany(ctx, []any{bson.D{{Key: "_id", Value: "string"}}, bson.D{{Key: "_id", Value: primitive.NewObjectID()}}}); e != nil {
		t.Fatal(e)
	}
	if _, _, _, e = mongoUpper(ctx, mixed, productionLimits()); e == nil || e.Error() != "mongo_mixed_id_types_unsupported" {
		t.Fatal("type bracketing could skip mixed IDs")
	}
	unsupported := db.Collection("unsupported")
	if _, e = unsupported.InsertOne(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "compound", Value: 1}}}}); e != nil {
		t.Fatal(e)
	}
	if _, _, _, e = mongoUpper(ctx, unsupported, productionLimits()); e == nil || e.Error() != "mongo_id_type_unsupported" {
		t.Fatal("unsupported BSON type accepted")
	}
	if e = db.CreateCollection(ctx, "empty", options.CreateCollection()); e != nil {
		t.Fatal(e)
	}
	if token, kind, empty, e := mongoUpper(ctx, db.Collection("empty"), productionLimits()); e != nil || !empty || token != "" || kind != "" {
		t.Fatal("empty collection upper not explicit")
	}
}
