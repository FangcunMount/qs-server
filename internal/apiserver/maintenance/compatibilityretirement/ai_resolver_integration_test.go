//go:build integration

package retirement

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

type aiNativeFixture struct {
	db             *sql.DB
	binding        AIResolverBinding
	bridge, legacy *DecodedAICommand
	graph          aiLocalGraphFixture
}

// Native tests may only use the root-owned disposable fixture, after exact
// container/CID/image/labels/volume and loopback-port verification. Credentials
// are supplied by the private child environment, never argv or diagnostics.
func aiNativeOwnedMySQL(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("QS_RETIREMENT_LOCAL_INTEGRATION") != "1" {
		t.Skip("owned local native fixture not requested")
	}
	if os.Getenv("MYSQL_HOST") != "127.0.0.1" || os.Getenv("MYSQL_PORT") != "34306" {
		t.Fatal("non-owned native endpoint rejected")
	}
	raw, err := os.ReadFile("/private/tmp/qs-compatibility-retirement/owned-mysql.json")
	if err != nil {
		t.Fatal("owned fixture manifest required")
	}
	var manifest struct {
		Version      int               `json:"format_version"`
		ID           string            `json:"container_id"`
		Name         string            `json:"container_name"`
		Image        string            `json:"image_id"`
		Architecture string            `json:"image_architecture"`
		Labels       map[string]string `json:"labels"`
		Port         int               `json:"loopback_port"`
		Environment  string            `json:"private_environment_file"`
		Ready        bool              `json:"ready"`
		Volumes      []string          `json:"volumes"`
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.ID == "" || manifest.Name == "" || manifest.Image == "" || len(manifest.Labels) == 0 || !manifest.Ready || manifest.Port != 34306 || len(manifest.Volumes) == 0 {
		t.Fatal("owned fixture binding invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inspect, err := exec.CommandContext(ctx, "docker", "inspect", manifest.ID).Output()
	if err != nil {
		t.Fatal("owned fixture inspection failed")
	}
	var actual []struct {
		ID              string `json:"Id"`
		Name            string
		Image           string
		Config          struct{ Labels map[string]string }
		Mounts          []struct{ Type, Name string }
		NetworkSettings struct {
			Ports map[string][]struct{ HostIP, HostPort string }
		}
	}
	if json.Unmarshal(inspect, &actual) != nil || len(actual) != 1 || actual[0].ID != manifest.ID || strings.TrimPrefix(actual[0].Name, "/") != manifest.Name || actual[0].Image != manifest.Image || !reflect.DeepEqual(actual[0].Config.Labels, manifest.Labels) {
		t.Fatal("owned fixture identity mismatch")
	}
	volumes := make([]string, 0, len(actual[0].Mounts))
	for _, mount := range actual[0].Mounts {
		if mount.Type != "volume" {
			t.Fatal("owned fixture unexpected mount")
		}
		volumes = append(volumes, mount.Name)
	}
	ports := actual[0].NetworkSettings.Ports["3306/tcp"]
	if !reflect.DeepEqual(volumes, manifest.Volumes) || len(ports) != 1 || ports[0].HostIP != "127.0.0.1" || ports[0].HostPort != "34306" {
		t.Fatal("owned fixture volume/port mismatch")
	}
	cfg := mysql.NewConfig()
	cfg.User = os.Getenv("MYSQL_USERNAME")
	cfg.Passwd = os.Getenv("MYSQL_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = "127.0.0.1:34306"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned native connection failed")
	}
	name := "qs_ai_resolver_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		if admin.Close() != nil {
			t.Error("native admin close failed")
		}
		t.Fatal("owned native schema creation failed")
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned native schema open failed")
	}
	db.SetMaxOpenConns(4)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error("owned native schema close failed")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Error("owned native schema cleanup failed")
		}
		if admin.Close() != nil {
			t.Error("owned native admin close failed")
		}
	})
	return db
}

func aiNativeExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
		var mysqlError *mysql.MySQLError
		if errors.As(err, &mysqlError) {
			t.Fatal("owned native statement rejected", mysqlError.Number, sourceSHA([]byte(q)))
		}
		t.Fatal("owned native statement rejected")
	}
}

func aiNativeInsert(t *testing.T, db *sql.DB, table string, row [][]byte) {
	t.Helper()
	switch table {
	case "ai_messaging_operations", "ai_messaging_outbox", "ai_messaging_inbox", "ai_messaging_failures", "ai_bridge_events":
	default:
		t.Fatal("native table rejected")
	}
	args := make([]any, len(row))
	for i, raw := range row {
		if raw != nil {
			args[i] = raw
		}
	}
	aiNativeExec(t, db, "INSERT INTO `"+table+"` VALUES("+strings.TrimSuffix(strings.Repeat("?,", len(row)), ",")+")", args...)
}

func aiNativeBinding(t *testing.T, db *sql.DB) AIResolverBinding {
	t.Helper()
	b := aiLocalTestBinding()
	reader := &AILocalResolver{pool: db}
	identity, err := reader.read(context.Background(), "SELECT @@server_uuid,DATABASE()")
	if err != nil || len(identity) != 1 || len(identity[0]) != 2 {
		t.Fatal("native identity read")
	}
	b.DatabaseIdentityHash = aiFramedParts("mysql_database_identity_v1", string(identity[0][0]), string(identity[0][1]))
	for _, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		ddl, err := reader.read(context.Background(), "SHOW CREATE TABLE `"+table+"`")
		if err != nil || len(ddl) != 1 || len(ddl[0]) != 2 {
			t.Fatal("native source schema")
		}
		meta, err := reader.read(context.Background(), "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION", table)
		if err != nil {
			t.Fatal("native source metadata")
		}
		columns := make(SQLColumns, len(meta))
		for i, row := range meta {
			columns[i] = make([]*string, len(row))
			for j, raw := range row {
				if raw != nil {
					columns[i][j] = ptrString(string(raw))
				}
			}
		}
		upper, err := reader.read(context.Background(), "SELECT CAST(MAX(command_id) AS BINARY) FROM `"+table+"`")
		if err != nil || len(upper) != 1 || len(upper[0]) != 1 {
			t.Fatal("native source upper")
		}
		boundary := SourceBoundary{Database: "mysql", Name: table, Kind: "base_table", Present: true, Empty: upper[0][0] == nil, PKType: "ascii_string", SchemaHash: aiJSONHash(SQLColumns{{ptrString(string(ddl[0][0])), ptrString(string(ddl[0][1]))}})}
		boundary.IdentityHash = aiFramedParts("mysql-object-v1", table, boundary.SchemaHash)
		if !boundary.Empty {
			boundary.UpperToken = base64.StdEncoding.EncodeToString(upper[0][0])
		}
		if table == AIBridgeCommandSource {
			b.BridgeBoundary, b.BridgeColumns = boundary, columns
		} else {
			b.LegacyBoundary, b.LegacyColumns = boundary, columns
		}
	}
	return b
}

