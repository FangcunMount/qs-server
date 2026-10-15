//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// This diagnostic has its own strict selector. The historical 53 and runtime
// generation 12 remain separate gates, with their source and names unchanged.
func TestM4MaintenanceTablesReadOnlyInspectNative(t *testing.T) {
	for _, name := range []string{"Clean102", "Clean103", "Committed102", "Committed103", "Dirty103", "Unknown104", "ForeignBoundary", "HiddenBoundary"} {
		t.Run(name, func(t *testing.T) {
			h := newMaintenanceNative(t)
			version := uint64(102)
			committed := strings.HasPrefix(name, "Committed") || name == "HiddenBoundary"
			if committed {
				h.capture()
			}
			if name == "Clean103" || name == "Committed103" || name == "Dirty103" || name == "Unknown104" {
				h.migrateSQL(103)
				version = 103
			}
			if committed {
				mode := "cancel102"
				if version == 103 {
					mode = "target103"
				}
				h.assertRuntimeMarker(h.action("mark-first-runtime", h.runtimeArgs(mode, "", "")...))
			}
			switch name {
			case "Dirty103":
				if _, e := h.db.ExecContext(t.Context(), "UPDATE schema_migrations SET dirty=TRUE"); e != nil {
					t.Fatal("owned dirty-state negative fixture unavailable")
				}
			case "Unknown104":
				if _, e := h.db.ExecContext(t.Context(), "UPDATE schema_migrations SET version=104"); e != nil {
					t.Fatal("owned unknown-state negative fixture unavailable")
				}
			case "ForeignBoundary":
				if _, e := h.db.ExecContext(t.Context(), "CREATE TABLE m4_d_runtime_boundary (foreign_id BIGINT PRIMARY KEY)"); e != nil {
					t.Fatal("owned foreign-boundary negative fixture unavailable")
				}
			}
			before, head, binding := h.inspectionSQLWitness(), h.mongoHead(), h.independentBinding()
			entriesBefore, e := inspectionDirectoryWitness(h.dir)
			if e != nil {
				t.Fatal(e)
			}
			reader := h.inspectionReader(name == "HiddenBoundary")
			r := reader.action("inspect")
			h.assertRestoreReleased(r)
			failure := ""
			switch name {
			case "Dirty103", "Unknown104":
				failure = "inspection_requires_clean102_or103"
			case "ForeignBoundary":
				failure = "runtime_database_boundary_schema_rejected"
			case "HiddenBoundary":
				failure = "runtime_database_boundary_unknown"
			}
			if failure == "" {
				if !r.Complete || r.Error != "none" || r.InspectedSQLVersion == nil || *r.InspectedSQLVersion != version || !r.SQLSnapshot || r.DatabaseBoundary == nil || (r.DatabaseBoundaryRecord != nil) != committed {
					t.Fatal("actual clean inspection did not prove its version and current record", r)
				}
				if emitReceipt(r, badOutput{}) != 1 {
					t.Fatal("inspection stdout delivery failure became success")
				}
			} else if r.Complete || r.Error != failure || r.InspectedSQLVersion != nil {
				t.Fatal("unsafe inspection was admitted", r)
			}
			if !r.ReadOnly || r.ApprovalConsumed || r.RuntimeGateAcquired || r.RuntimeGateReleased || r.MarkerWritten || r.MarkerWriteUnknown || r.DatabaseBoundaryAdvanced || r.DatabaseBoundaryAdvanceUnknown || r.ProductionFreezeCertified || r.CrossDatabaseAtomicRR {
				t.Fatal("diagnostic asserted runtime, recovery or cross-database authority", r)
			}
			entriesAfter, e := inspectionDirectoryWitness(h.dir)
			if e != nil || !reflect.DeepEqual(entriesBefore, entriesAfter) || before != h.inspectionSQLWitness() || !bytes.Equal(head, h.mongoHead()) || binding != h.independentBinding() {
				t.Fatal("inspection changed owned database facts or created a local file")
			}
		})
	}
}

