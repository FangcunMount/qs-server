//go:build integration

package retirement

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// The actual existing native host owns both SQL and Mongo lifecycles. This
// fixture authenticates one original SQL Outcome plus three truly empty old
// copies. It is deliberately not proof of Mongo Apply, six-type retirement,
// external qs-ai closure, a production writer fence or DROP qualification.
func qualifiedCASNativeFixture(t *testing.T) (*gorm.DB, *mongo.Client, *mongo.Database, MongoOwnerConfig, mongo.Session) {
	t.Helper()
	sqlDB, client, db, config := originNativeDBs(t, true)
	mongoBatchNativeIndexes(t, db)
	at := mongoLocalSheet().FilledAt
	evt := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: "native-qualified-original-outcome", EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: at}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:1", CommittedAt: at}}
	body, err := domainwire.EncodeEvent(evt)
	if err != nil {
		t.Fatal("original domain encoder fixture rejected")
	}
	row := fixtureSQLRow(t, body, "1")
	row[5] = []byte("7")
	originNativeInsertSQL(t, sqlDB, "domain_event_outbox", row)
	session, err := client.StartSession()
	if err != nil {
		t.Fatal("actual borrowed Mongo session unavailable")
	}
	t.Cleanup(func() { session.EndSession(context.Background()) })
	return sqlDB, client, db, config, session
}

// Existing helpers perform the full actual source scans, whole four-copy EOF,
// private original-business/joint qualification and SQL8/Mongo/AI14 scans. Only
// a graphless first proof survives its genuinely ended scopes. No report is
// imported and no EventEvidence is supplied by this new native helper.
func qualifiedCASNativeFresh(t *testing.T, sqlDB *gorm.DB, db *mongo.Database, config MongoOwnerConfig, session mongo.Session, afterFirst func() error, use func(context.Context, *persistenceJointNativePage, *SourceOriginRecheckProof, *AIReverseFreshProof) error) error {
	t.Helper()
	var copies authFixture
	var anchor *AIReverseRecheckAnchor
	err := persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
		copies = originNativeCopies(t, ctx, tx, global)
		page, err := persistenceJointNativePreparePage(t, ctx, current, global, copies)
		if err != nil {
			return err
		}
		anchor, err = page.ai.FreezeFreshAnchor(ctx, page.origin)
		return err
	})
	if err != nil {
		return err
	}
	if afterFirst != nil {
		if err = afterFirst(); err != nil {
			return err
		}
	}
	return persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
		page, err := persistenceJointNativePreparePage(t, ctx, current, global, copies)
		if err != nil {
			return err
		}
		freshAI, err := anchor.RecheckFresh(ctx, current, 99, page.coordinator, copies.inputs())
		if err != nil {
			return err
		}
		freshOrigin, err := anchor.RecheckOrigin(ctx, freshAI, current, global, page.coordinator, originTestReaders(copies))
		if err != nil {
			return err
		}
		return use(ctx, page, freshOrigin, freshAI)
	})
}

