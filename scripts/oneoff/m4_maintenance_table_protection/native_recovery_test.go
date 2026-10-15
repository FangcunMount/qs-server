//go:build integration

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
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

const original103 = "000103_retire_legacy_maintenance_tables.up.sql"

// Clean103 executes the complete exact historical resource. Dirty cases use a
// clearly test-only source: an exact original DROP prefix followed by SIGNAL.
// The normal provider owns every dirty/clean ledger write. No Force, SetVersion
// or direct fixture ledger manipulation manufactures these states.
func (h *maintenanceNative) upgradePrefix(drops int, clean bool) {
	h.t.Helper()
	if clean {
		if drops != len(scope) {
			h.t.Fatal("clean requires exact complete original103")
		}
		h.migrateSQL(103)
		h.assertSQLHead(103, false)
		return
	}
	if drops < 0 || drops > len(scope) {
		h.t.Fatal("fixed five DROP prefix required")
	}
	raw := h.sqlSource[original103].Data
	parts := strings.Split(strings.TrimSpace(string(raw)), ";")
	if len(parts) != len(scope)+1 || parts[len(scope)] != "" {
		h.t.Fatal("exact original103 statements unavailable")
	}
	for i, s := range scope {
		if strings.TrimSpace(parts[i]) != "DROP TABLE `"+s.name+"`" {
			h.t.Fatal("original103 DROP order changed")
		}
	}
	var prefix strings.Builder
	for _, statement := range parts[:drops] {
		prefix.WriteString(statement)
		prefix.WriteByte(';')
	}
	prefix.WriteString("\nSIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned interrupted103';\n")
	source := fstest.MapFS{}
	for name, file := range h.sqlSource {
		if !strings.HasPrefix(name, "000103_") {
			source[name] = file
		}
	}
	source["000103_owned_fault.up.sql"] = &fstest.MapFile{Data: []byte(prefix.String())}
	source["000103_owned_fault.down.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
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
		h.t.Fatal("normal prefix provider unavailable")
	}
	m, e := migrate.NewWithInstance("iofs", sd, "mysql", dd)
	if e != nil {
		_ = sd.Close()
		_ = dd.Close()
		h.t.Fatal("normal prefix migration assembly failed")
	}
	e = m.Steps(1)
	se, de := m.Close()
	if e == nil || se != nil || de != nil {
		h.t.Fatal("interrupted103 not observed or provider release unproven")
	}
	h.assertSQLHead(103, true)
	h.t.Logf("fixture controlled original103 prefix=%d; original_sha=%s fault_sha=%s; provider dirty103 observed", drops, fingerprint(raw), fingerprint([]byte(prefix.String())))
}

func (h *maintenanceNative) assertSQLHead(version uint64, dirty bool) {
	h.t.Helper()
	want := [][]sql.NullString{{{String: strconv.FormatUint(version, 10), Valid: true}, {String: "0", Valid: true}}}
	if dirty {
		want[0][1].String = "1"
	}
	if !reflect.DeepEqual(h.query("SELECT version,dirty FROM schema_migrations ORDER BY version"), want) {
		h.t.Fatal("actual provider head differs")
	}
}

