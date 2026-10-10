//go:build integration

package retirement

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// The existing inspected fixture host owns random SQL99/Mongo38 namespaces.
// No synthetic external Q, writer fence, Window or production approval is used.
// This covers the event-only final native adapter; qs-ai Q remains separate.
func TestFinalHistoricalEOFNativePersistedOriginalAndMissingConflictSourceDrift(t *testing.T) {
	for _, mode := range []string{"persisted", "missing", "evidence_conflict", "source_delete"} {
		t.Run(mode, func(t *testing.T) {
			sqlDB, _, db, config, session := qualifiedCASNativeFixture(t)
			var copies authFixture
			var plan *sqlevaluation.SQLHistoricalBatchCASPlan
			err := qualifiedCASNativeFresh(t, sqlDB, db, config, session, nil, func(ctx context.Context, p *persistenceJointNativePage, origin *SourceOriginRecheckProof, ai *AIReverseFreshProof) error {
				var mongoPlan *MongoHistoricalBatchCASPlan
				var sealed *HistoricalCASPersistencePage
				var e error
				plan, mongoPlan, sealed, e = p.coordinator.PrepareQualifiedHistoricalCAS(ctx, p.joint, origin, ai)
				if e != nil || plan == nil || mongoPlan != nil || sealed == nil {
					return errors.New("actual factory did not produce event-only plan")
				}
				return nil
			})
			if err != nil {
				t.Fatal("actual original two-epoch qualification", err)
			}
			// Capture physical frozen source bytes before any drift. They stay
			// separate from the current business evidence and returned report.
			err = persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, _ *SQLResponsibilitySnapshot, g *MongoResponsibilitySnapshot, tx *gorm.DB) error {
				copies = originNativeCopies(t, ctx, tx, g)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if mode != "missing" {
				err = sqlDB.Transaction(func(tx *gorm.DB) error { _, e := plan.Apply(hostmysql.WithTx(t.Context(), tx)); return e }, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
				if err != nil {
					t.Fatal("actual fixture-only evidence CAS/commit", err)
				}
			}
			if mode == "evidence_conflict" {
				v := sqlDB.Exec("UPDATE evaluation_outcome SET historical_committed_evidence=JSON_SET(historical_committed_evidence,'$.entries[0].proof.business_binding_sha256',?) WHERE id=9001", sourceSHA([]byte("different")))
				if v.Error != nil || v.RowsAffected != 1 {
					t.Fatal("actual evidence mutation ineffective")
				}
			}
			if mode == "source_delete" {
				v := sqlDB.Exec("DELETE FROM domain_event_outbox WHERE id=1")
				if v.Error != nil || v.RowsAffected != 1 {
					t.Fatal("owned original deletion ineffective")
				}
			}
			var identity string
			var observed *FinalHistoricalObservation
			err = persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, s *SQLResponsibilitySnapshot, _ *MongoResponsibilitySnapshot, _ *gorm.DB) error {
				identity = s.Report().DatabaseIdentitySHA256
				var e error
				observed, e = PrepareFinalHistoricalEOF(ctx, FinalHistoricalInput{Binding: coordinatorBinding(), SQLIdentity: identity, SQLHead: 99, MongoDatabase: db, MongoConfig: config, Copies: copies.inputs()})
				if mode == "persisted" && e == nil {
					r := observed.Summary()
					if r.EventReferences != 1 || r.AICommands != 0 || r.SourceReferences != 1 || r.DropReady || r.CASAuthority || r.WholeWriterFence || !evidenceHash(r.PersistedReferencesSHA256) {
						return errors.New("final observation altered proof scope")
					}
					return observed.ValidateBorrowedSnapshot(ctx)
				}
				return e
			})
			if mode == "persisted" {
				if err != nil || observed == nil {
					t.Fatal("actual final persisted readback failed", err)
				}
				if observed.ValidateBorrowedSnapshot(t.Context()) == nil {
					t.Fatal("ended native epochs remained valid")
				}
			} else if err == nil || observed != nil {
				t.Fatal("missing/conflict/deleted original falsely completed")
			}
			var n int
			if sqlDB.Raw("SELECT COUNT(*) FROM evaluation_outcome WHERE id=9001 AND committed_event_id IS NULL AND committed_event_evidence IS NULL").Row().Scan(&n) != nil || n != 1 {
				t.Fatal("final RO observation changed current standard facts")
			}
		})
	}
}