func TestQualifiedHistoricalCASNativeRealFactoryApplyCommitAndReadback(t *testing.T) {
	sqlDB, _, db, config, session := qualifiedCASNativeFixture(t)
	var plan *sqlevaluation.SQLHistoricalBatchCASPlan
	var sealed *HistoricalCASPersistencePage
	var source *VerifiedSourceEvent
	err := qualifiedCASNativeFresh(t, sqlDB, db, config, session, nil, func(ctx context.Context, page *persistenceJointNativePage, origin *SourceOriginRecheckProof, ai *AIReverseFreshProof) error {
		var mongoPlan *MongoHistoricalBatchCASPlan
		var err error
		plan, mongoPlan, sealed, err = page.coordinator.PrepareQualifiedHistoricalCAS(ctx, page.joint, origin, ai)
		if err != nil {
			return err
		}
		if plan == nil || mongoPlan != nil || sealed == nil || sealed.sql == nil {
			return ErrCoordinatorCASQualification
		}
		attachments, err := sealed.sql.Attachments()
		if err != nil || len(attachments) != 1 {
			return ErrCoordinatorCASQualification
		}
		facts, err := page.source.Facts()
		if err != nil {
			return err
		}
		entry := attachments[0].Entry
		if entry.Validate() != nil || entry.EventID != facts.EventID || entry.Source != facts.Source || entry.Proof.Digest != facts.Source.Digest || attachments[0].ContentDigest != facts.ContentDigest || !evidence.ValidSHA256(entry.Proof.BusinessBindingSHA256) || entry.Run == nil || entry.Run.RunID != "42:1" || entry.Run.Attempt != 1 || entry.Proof.Verification.Method != "actual-whole-source-joint-fresh-origin-ai14" {
			return ErrCoordinatorCASQualification
		}
		// Actual scoped components remain alive after factory construction.
		if err = page.joint.ValidateBorrowedSnapshot(ctx); err != nil {
			return err
		}
		source = page.source
		return nil
	})
	if err != nil || plan == nil || sealed == nil || source == nil {
		t.Fatal("genuine two-epoch factory failed to prepare the actual bounded plan")
	}
	var applied *HistoricalCASAppliedPage
	// Only this native TEST HOST starts/commits the actual writer. Factory
	// preparation did not write, finish scopes or claim production approval.
	err = sqlDB.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		statement, err := plan.Apply(ctx)
		if err != nil {
			return err
		}
		applied, err = sealed.BindApplied(ctx, statement, nil)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil || applied == nil {
		t.Fatal("actual host-owned SQL apply/commit did not complete")
	}
	observed, err := persistenceJointNativeReadback(t, sqlDB, db, config, session, source, applied)
	if err != nil {
		t.Fatal("genuine independent third epoch did not observe the factory evidence")
	}
	persistenceJointNativeLimitedReport(t, observed)
	var preserved int
	err = sqlDB.Raw("SELECT COUNT(*) FROM evaluation_outcome WHERE id=9001 AND committed_event_id IS NULL AND committed_event_evidence IS NULL AND JSON_LENGTH(JSON_EXTRACT(historical_committed_evidence,'$.entries'))=1").Row().Scan(&preserved)
	if err != nil || preserved != 1 {
		t.Fatal("factory evidence changed the standard single slot or source reference count")
	}
}

func TestQualifiedHistoricalCASNativeActualProofAndOwnerRejections(t *testing.T) {
	for _, mode := range []string{"copied_proof", "mismatched_source_handle", "cross_organization_actual_drift", "retained_original_run_unknown_status"} {
		t.Run(mode, func(t *testing.T) {
			sqlDB, _, db, config, session := qualifiedCASNativeFixture(t)
			var afterFirst func() error
			if mode == "cross_organization_actual_drift" || mode == "retained_original_run_unknown_status" {
				afterFirst = func() error {
					statement := "UPDATE assessment SET org_id=8 WHERE id=42"
					if mode == "retained_original_run_unknown_status" {
						// Retained unknown is a conflict, not an absent historical
						// Run. No latest Run or nil-Run absence rule may hide it.
						statement = "UPDATE runtime_checkpoint SET status='unsupported' WHERE scope='evaluation_run' AND resource_id='42:1'"
					}
					result := sqlDB.Exec(statement)
					if result.Error != nil || result.RowsAffected != 1 {
						return errors.New("actual owner or original Run drift fixture ineffective")
					}
					return nil
				}
			}
			called := false
			err := qualifiedCASNativeFresh(t, sqlDB, db, config, session, afterFirst, func(ctx context.Context, page *persistenceJointNativePage, origin *SourceOriginRecheckProof, ai *AIReverseFreshProof) error {
				called = true
				switch mode {
				case "copied_proof":
					copy := *ai
					ai = &copy // genuine fields cannot copy the self identity
				case "mismatched_source_handle":
					original := page.joint.current[0]
					page.joint.current[0] = &VerifiedSourceEvent{}
					defer func() { page.joint.current[0] = original }()
				default:
					return errors.New("contradictory actual graph obtained fresh qualification")
				}
				plan, mongoPlan, sealed, rejected := page.coordinator.PrepareQualifiedHistoricalCAS(ctx, page.joint, origin, ai)
				if plan != nil || mongoPlan != nil || sealed != nil || rejected == nil {
					return errors.New("false or mismatched private proof minted a plan")
				}
				if mode == "copied_proof" && !errors.Is(rejected, ErrCoordinatorCASQualification) {
					return errors.New("copied proof rejection category changed")
				}
				return page.joint.ValidateBorrowedSnapshot(ctx)
			})
			if afterFirst == nil {
				if err != nil || !called {
					t.Fatal("actual proofs were not exercised or rejection damaged the borrowed scope")
				}
			} else if err == nil || called {
				t.Fatal("cross-org or retained unknown Run acquired a genuine fresh capability")
			}
			var written int
			if err = sqlDB.Raw("SELECT COUNT(*) FROM evaluation_outcome WHERE historical_committed_evidence IS NOT NULL").Row().Scan(&written); err != nil || written != 0 {
				t.Fatal("rejected native qualification left historical evidence writes")
			}
		})
	}
}