func newAINativeFixture(t *testing.T, mapped bool) *aiNativeFixture {
	t.Helper()
	db := aiNativeOwnedMySQL(t)
	for _, name := range []string{"000072_ai_bridge_delivery.up.sql", "000083_ai_runtime_index.up.sql", "000091_ai_messaging.up.sql", "000092_ai_messaging_failures.up.sql", "000093_ai_messaging_legacy_commands.up.sql", "000094_ai_messaging_admission.up.sql", "000097_ai_command_retirement.up.sql"} {
		raw, err := os.ReadFile(filepath.Join("../../../../internal/pkg/migration/migrations/mysql", name))
		if err != nil {
			t.Fatal("native retained schema source missing")
		}
		var lines []string
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				lines = append(lines, line)
			}
		}
		for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
			if strings.TrimSpace(statement) != "" {
				aiNativeExec(t, db, statement)
			}
		}
	}
	aiNativeExec(t, db, "CREATE TABLE schema_migrations(version BIGINT NOT NULL,dirty BOOLEAN NOT NULL)")
	aiNativeExec(t, db, "INSERT INTO schema_migrations VALUES(99,FALSE)")
	aiNativeExec(t, db, "CREATE TABLE borrowed_marker(id INT PRIMARY KEY)")
	aiNativeExec(t, db, "UPDATE ai_messaging_admission SET closed=TRUE,revision=7 WHERE singleton=1")
	graph := aiLocalGraph(t)
	raw, _ := aiFixturePayload(t, "start")
	projectionRaw, err := json.Marshal(graph.projection)
	if err != nil {
		t.Fatal("native projection encoding")
	}
	aiNativeExec(t, db, `INSERT INTO ai_bridge_requests(request_id,request_hash,payload,session_id,version,status,projection,organization_id,subject_id,testee_id,created_at,updated_at) VALUES(?,?,CONVERT(? USING utf8mb4),?,?,?,CONVERT(? USING utf8mb4),?,?,?,'2026-10-08 11:12:12.123456','2026-10-08 11:12:14.123456')`, graph.request.RequestID, sourceSHA(raw), raw, graph.projection.SessionID, graph.projection.Version, graph.projection.Status, projectionRaw, graph.request.Actor.OrgID, graph.request.Actor.SubjectID, graph.request.TesteeID)
	delivered := 1
	if mapped {
		aiNativeExec(t, db, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", graph.request.RequestID)
		delivered = 0
	}
	aiNativeExec(t, db, `INSERT INTO ai_bridge_commands(command_id,request_id,kind,payload,payload_hash,delivered,attempts,available_at) SELECT request_id,request_id,'start',payload,request_hash,?,3,'2026-10-08 11:12:13.123456' FROM ai_bridge_requests`, delivered)
	for _, id := range graph.request.AssessmentIDs {
		aiNativeExec(t, db, "INSERT INTO ai_bridge_request_assessments VALUES(?,?)", graph.request.RequestID, id)
	}
	for _, row := range graph.events {
		aiNativeInsert(t, db, "ai_bridge_events", row)
	}
	if mapped {
		aiNativeExec(t, db, `INSERT INTO ai_messaging_legacy_commands SELECT command_id,request_id,kind,CAST(payload AS BINARY),payload_hash,attempts,available_at,'2026-10-08T19:12:12.123456+08:00',?,'2026-10-08 11:12:14.123456' FROM ai_bridge_commands`, graph.hashes[graph.request.RequestID])
		for _, group := range []struct {
			table string
			rows  [][][]byte
		}{{"ai_messaging_operations", graph.ops}, {"ai_messaging_outbox", graph.boxes}, {"ai_messaging_inbox", graph.inbox}, {"ai_messaging_failures", graph.failures}} {
			for _, row := range group.rows {
				aiNativeInsert(t, db, group.table, row)
			}
		}
	}
	binding := aiNativeBinding(t, db)
	reader := &AILocalResolver{pool: db}
	rows, err := reader.read(context.Background(), aiBridgeQuery, graph.request.RequestID, graph.request.RequestID)
	if err != nil || len(rows) != 1 {
		t.Fatal("native bridge source read")
	}
	bridge, err := aiCurrentSource(rows[0], AIBridgeCommandSource, binding.BridgeColumns, binding.BridgeBoundary)
	if err != nil {
		t.Fatal("native full bridge decode", err)
	}
	var legacySource *DecodedAICommand
	if mapped {
		rows, err = reader.read(context.Background(), aiLegacyQuery, graph.request.RequestID, graph.request.RequestID)
		if err != nil || len(rows) != 1 {
			t.Fatal("native legacy source read")
		}
		legacySource, err = aiCurrentSource(rows[0], AILegacyCommandSource, binding.LegacyColumns, binding.LegacyBoundary)
		if err != nil {
			t.Fatal("native full legacy decode", err)
		}
	}
	return &aiNativeFixture{db: db, binding: binding, bridge: bridge, legacy: legacySource, graph: graph}
}

