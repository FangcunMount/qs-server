//go:build integration

package main

import (
	"context"
	"reflect"
	"testing"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
)

// Both epochs use the real CLI host, complete SQL99/Mongo38 migrations,
// authenticated source files and actual borrowed RR-RO/snapshot scopes. Empty
// AI source copies must not hide admission drift behind zero candidate counts.
func TestHistoryCLINativeEmptyAIAdmissionDrift(t *testing.T) {
	for _, change := range []string{"stable", "revision", "closed"} {
		t.Run(change, func(t *testing.T) {
			testSource(t)
			pool, client, database, _ := nativeFixture(t, false)
			_, _, inputs := nativeInputs(t, pool, client, database)
			host, err := openDatabases(t.Context(), inputs)
			if err != nil {
				t.Fatal("actual authenticated CLI host open failed: " + safeCategory(err))
			}
			defer func() {
				if host.close() != nil {
					t.Error("owned CLI host close failed")
				}
			}()

			var first, second *epochResult
			var firstContext context.Context
			var firstSQL *retirement.SQLResponsibilitySnapshot
			if err = host.epoch(t.Context(), func(scope context.Context) error {
				firstContext = scope
				var buildErr error
				first, buildErr = buildEpoch(scope, inputs, host)
				if buildErr != nil {
					return buildErr
				}
				firstSQL = first.sql
				return first.compactOrigin(scope)
			}); err != nil {
				t.Fatal("first actual empty-source epoch failed: " + safeCategory(err))
			}
			if firstSQL == nil || firstSQL.ValidateBorrowedSnapshot(firstContext) == nil {
				t.Fatal("first actual SQL scope remains active between epochs")
			}
			if first.coordinator.CandidateCount != 0 || first.aiPages != 0 {
				t.Fatal("empty source emitted AI candidates")
			}
			if first.aiFacts.CASAuthority || first.aiFacts.DropReady || first.aiFacts.ExternalClosure != "unknown" || first.aiFacts.GlobalReverseCoverage != "unknown" {
				t.Fatal("local empty-source observation overstated closure")
			}

			var statement string
			switch change {
			case "revision":
				statement = "UPDATE ai_messaging_admission SET revision=revision+1 WHERE singleton=1"
			case "closed":
				statement = "UPDATE ai_messaging_admission SET closed=NOT closed,revision=revision+1 WHERE singleton=1"
			}
			if statement != "" {
				result, updateErr := pool.ExecContext(t.Context(), statement)
				if updateErr != nil {
					t.Fatal("owned admission mutation failed")
				}
				if count, countErr := result.RowsAffected(); countErr != nil || count != 1 {
					t.Fatal("owned admission singleton not changed exactly once")
				}
			}

			if err = host.epoch(t.Context(), func(scope context.Context) error {
				var buildErr error
				second, buildErr = buildEpoch(scope, inputs, host)
				return buildErr
			}); err != nil {
				t.Fatal("second actual empty-source epoch failed: " + safeCategory(err))
			}
			if second.coordinator.CandidateCount != 0 || second.aiPages != 0 || first.coordinator.CandidateSHA256 != second.coordinator.CandidateSHA256 || first.coordinator.ApprovedCopies != second.coordinator.ApprovedCopies || first.coordinator.SecondPassCopies != second.coordinator.SecondPassCopies || first.index.IndexSHA256 != second.index.IndexSHA256 || !reflect.DeepEqual(first.sqlFacts, second.sqlFacts) || !reflect.DeepEqual(first.mongoFacts, second.mongoFacts) || !reflect.DeepEqual(first.reasons, second.reasons) {
				t.Fatal("admission-only fixture changed unrelated source or business facts")
			}
			comparison := compareEpochs(first, second)
			if change == "stable" {
				if first.aiFacts != second.aiFacts || comparison != nil {
					t.Fatal("unchanged actual empty-source epochs rejected")
				}
				return
			}
			if first.aiFacts == second.aiFacts || safeCategory(comparison) != "history_independent_epoch_facts_changed" {
				t.Fatal("actual empty AI source hid admission drift")
			}
			if second.aiFacts.AdmissionRevision != first.aiFacts.AdmissionRevision+1 || (change == "revision" && second.aiFacts.AdmissionClosed != first.aiFacts.AdmissionClosed) || (change == "closed" && second.aiFacts.AdmissionClosed == first.aiFacts.AdmissionClosed) || second.aiFacts.AdmissionRowSHA256 == first.aiFacts.AdmissionRowSHA256 {
				t.Fatal("fresh actual admission facts do not match isolated mutation")
			}
		})
	}
}
