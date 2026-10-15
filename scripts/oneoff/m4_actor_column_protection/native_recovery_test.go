//go:build integration

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// A normal provider records dirty102 before these exact original DROP prefixes
// and an owned SIGNAL. The fixture never manufactures a ledger or calls Force.
func (h *actorNative) upgradePrefix(drops int, clean bool) {
	h.t.Helper()
	if clean {
		if drops != 2 {
			h.t.Fatal("only the complete original migration may produce clean102")
		}
		h.migrateSQL(102)
		h.assertSQLHead(102, false)
		return
	}
	if drops < 0 || drops > 2 {
		h.t.Fatal("fixed two-DROP prefix required")
	}
	raw := h.sqlSource["000102_retire_actor_compatibility_columns.up.sql"].Data
	parts := strings.Split(string(raw), "ALTER TABLE `operators`")
	if len(parts) != 2 || strings.Count(string(raw), "DROP COLUMN") != 2 {
		h.t.Fatal("exact historical102 DROP resource unavailable")
	}
	fault := ""
	if drops >= 1 {
		fault = parts[0]
	}
	if drops == 2 {
		fault += "ALTER TABLE `operators`" + parts[1]
	}
	fault += "\nSIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned interrupted102';\n"
	source := fstest.MapFS{}
	for name, file := range h.sqlSource {
		if !strings.HasPrefix(name, "000102_") {
			source[name] = file
		}
	}
	source["000102_owned_fault.up.sql"] = &fstest.MapFile{Data: []byte(fault)}
	source["000102_owned_fault.down.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	sd, e := iofs.New(source, ".")
	if e != nil {
		h.t.Fatal("owned prefix source unavailable")
	}
	pool, e := sql.Open("mysql", h.dsn)
	if e != nil {
		_ = sd.Close()
		h.t.Fatal("owned prefix pool unavailable")
	}
	dd, e := migratemysql.WithInstance(pool, &migratemysql.Config{DatabaseName: h.name})
	if e != nil {
		_ = sd.Close()
		_ = pool.Close()
		h.t.Fatal("owned normal prefix provider unavailable")
	}
	m, e := migrate.NewWithInstance("iofs", sd, "mysql", dd)
	if e != nil {
		_ = sd.Close()
		_ = dd.Close()
		h.t.Fatal("owned prefix migration unavailable")
	}
	e = m.Steps(1)
	se, de := m.Close()
	if e == nil || se != nil || de != nil {
		h.t.Fatal("normal provider did not observe and release interrupted102")
	}
	h.assertSQLHead(102, true)
}

func (h *actorNative) assertSQLHead(version uint64, dirty bool) {
	h.t.Helper()
	want := [][]sql.NullString{{{String: strconv.FormatUint(version, 10), Valid: true}, {String: "0", Valid: true}}}
	if dirty {
		want[0][1].String = "1"
	}
	if !reflect.DeepEqual(h.query("SELECT version,dirty FROM schema_migrations ORDER BY version"), want) {
		h.t.Fatal("actual provider ledger state differs")
	}
}

func (h *actorNative) richOriginalValues(extra int) {
	h.t.Helper()
	for _, query := range []string{
		`UPDATE testee SET tags=JSON_OBJECT('z',JSON_ARRAY('第二','first','🧭'),'a','retained') WHERE id=18446744073709551615`,
		`UPDATE operators SET effective_roles=JSON_OBJECT('z',JSON_ARRAY('historical','🧭','second'),'a',TRUE) WHERE id=18446744073709551615`,
	} {
		if _, e := h.db.ExecContext(h.t.Context(), query); e != nil {
			h.t.Fatal("owned four-byte/raw-order original values unavailable")
		}
	}
	for _, target := range []struct{ table, column string }{{"testee", "tags"}, {"operators", "effective_roles"}} {
		raw := h.query("SELECT CAST(`" + target.column + "` AS BINARY) FROM `" + target.table + "` WHERE id=18446744073709551615")
		if len(raw) != 1 || !raw[0][0].Valid || !strings.Contains(raw[0][0].String, "🧭") {
			h.t.Fatal("four-byte original JSON was not retained by the actual server")
		}
	}
	for n := 0; n < extra; n++ {
		id := 1000 + n
		if _, e := h.db.ExecContext(h.t.Context(), `INSERT INTO testee(id,org_id,name,gender,tags) VALUES(?,1,'owned-batch',0,JSON_ARRAY('🧭','second','first',?))`, id, n); e != nil {
			h.t.Fatal("owned batched tags unavailable")
		}
		if _, e := h.db.ExecContext(h.t.Context(), `INSERT INTO operators(id,org_id,user_id,roles,effective_roles,name,is_active) VALUES(?,1,?,JSON_ARRAY('qs:admin'),JSON_ARRAY('historical','🧭',?),'owned-batch',TRUE)`, id, 100000+n, n); e != nil {
			h.t.Fatal("owned batched effective roles unavailable")
		}
	}
}

type actorOriginalWitness struct {
	ddl      [5][][]sql.NullString
	allRows  [4][][]sql.NullString
	retained [4][][]sql.NullString
	ledger   [][]sql.NullString
	mongo    []byte
	binding  binding
}

func (h *actorNative) originalWitness() actorOriginalWitness {
	h.t.Helper()
	w := actorOriginalWitness{retained: h.retainedRaw(), ledger: h.query("SELECT version,dirty FROM schema_migrations ORDER BY version"), mongo: h.mongoHead(), binding: h.independentBinding()}
	for i, s := range scope {
		w.ddl[i] = h.query("SHOW CREATE TABLE `" + s.table + "`")
		var selected []string
		for _, row := range h.query("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='" + s.table + "' ORDER BY ORDINAL_POSITION") {
			selected = append(selected, "CAST(`"+row[0].String+"` AS BINARY)")
		}
		w.allRows[i] = h.query("SELECT " + strings.Join(selected, ",") + " FROM `" + s.table + "` ORDER BY `" + s.key + "`")
	}
	w.ddl[4] = h.query("SHOW CREATE TABLE schema_migrations")
	return w
}

func (h *actorNative) assertOriginalWitness(w actorOriginalWitness) {
	h.t.Helper()
	if got := h.originalWitness(); !reflect.DeepEqual(w, got) {
		h.t.Fatal("exact original DDL, raw target/retained values, complete ledger or independent Mongo identity differs")
	}
	h.assertModernReads()
}

func (h *actorNative) restoreApproval() approval {
	h.t.Helper()
	now := time.Now().UTC()
	return approval{Format: 1, Kind: "m4_c_pre_start_actor_column_restore", Operation: fmt.Sprintf("%d-1", now.UnixNano()), ToolSource: h.tool, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, MaterialHash: h.materialHash, FreezeHash: fingerprint([]byte("owned synthetic writer freeze")), ResponsibilityHash: fingerprint([]byte("owned synthetic responsibility")), Binding: h.binding, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: filepath.Join(h.dir, "first-runtime.marker"), WindowStarted: now.Add(-time.Minute).Format(time.RFC3339Nano), RecoveryDecision: now.Format(time.RFC3339Nano), Deadline: now.Add(time.Minute).Format(time.RFC3339Nano), Expires: now.Add(2 * time.Minute).Format(time.RFC3339Nano), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
}

func (h *actorNative) approvedRestoreArgs(a approval) []string {
	h.t.Helper()
	raw, e := json.Marshal(a)
	path := filepath.Join(h.dir, a.Operation+"-approval.json")
	if e != nil || writeExclusive(path, raw) != nil {
		h.t.Fatal("owned independent approval unavailable")
	}
	return []string{"--material-file=" + h.materialPath, "--material-sha256=" + h.materialHash, "--approval-file=" + path, "--approval-sha256=" + fingerprint(raw)}
}

func requireActorRestoreSuccess(t *testing.T, r receipt) {
	t.Helper()
	if !r.Complete || r.ReadOnly || !r.ApprovalConsumed || !r.SQLSnapshot || !r.MandatoryEOF || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.ReleaseError != "" || r.TransactionReleaseUnproven || r.Error != "none" || r.ProductionFreezeCertified || r.CrossDatabaseAtomicRR {
		t.Fatal("actual restore or owned release was not proved", r.Error, r.ReleaseError)
	}
}

func TestM4ActorColumnsNativeRestoreOriginalStates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drops int
		clean bool
	}{{"dirty_before_drop", 0, false}, {"dirty_after_tags_drop", 1, false}, {"dirty_after_both_drops", 2, false}, {"clean_after_both_drops", 2, true}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newActorNative(t)
			h.richOriginalValues(0)
			before := h.originalWitness()
			h.capture()
			h.upgradePrefix(tc.drops, tc.clean)
			if tc.clean {
				if r := h.verify(); !r.Complete {
					t.Fatal("normal clean102 verification failed", r.Error)
				}
			}
			r := h.action("restore", h.approvedRestoreArgs(h.restoreApproval())...)
			requireActorRestoreSuccess(t, r)
			h.assertSQLHead(101, false)
			h.assertOriginalWitness(before)
			t.Log("original column definitions/raw JSON/SQL NULL/0/MAX, four retained tables, modern archive readers and original10140 ledger restored; no business runtime started")
		})
	}
}