func aiNativeTx(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal("native original transaction begin")
	}
	t.Cleanup(func() {
		err := tx.Rollback()
		if err != nil && err != sql.ErrTxDone {
			t.Error("native original transaction cleanup")
		}
	})
	return tx
}

func TestAILocalNativeBorrowedTransactionAndUnknownExternal(t *testing.T) {
	for _, mapped := range []bool{false, true} {
		t.Run(strconv.FormatBool(mapped), func(t *testing.T) {
			f := newAINativeFixture(t, mapped)
			tx := aiNativeTx(t, f.db)
			if _, err := tx.Exec("INSERT INTO borrowed_marker VALUES(1)"); err != nil {
				t.Fatal("native marker insertion")
			}
			r, err := NewAILocalResolver(context.Background(), aiLocalGORM(tx), f.binding)
			if err != nil {
				t.Fatal("native borrowed resolver", err)
			}
			q, err := r.Resolve(context.Background(), f.bridge, f.legacy)
			if err != nil {
				t.Fatal("native qualified resolver", err)
			}
			if err = q.Recheck(context.Background(), aiLocalGORM(tx)); err != nil {
				t.Fatal("same native baseline rejected", err)
			}
			if s := q.Summary(); !s.LocalQualified || s.DropReady || s.ExternalClosure != "unknown" || s.GlobalReverseCoverage != "unknown" || len(s.Gaps) < 4 {
				t.Fatal("native qualification overstated")
			}
			var count int
			if err = f.db.QueryRow("SELECT COUNT(*) FROM borrowed_marker").Scan(&count); err != nil || count != 0 {
				t.Fatal("resolver committed borrowed transaction")
			}
			if tx.Rollback() != nil {
				t.Fatal("resolver ended borrowed transaction")
			}
			if err = f.db.QueryRow("SELECT COUNT(*) FROM borrowed_marker").Scan(&count); err != nil || count != 0 {
				t.Fatal("native rollback did not preserve ownership")
			}
		})
	}
}

func TestAILocalNativeRecheckCurrentReadsFullBaseline(t *testing.T) {
	f := newAINativeFixture(t, false)
	tx := aiNativeTx(t, f.db)
	r, err := NewAILocalResolver(context.Background(), aiLocalGORM(tx), f.binding)
	if err != nil {
		t.Fatal("native resolver", err)
	}
	q, err := r.Resolve(context.Background(), f.bridge, nil)
	if err != nil {
		t.Fatal("native qualification", err)
	}
	if tx.Rollback() != nil {
		t.Fatal("native baseline release")
	}
	// Establish a genuine stale RR snapshot before the external writer changes
	// a transport-only field. Recheck's locking reads must still see current.
	checkTx := aiNativeTx(t, f.db)
	var attempts int
	if err = checkTx.QueryRow("SELECT attempts FROM ai_bridge_commands WHERE command_id=?", f.bridge.CommandID).Scan(&attempts); err != nil || attempts != 3 {
		t.Fatal("native stale snapshot establishment")
	}
	aiNativeExec(t, f.db, "UPDATE ai_bridge_commands SET attempts=4 WHERE command_id=?", f.bridge.CommandID)
	if err = q.Recheck(context.Background(), aiLocalGORM(checkTx)); err != ErrAILocalChanged || q.Summary().LocalQualified {
		t.Fatal("stale snapshot hid changed source")
	}
	if err = q.Recheck(context.Background(), aiLocalGORM(checkTx)); err != ErrAILocalChanged {
		t.Fatal("invalid native qualification resumed")
	}
}

