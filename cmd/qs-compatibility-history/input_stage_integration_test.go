//go:build integration

package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/sys/unix"
)

func TestHistoryCLINativeInitialTwoSnapshotInputRounds(t *testing.T) {
	for _, nonempty := range []bool{false, true} {
		t.Run(strconv.FormatBool(nonempty), func(t *testing.T) {
			testSource(t)
			pool, client, db, _ := nativeFixture(t, nonempty)
			_, _, a := nativeInputs(t, pool, client, db)
			if nonempty {
				wire := []byte("synthetic-unbound-cli-input")
				if _, e := pool.ExecContext(t.Context(), "INSERT INTO ai_messaging_quarantine(wire_sha256,wire,code,attempts,first_seen_at,last_seen_at) VALUES(?,?,?,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", rawHash(wire), wire, "synthetic_unknown"); e != nil {
					t.Fatal("owned unknown AI fixture")
				}
			}
			host, err := openDatabases(t.Context(), a)
			if err != nil {
				t.Fatal(safeCategory(err))
			}
			defer func() {
				if host.close() != nil {
					t.Error("host close")
				}
			}()
			// This separately probes the same host scope: actual driver state must
			// be snapshot-only, and Mongo must reject transaction creation.
			err = host.snapshotInputScope(t.Context(), func(ctx context.Context) error {
				s := mongo.SessionFromContext(ctx)
				x, ok := s.(mongo.XSession) //nolint:staticcheck // pinned native mode, not imported facts
				if !ok || !x.ClientSession().Snapshot || x.ClientSession().TransactionRunning() || s.StartTransaction() == nil {
					t.Fatal("host opened a Mongo transaction input scope")
				}
				return nil
			})
			if err != nil {
				t.Fatal(safeCategory(err))
			}
			j, err := newHistoryWriteJournal(filepath.Join(privateTestDir(t), "input"), a)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if j.dir.Close() != nil {
					t.Error("journal close")
				}
			}()
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			input, err := captureHistoryInitialInputs(t.Context(), a, host, j)
			if err != nil {
				t.Fatal("actual initial input capture: " + safeCategory(err))
			}
			defer func() {
				if input.close() != nil {
					t.Error("input close")
				}
			}()
			if input.sources.ValidateFrozen(t.Context()) != nil || input.ai.ValidateFrozen(t.Context()) != nil || !input.sources.Summary().TwoIndependentInputsMatched || !input.ai.Summary().TwoIndependentInputsMatched || input.ai.Summary().CASAuthority || input.sources.Summary().DropReady {
				t.Fatal("native input pair forged authorization or lost original bytes")
			}
			if input.components == nil || input.components.ValidateInputSources(t.Context(), input.sources) != nil || input.ownerSpool == nil || nonempty && len(input.components.Components()) == 0 || !nonempty && len(input.components.Components()) != 0 {
				t.Fatal("actual stopped input owner recipes missing or incomplete")
			}
			for round := range 2 {
				s := input.sourceEpochs[round].Summary()
				if !s.CaptureStopped || s.LiveSQLGraphRetained || len(input.sqlFacts[round].Ledgers) != 8 || len(input.mongoFacts[round].Collections) != 11 || len(input.aiEpochs[round].Summary().Ledgers) != 14 || !s.CompleteInput || input.sourceEpochs[round].StopCapture(t.Context()) == nil {
					t.Fatal("capture graph retained or stopped scope reused")
				}
			}
			if nonempty && input.sources.Summary().Sources[3].Records != 1 {
				t.Fatal("original submitted source skipped")
			}
			if nonempty && (input.ai.Summary().Unknown == 0 || input.ai.Summary().Blocking == 0) {
				t.Fatal("actual unknown AI responsibility was erased from pure input")
			}
			if !nonempty && input.sources.Summary().Sources[3].Records != 0 {
				t.Fatal("empty source fabricated")
			}
			runtime.GC()
			runtime.ReadMemStats(&after)
			var usage unix.Rusage
			if unix.Getrusage(unix.RUSAGE_SELF, &usage) != nil {
				t.Fatal("actual RSS unavailable")
			}
			t.Logf("actual_initial_input_rounds=2 sql8_rounds=2 mongo11_rounds=2 native_four_source_rounds=2 ai14_rounds=2 input_files=6 owner_spool_files=1 owner_components=%d retained_sql_graphs=0 heap_before_bytes=%d heap_after_gc_bytes=%d actual_process_peak_rss_platform_units=%d", len(input.components.Components()), before.HeapAlloc, after.HeapAlloc, usage.Maxrss)
			if input.close() != nil {
				t.Fatal("actual input file close")
			}
			if len(j.created) != 10 || j.sequence != 3 {
				t.Fatal("input/journal original members changed")
			}
			if hash, e := j.snapshotMaterials(t.Context()); e != nil || hash == "" {
				t.Fatal("original material handoff: " + safeCategory(e))
			}
		})
	}
}
