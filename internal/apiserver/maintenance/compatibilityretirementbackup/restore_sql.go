package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"time"
)

// RestoreSQL requires an exclusive host-owned connection to an empty isolated
// restore namespace, outside any business transaction: MySQL DDL auto-commits.
// It never opens/closes a pool or starts/commits/rolls back a borrowed transaction.
// The original FK session setting is restored on every path. Missing non-target
// parents remain absent; preserving their FK definition is not business closure.
func restoreSQL(ctx context.Context, conn *sql.Conn, a *Archive) (v Verification, result error) {
	v = Verification{Database: "mysql", StartedAt: time.Now().UTC(), SourceOriginAuthentication: "host_binding_required", ForeignKeyBusinessClosure: "not_proven", Isolation: "host_runtime_inspection_required"}
	defer func() {
		v.FinishedAt = time.Now().UTC()
		v.ElapsedMillis = v.FinishedAt.Sub(v.StartedAt).Milliseconds()
		if v.ElapsedMillis > MaxRestoreSeconds*1000 && result == nil {
			result = ErrBudget
			v.ContentEqual = false
			v.SchemaEqual = false
		}
	}()
	if conn == nil || a == nil {
		return v, ErrRestore
	}
	q, c, e := boundedRestore(ctx)
	if e != nil {
		return v, e
	}
	defer c()
	if e = a.verifyAssets(q); e != nil {
		return v, e
	}
	ids, e := readSQL(q, conn, "SELECT @@server_uuid,DATABASE(),VERSION()")
	if e != nil || len(ids) != 1 || !strings.HasPrefix(cell(ids[0], 2), "8.") {
		return v, ErrIdentity
	}
	v.RestoreIdentityHash = parts("mysql_database_identity_v1", cell(ids[0], 0), cell(ids[0], 1))
	if v.RestoreIdentityHash == a.data.Inventory.Bindings["mysql"].IdentityHash || cell(ids[0], 1) == "" {
		return v, ErrIsolation
	}
	tables, e := readSQL(q, conn, "SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema=DATABASE()")
	if e != nil || len(tables) != 0 {
		return v, ErrIsolation
	}
	extra, e := readSQL(q, conn, "SELECT (SELECT COUNT(*) FROM information_schema.routines WHERE ROUTINE_SCHEMA=DATABASE()),(SELECT COUNT(*) FROM information_schema.events WHERE EVENT_SCHEMA=DATABASE())")
	if e != nil || len(extra) != 1 || cell(extra[0], 0) != "0" || cell(extra[0], 1) != "0" {
		return v, ErrIsolation
	}
	settings, e := readSQL(q, conn, "SELECT @@session.foreign_key_checks,@@session.autocommit")
	if e != nil || len(settings) != 1 || (cell(settings[0], 0) != "0" && cell(settings[0], 0) != "1") || cell(settings[0], 1) != "1" {
		return v, ErrRestore
	}
	original := cell(settings[0], 0)
	if _, e = conn.ExecContext(q, "SET SESSION FOREIGN_KEY_CHECKS=0"); e != nil {
		return v, ErrRestore
	}
	defer func() {
		cleanup, cc := context.WithTimeout(context.Background(), 5*time.Second)
		defer cc()
		if _, e := conn.ExecContext(cleanup, "SET SESSION FOREIGN_KEY_CHECKS="+original); e != nil {
			result = ErrRestore
			v.ContentEqual = false
			v.SchemaEqual = false
		}
		state, e := readSQL(cleanup, conn, "SELECT @@session.foreign_key_checks")
		if e != nil || len(state) != 1 || cell(state[0], 0) != original {
			result = ErrRestore
			v.ContentEqual = false
			v.SchemaEqual = false
		}
	}()
	for i, structure := range a.data.SQL {
		if !strings.HasPrefix(structure.DDL, "CREATE TABLE "+quote(targetNames[i])+" (") {
			return v, ErrStructure
		}
		if _, e = conn.ExecContext(q, structure.DDL); e != nil {
			return v, ErrRestore
		}
		metrics, err := loadSQLRows(q, conn, a, i)
		if err != nil {
			return v, err
		}
		v.Targets = append(v.Targets, metrics)
	}
	tables, e = readSQL(q, conn, "SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema=DATABASE() ORDER BY TABLE_NAME")
	if e != nil || len(tables) != 3 {
		return v, ErrIsolation
	}
	for i := 0; i < 3; i++ {
		structure, e := sqlStructure(q, conn, i)
		if e != nil || !sqlStructuresEqual(structure, a.data.SQL[i]) {
			return v, ErrStructure
		}
		v.OriginalSQLDDLHashes = append(v.OriginalSQLDDLHashes, sha([]byte(a.data.SQL[i].DDL)))
		v.RestoredSQLDDLHashes = append(v.RestoredSQLDDLHashes, sha([]byte(structure.DDL)))
		v.SQLSemanticHashes = append(v.SQLSemanticHashes, jsonSHA(struct {
			DDL           string
			Columns       any
			CharacterSets any
		}{normalizedSQLDDL(structure), structure.Columns, structure.CharacterSets}))
		v.SQLShowCreateEnvironmentSHA256 = jsonSHA(structure.ShowCreateEnvironment)
		if e = verifySQLContent(q, conn, structure, a.data.Inventory.Targets[i], i); e != nil {
			return v, e
		}
		// The full database reread measured the same count, raw bytes and framed
		// content digest; these numbers are qualified only by that actual read.
		v.Targets[i].RestoredRawBytes = v.Targets[i].SourceRawBytes
	}
	if q.Err() != nil {
		return v, ErrBudget
	}
	v.ArchiveSHA256 = a.digest
	v.TargetCount = 3
	v.ContentEqual = true
	v.SchemaEqual = true
	v.SQLStructureSemantics = "exact_columns_charsets_keys_fk_constraints_options_verified_redundant_charset_display_v1"
	return v, nil
}
func loadSQLRows(ctx context.Context, conn *sql.Conn, a *Archive, index int) (metrics TargetLoadMetrics, result error) {
	metrics.Database = "mysql"
	metrics.Name = targetNames[index]
	metrics.SourceFileBytes = a.data.Assets[index].Bytes
	f, r, e := a.openSource(ctx, index)
	if e != nil {
		return metrics, e
	}
	defer func() {
		if f.Close() != nil && result == nil {
			result = ErrPrivate
		}
	}()
	columns := make([]string, len(a.data.SQL[index].Columns))
	for i, c := range a.data.SQL[index].Columns {
		columns[i] = quote(*c[0])
	}
	tuple := "(" + strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",") + ")"
	prefix := "INSERT INTO " + quote(targetNames[index]) + " (" + strings.Join(columns, ",") + ") VALUES "
	// Bound each actual server insert independently; no original body is saved
	// in an error or print statement, including server/driver diagnostics.
	for {
		var args []any
		tuples := []string{}
		size := 0
		for len(tuples) < 250 && size < 1<<20 {
			row, e := r.next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return metrics, e
			}
			for i, b := range row.cells {
				if row.nulls[i] {
					args = append(args, nil)
				} else {
					args = append(args, b)
					size += len(b)
				}
			}
			tuples = append(tuples, tuple)
		}
		if len(tuples) == 0 {
			break
		}
		actual, e := conn.ExecContext(ctx, prefix+strings.Join(tuples, ","), args...)
		if e != nil {
			return metrics, ErrRestore
		}
		affected, e := actual.RowsAffected()
		if e != nil || affected != int64(len(tuples)) {
			return metrics, ErrRestore
		}
		metrics.RestoredRecords += uint64(affected)
		metrics.InsertStatements++
		metrics.AutocommitInsertBatches++
		if uint64(len(tuples)) > metrics.MaxBatchRecords {
			metrics.MaxBatchRecords = uint64(len(tuples))
		}
		if uint64(size) > metrics.MaxBatchRawBytes {
			metrics.MaxBatchRawBytes = uint64(size)
		}
		if r.done {
			break
		}
	}
	if !r.done {
		return metrics, ErrSource
	}
	metrics.SourceRecords = r.records
	metrics.SourceRawBytes = r.size
	if metrics.RestoredRecords != r.records {
		return metrics, ErrContent
	}
	return metrics, nil
}