func inspectionDirectoryWitness(path string) (map[string]string, error) {
	entries, e := os.ReadDir(path)
	if e != nil {
		return nil, e
	}
	out := map[string]string{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return nil, errors.New("unexpected owned inspection-directory object")
		}
		raw, e := os.ReadFile(path + string(os.PathSeparator) + entry.Name())
		if e != nil {
			return nil, e
		}
		out[entry.Name()] = fingerprint(raw)
	}
	return out, nil
}

func (h *maintenanceNative) inspectionReader(hideBoundary bool) *maintenanceNative {
	h.t.Helper()
	user := "qs_inspect_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	password := "owned_disposable_inspect_only"
	if _, e := h.root.ExecContext(h.t.Context(), "CREATE USER '"+user+"'@'%' IDENTIFIED BY '"+password+"'"); e != nil {
		h.t.Fatal("owned read-only account unavailable")
	}
	h.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
		defer cancel()
		if _, e := h.root.ExecContext(ctx, "DROP USER '"+user+"'@'%'"); e != nil {
			h.t.Error("owned read-only account not released")
		}
	})
	if hideBoundary {
		for _, row := range h.query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() ORDER BY BINARY TABLE_NAME") {
			if row[0].String == runtimeBoundaryTable {
				continue
			}
			if _, e := h.root.ExecContext(h.t.Context(), "GRANT SELECT ON `"+h.name+"`.`"+row[0].String+"` TO '"+user+"'@'%'"); e != nil {
				h.t.Fatal("owned limited account grant unavailable")
			}
		}
	} else if _, e := h.root.ExecContext(h.t.Context(), "GRANT SELECT ON `"+h.name+"`.* TO '"+user+"'@'%'"); e != nil {
		h.t.Fatal("owned read-only grant unavailable")
	}
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil {
		h.t.Fatal("owned selected DSN unavailable")
	}
	cfg.User, cfg.Passwd = user, password
	reader := *h
	reader.dsn = cfg.FormatDSN()
	pool, e := sql.Open("mysql", reader.dsn)
	if e != nil {
		h.t.Fatal("owned read-only pool unavailable")
	}
	defer func() {
		if pool.Close() != nil || pool.Stats().OpenConnections != 0 {
			h.t.Error("owned read-only probe pool release unknown")
		}
	}()
	for _, statement := range []string{"UPDATE schema_migrations SET dirty=dirty", "CREATE TABLE inspection_mutation_forbidden (id BIGINT)"} {
		_, e := pool.ExecContext(h.t.Context(), statement)
		var server *mysql.MySQLError
		if !errors.As(e, &server) || server.Number != 1142 {
			h.t.Fatal("actual SELECT-only account did not reject write privilege", e)
		}
	}
	return &reader
}

// This independent witness reads every owned SQL table, DDL and raw row rather
// than trusting the tool's database_read_only flag. Mongo's actual ledger bytes
// and UUID/cluster binding are compared separately; its owned fixture has no
// authentication enforcement and is not claimed to prove a read-only Mongo user.
func (h *maintenanceNative) inspectionSQLWitness() string {
	h.t.Helper()
	var records []string
	for _, row := range h.query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() ORDER BY BINARY TABLE_NAME") {
		name := row[0].String
		if !regexp.MustCompile(`^[a-z][a-z0-9_]+$`).MatchString(name) {
			h.t.Fatal("owned table name outside the fixed SQL fixture")
		}
		ddl, e := json.Marshal(h.query("SHOW CREATE TABLE `" + name + "`"))
		if e != nil {
			h.t.Fatal(e)
		}
		var values []string
		for _, cells := range h.query("SELECT * FROM `" + name + "`") {
			original := make([]cell, len(cells))
			for i, value := range cells {
				original[i].SQLNull = !value.Valid
				if value.Valid {
					original[i].Bytes = []byte(value.String)
				}
			}
			raw, e := json.Marshal(original)
			if e != nil {
				h.t.Fatal(e)
			}
			values = append(values, string(raw))
		}
		sort.Strings(values)
		records = append(records, name, string(ddl), strings.Join(values, "\n"))
	}
	raw, e := json.Marshal(records)
	if e != nil {
		h.t.Fatal(e)
	}
	return fingerprint(raw)
}