func (h *maintenanceNative) restoreInput(operation string) ([]string, string, string) {
	h.t.Helper()
	captured, e := time.Parse(time.RFC3339Nano, h.original.CapturedAt)
	if e != nil {
		h.t.Fatal("actual first capture timestamp invalid")
	}
	now := time.Now().UTC()
	a := approval{Format: 1, Kind: "m4_d_pre_start_maintenance_table_restore", Operation: operation, ToolSource: h.tool, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, MaterialHash: h.materialHash, FreezeHash: fingerprint([]byte("owned fixture has no business runtime writers")), ResponsibilityHash: fingerprint([]byte("owned synthetic objects only")), Binding: h.binding, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: filepath.Join(h.dir, "first-runtime"), WindowStarted: captured.Add(-time.Second).Format(time.RFC3339Nano), RecoveryDecision: now.Format(time.RFC3339Nano), Deadline: now.Add(10 * time.Minute).Format(time.RFC3339Nano), Expires: now.Add(15 * time.Minute).Format(time.RFC3339Nano), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
	raw, e := json.Marshal(a)
	if e != nil {
		h.t.Fatal("owned approval encoding failed")
	}
	path := filepath.Join(h.dir, "approval-"+operation+".json")
	if writeExclusive(path, raw) != nil {
		h.t.Fatal("independent owned approval unavailable")
	}
	hash := fingerprint(raw)
	return []string{"--material-file=" + h.materialPath, "--material-sha256=" + h.materialHash, "--approval-file=" + path, "--approval-sha256=" + hash}, path, hash
}

func (h *maintenanceNative) assertRestoreReleased(r receipt) {
	h.t.Helper()
	if !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.ReleaseError != "" || r.TransactionReleaseUnproven || r.RuntimeGateReleaseUnproven || r.RuntimeGateAcquired && !r.RuntimeGateReleased || r.ProductionFreezeCertified || r.CrossDatabaseAtomicRR {
		h.t.Fatal("owned restore resource release unproven", r.Error, r.ReleaseError)
	}
}

func (h *maintenanceNative) restoreAndVerify(before [5][][]sql.NullString, modern [8][][]sql.NullString, head []byte, bindingBefore binding) {
	h.t.Helper()
	args, _, _ := h.restoreInput("1001-1")
	h.restoreUsingArgsAndVerify(args, before, modern, head, bindingBefore)
}

func (h *maintenanceNative) restoreUsingArgsAndVerify(args []string, before [5][][]sql.NullString, modern [8][][]sql.NullString, head []byte, bindingBefore binding) {
	h.t.Helper()
	r := h.action("restore", args...)
	h.assertRestoreReleased(r)
	if !r.Complete || r.Error != "none" || r.ReadOnly || !r.ApprovalConsumed || !r.MandatoryEOF || !r.RetainedEOF {
		h.t.Fatal("formal pre-start five-table restoration unproved", r.Error)
	}
	h.assertSQLHead(102, false)
	h.assertCaptureRaw(before)
	h.assertRetained(modern)
	h.assertModernReads()
	if !reflect.DeepEqual(before, h.originalRaw()) || !reflect.DeepEqual(modern, h.retainedRaw()) || !bytes.Equal(head, h.mongoHead()) || bindingBefore != h.independentBinding() {
		h.t.Fatal("original raw facts/modern archives/Mongo identity changed")
	}
	verified := h.action("verify-restored", args...)
	h.assertReadOnlyReleased(verified)
	if !verified.Complete || verified.Error != "none" || verified.ApprovalConsumed || !verified.MandatoryEOF || !verified.RetainedEOF {
		h.t.Fatal("fresh read-only terminal restoration proof failed", verified.Error)
	}
	// Reusing an already completed operation cannot run another restore. This
	// checks fresh resources/state, and does not infer server abort from ErrTxDone.
	reused := h.action("restore", args...)
	h.assertRestoreReleased(reused)
	if reused.Complete || reused.ApprovalConsumed || reused.Error != "restore_requires_sql103_mongo40" {
		h.t.Fatal("completed operation reused", reused.Error)
	}
	h.assertSQLHead(102, false)
}

func TestM4MaintenanceTablesNativeRestorePrefixes(t *testing.T) {
	for prefix := 0; prefix <= len(scope); prefix++ {
		t.Run(fmt.Sprintf("dirty%d", prefix), func(t *testing.T) {
			h := newMaintenanceNative(t)
			before, modern, head, identity := h.originalRaw(), h.retainedRaw(), h.mongoHead(), h.independentBinding()
			h.capture()
			h.upgradePrefix(prefix, false)
			h.assertModernReads()
			h.restoreAndVerify(before, modern, head, identity)
		})
	}
	t.Run("clean5", func(t *testing.T) {
		h := newMaintenanceNative(t)
		before, modern, head, identity := h.originalRaw(), h.retainedRaw(), h.mongoHead(), h.independentBinding()
		h.capture()
		h.upgradePrefix(len(scope), true)
		h.assertModernReads()
		h.restoreAndVerify(before, modern, head, identity)
	})
}

func TestM4MaintenanceTablesNativeRestoreClosedCollations(t *testing.T) {
	for _, collation := range originalCollations {
		t.Run(collation, func(t *testing.T) {
			h := newMaintenanceNative(t)
			// This is explicit owned fixture preparation after historical bootstrap,
			// not a modified production migration. Every full SHOW CREATE must match
			// the closed original template before the formal capture is accepted.
			for i, s := range scope {
				if _, e := h.db.ExecContext(t.Context(), "ALTER TABLE `"+s.name+"` CONVERT TO CHARACTER SET utf8mb4 COLLATE "+collation); e != nil {
					t.Fatal("owned known original collation fixture failed")
				}
				definition := h.query("SHOW CREATE TABLE `" + s.name + "`")
				if len(definition) != 1 || definition[0][1].String != originalDDL(i, collation) {
					t.Fatal("actual allowed SHOW CREATE differs from closed original qualifier")
				}
			}
			before, modern, head, identity := h.originalRaw(), h.retainedRaw(), h.mongoHead(), h.independentBinding()
			h.capture()
			h.upgradePrefix(len(scope), true)
			h.restoreAndVerify(before, modern, head, identity)
		})
	}
}