func TestAILocalNativeRejectsUnfinishedWrongOwnerAndCorruption(t *testing.T) {
	cases := []struct {
		name   string
		mapped bool
		query  string
		args   []any
		want   SourceError
	}{
		{"business_active", false, "UPDATE ai_bridge_requests SET status='running',projection=JSON_SET(projection,'$.status','running')", nil, ErrAILocalTerminal},
		{"owner_index_changed", false, "UPDATE ai_bridge_requests SET organization_id=1", nil, ErrAILocalRelation},
		{"request_hash_changed", false, "UPDATE ai_bridge_requests SET request_hash=?", []any{strings.Repeat("f", 64)}, ErrAILocalRelation},
		{"event_hash_changed", false, "UPDATE ai_bridge_events SET payload_hash=?", []any{strings.Repeat("f", 64)}, ErrAILocalTerminal},
		{"orphan_wrong_org_operation", true, "UPDATE ai_messaging_operations SET organization_id=1", nil, ErrAILocalRelation},
		{"pending_outbox", true, "UPDATE ai_messaging_outbox SET stage='published',confirmed_at=NULL WHERE requires_receipt=TRUE", nil, ErrAILocalResponsibility},
		{"orphan_ack", true, "DELETE FROM ai_messaging_inbox LIMIT 1", nil, ErrAILocalRelation},
		{"held_inbox", true, "UPDATE ai_messaging_inbox SET outcome='held' LIMIT 1", nil, ErrAILocalResponsibility},
		{"unknown_failure", false, "INSERT INTO ai_messaging_failures VALUES('qs-ai',?,?,7,?,'private-corrupt-wire',8,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", []any{aiFixtureCommandID, strings.Repeat("f", 64), aiFixtureRequestID}, ErrAILocalResponsibility},
		{"quarantine_unknown", false, "INSERT INTO ai_messaging_quarantine VALUES(?,'private-corrupt-wire','private-code',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", []any{strings.Repeat("f", 64)}, ErrAILocalUnknown},
		{"admission_open", false, "UPDATE ai_messaging_admission SET closed=FALSE", nil, ErrAILocalBinding},
		{"dirty_head", false, "UPDATE schema_migrations SET dirty=TRUE", nil, ErrAILocalBinding},
		{"extra_source_column", false, "ALTER TABLE ai_bridge_commands ADD COLUMN unexpected_source_fact INT NULL", nil, ErrAILocalBinding},
		{"missing_aggregate_order", true, "DELETE FROM ai_messaging_aggregates", nil, ErrAILocalRelation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAINativeFixture(t, tc.mapped)
			aiNativeExec(t, f.db, tc.query, tc.args...)
			tx := aiNativeTx(t, f.db)
			r, err := NewAILocalResolver(context.Background(), aiLocalGORM(tx), f.binding)
			if err == nil {
				_, err = r.Resolve(context.Background(), f.bridge, f.legacy)
			}
			if err != tc.want {
				t.Fatal("native fixed rejection mismatch", err)
			}
		})
	}
}

func TestAILocalNativeWrongDatabaseAndInactiveTransaction(t *testing.T) {
	f := newAINativeFixture(t, false)
	tx := aiNativeTx(t, f.db)
	b := f.binding
	b.DatabaseIdentityHash = aiFramedParts("mysql_database_identity_v1", "different-uuid", "different-db")
	if _, err := NewAILocalResolver(context.Background(), aiLocalGORM(tx), b); err != ErrAILocalBinding {
		t.Fatal("wrong native database accepted")
	}
	if tx.Rollback() != nil {
		t.Fatal("native transaction rollback")
	}
	if _, err := NewAILocalResolver(context.Background(), aiLocalGORM(tx), f.binding); err != ErrAILocalRead {
		t.Fatal("inactive native transaction accepted")
	}
}