func TestM4ActorColumnsNativeRestoreContinuation(t *testing.T) {
	for _, step := range []string{"roles_add_ack", "first_roles_batch_commit_ack", "tags_add_ack"} {
		t.Run(step, func(t *testing.T) {
			h := newActorNative(t)
			h.richOriginalValues(205)
			before := h.originalWitness()
			h.capture()
			h.upgradePrefix(2, true)
			a := h.restoreApproval()
			args := h.approvedRestoreArgs(a)
			directDSN := h.dsn
			observed, undo := h.dropRealRestoreACK(step)
			r := h.action("restore", args...)
			undo()
			h.dsn = directDSN
			if r.Complete || !r.ApprovalConsumed || !observed.Load() || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.ReleaseError != "" || r.TransactionReleaseUnproven {
				t.Fatal("actual acknowledged interruption or owned release was not proved", r.Error)
			}
			h.assertSQLHead(102, false)
			if !reflect.DeepEqual(before.retained, h.retainedRaw()) || !bytes.Equal(before.mongo, h.mongoHead()) || before.binding != h.independentBinding() {
				t.Fatal("interrupted recovery changed retained facts or Mongo identity")
			}
			roles := h.query("SELECT COUNT(*),SUM(effective_roles IS NULL) FROM operators")
			nullable := h.query("SELECT IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='operators' AND COLUMN_NAME='effective_roles'")
			wantNull, wantNullable := "209", "YES"
			if step == "first_roles_batch_commit_ack" {
				wantNull = "109"
			}
			if step == "tags_add_ack" {
				wantNull, wantNullable = "0", "NO"
				if v := h.query("SELECT COUNT(*),SUM(tags IS NULL) FROM testee"); len(v) != 1 || v[0][0].String != "209" || v[0][1].String != "209" {
					t.Fatal("acknowledged tags ADD did not leave exact all-NULL tags")
				}
			}
			if len(roles) != 1 || roles[0][0].String != "209" || roles[0][1].String != wantNull || len(nullable) != 1 || nullable[0][0].String != wantNullable {
				t.Fatal("actual nullable/partial batch state differs from the acknowledged boundary")
			}
			partial := h.originalWitness()
			same := h.action("restore", args...)
			if same.Complete || same.ApprovalConsumed || !reflect.DeepEqual(partial, h.originalWitness()) {
				t.Fatal("consumed operation was reused or changed partial recovery")
			}
			firstBoundary, _, e := recoveryPaths(h.original, h.materialHash, a.Operation)
			originalBoundary, be := readOwnedPrivate(firstBoundary, 32768)
			if e != nil || be != nil {
				t.Fatal("first recovery boundary not retained")
			}
			a.Operation = fmt.Sprintf("%d-2", time.Now().UnixNano())
			requireActorRestoreSuccess(t, h.action("restore", h.approvedRestoreArgs(a)...))
			h.assertOriginalWitness(before)
			afterBoundary, e := readOwnedPrivate(firstBoundary, 32768)
			if e != nil || !bytes.Equal(originalBoundary, afterBoundary) {
				t.Fatal("continuation replaced first deadline/material/marker scope")
			}
			t.Log("real server OK was dropped; old receipt remains failure, original102 ledger retained, new operation continued the same first boundary and exactly restored original facts")
		})
	}
}

func TestM4ActorColumnsNativeRestoreRefusesForeignPartialState(t *testing.T) {
	h := newActorNative(t)
	h.capture()
	h.upgradePrefix(2, true)
	if _, e := h.db.ExecContext(t.Context(), "ALTER TABLE operators ADD COLUMN effective_roles JSON NULL AFTER roles"); e != nil {
		t.Fatal("owned foreign nullable state unavailable")
	}
	before := h.originalWitness()
	a := h.restoreApproval()
	r := h.action("restore", h.approvedRestoreArgs(a)...)
	if r.Complete || r.ApprovalConsumed || r.Error != "unproved_restore_prefix_or_partial_state" || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || !reflect.DeepEqual(before, h.originalWitness()) {
		t.Fatal("fresh boundary promoted a foreign nullable intermediate state", r.Error)
	}
	boundary, claim, e := recoveryPaths(h.original, h.materialHash, a.Operation)
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{boundary, claim} {
		if _, e := os.Lstat(path); !os.IsNotExist(e) {
			t.Fatal("rejected foreign partial state created recovery authority")
		}
	}
}
