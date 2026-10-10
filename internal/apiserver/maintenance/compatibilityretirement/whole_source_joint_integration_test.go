//go:build integration

package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func wholeJointNativeFixture(t *testing.T, omitGenerated bool) (*gorm.DB, *mongo.Client, *mongo.Database, MongoOwnerConfig, authFixture, []event.DomainEvent) {
	t.Helper()
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	generated, _ := mongoLocalGeneratedFixture(t, db)
	mongoBatchNativeIndexes(t, db)
	sheet := mongoBatchNativeAssessmentSheet()
	insertMongoLocalSheet(t, db, sheet)
	submitted, err := mongoSubmissionPayload(sheet)
	if err != nil {
		t.Fatal(err)
	}
	at := sheet.FilledAt
	requested := eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, QuestionnaireCode: "Q", QuestionnaireVer: "1.0", AnswerSheetID: "10042", ModelKind: "scale", ModelCode: "M", ModelVersion: "1.0", RequestedAt: at}
	committed := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: "whole-joint-original-outcome", EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: at}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:1", CommittedAt: at}}
	mongoEvents := []event.DomainEvent{mongoBatchNativeEvent(t, submitted, "answersheet.submitted", "whole-joint-original-sheet"), mongoBatchNativeEvent(t, generated, "interpretation.report.generated", "whole-joint-original-generated")}
	selected := mongoEvents
	if omitGenerated {
		selected = mongoEvents[:1]
	}
	f := coordinatorNativeFixtureCopies(t, []event.DomainEvent{mongoBatchNativeEvent(t, requested, "evaluation.requested", "whole-joint-original-requested"), committed}, selected)
	for i, evt := range mongoEvents {
		suffix := "sheet"
		if i == 1 {
			suffix = "generated"
		}
		crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", suffix, nil)
	}
	return sqlDB, client, db, config, f, mongoEvents
}

func TestWholeSourceJointNativeSixTypesActualRetryLifecycleRetainsMissingSourceRun(t *testing.T) {
	sqlDB, client, db, config, _, mongoEvents := wholeJointNativeFixture(t, false)
	at := mongoLocalSheet().FilledAt
	failedAt := at.Add(time.Second)
	retryAt := failedAt.Add(time.Millisecond)
	evaluatedAt := at.Add(2 * time.Second)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"UPDATE runtime_checkpoint SET status='failed',finished_at=?,retry_disposition='automatic',retry_event_id='whole-joint-original-retry' WHERE scope='evaluation_run' AND resource_id='42:1'", []any{failedAt}},
		{"INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,attempt_origin,started_at,finished_at) VALUES('evaluation_run','42:2',2,42,'succeeded','automatic',?,?)", []any{retryAt, evaluatedAt}},
		{"UPDATE evaluation_outcome SET evaluation_run_id='42:2',evaluated_at=? WHERE id=9001", []any{evaluatedAt}},
		{"UPDATE assessment SET evaluated_at=? WHERE id=42", []any{evaluatedAt}},
		{"DELETE FROM retry_event_hold", nil},
	} {
		if err := sqlDB.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Collection("interpret_report_artifacts").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(20)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "generated_at", Value: evaluatedAt}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("interpretation_runs").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(11)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "started_at", Value: evaluatedAt}, {Key: "finished_at", Value: evaluatedAt}}}}); err != nil {
		t.Fatal(err)
	}
	generated := mongoEvents[1].(event.Event[eventoutcome.ReportGeneratedPayload])
	generated.Data.GeneratedAt = evaluatedAt
	generated.OccurredAtValue = evaluatedAt.Add(time.Millisecond)
	mongoEvents[1] = generated
	requested := eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, QuestionnaireCode: "Q", QuestionnaireVer: "1.0", AnswerSheetID: "10042", ModelKind: "scale", ModelCode: "M", ModelVersion: "1.0", RequestedAt: at}
	retry := requested
	retry.ExpectedAttempt = 1
	retry.Mode = "next_attempt"
	retry.AttemptOrigin = "automatic"
	retry.RequestedAt = retryAt
	failed := eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, Reason: "synthetic original terminal failure before retry", FailedAt: failedAt}
	committed := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: "whole-joint-original-outcome", EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: evaluatedAt.Add(time.Millisecond)}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:2", CommittedAt: evaluatedAt}}
	retryEvent := mongoBatchNativeEvent(t, retry, "evaluation.retry.requested", "whole-joint-original-retry").(event.Event[eventpayload.EvaluationRequestedData])
	retryEvent.OccurredAtValue = retryAt.Add(time.Millisecond)
	failedEvent := mongoBatchNativeEvent(t, failed, "evaluation.failed", "whole-joint-original-failed").(event.Event[eventpayload.EvaluationFailedData])
	failedEvent.OccurredAtValue = failedAt.Add(time.Millisecond)
	f := coordinatorNativeFixtureCopies(t, []event.DomainEvent{mongoBatchNativeEvent(t, requested, "evaluation.requested", "whole-joint-original-requested"), retryEvent, failedEvent, committed}, mongoEvents)
	for _, evt := range mongoEvents {
		crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", evt.EventType(), nil)
	}
	c, index := wholeJointNativeCoordinator(t, f, 128)
	page, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sources, err := page.Events()
	if err != nil {
		t.Fatal(err)
	}
	err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
		joint, e := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, sqlBatch, mongoBatch.global)
		if e != nil {
			return e
		}
		for _, source := range sources {
			v, e := joint.Candidate(ctx, source)
			if e != nil {
				return e
			}
			if !v.LocalQualified || len(v.BlockingReasons) != 0 {
				t.Logf("native_six_type=%s blocking_categories=%v", v.EventType, v.BlockingReasons)
				return ErrWholeSourceJoint
			}
			switch v.EventType {
			case "evaluation.requested", "evaluation.failed":
				if v.ActualOriginalRun != nil || len(v.OriginalRunMissing) == 0 {
					return ErrWholeSourceJoint
				}
			case "evaluation.retry.requested":
				if v.ActualOriginalRun != nil || v.AuthorizationRun == nil || v.AuthorizationRun.RunID != "42:1" || v.ExecutionRun == nil || v.ExecutionRun.RunID != "42:2" {
					return ErrWholeSourceJoint
				}
			case "evaluation.outcome.committed":
				if v.ActualOriginalRun == nil || v.ActualOriginalRun.RunID != "42:2" {
					return ErrWholeSourceJoint
				}
			}
		}
		return c.QualifyWholeSourceJointPage(ctx, page, joint)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.NextPage(t.Context()); err != io.EOF {
		t.Fatal(err)
	}
	if c.Receipt().ConsumedRecords != [4]uint64{4, 0, 0, 2} || c.Receipt().DropReady {
		t.Fatal("six-source local retry facts became destructive authorization")
	}
	t.Log("six_types_local_business_and_responsibility_qualified=true failed_requested_run_remain_missing=true retry_authorization_and_execution_are_separate=true historical_anchor_gaps_retained=true drop_ready=false")
}

func wholeJointNativeCoordinator(t *testing.T, f authFixture, records int) (*HistoricalCoordinator, *WholeSourceJointIndex) {
	t.Helper()
	limits := DefaultHistoricalCoordinatorLimits()
	limits.MaxPageRecords = records
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
	if err != nil {
		t.Fatal(err)
	}
	x, err := c.PrepareWholeSourceJointIndex(t.Context(), wholeJointCopies(f), DefaultWholeSourceJointLimits())
	if err != nil {
		t.Fatal(err)
	}
	return c, x
}

func TestWholeSourceJointNativeCompatibleHistoricalVersionsQualifyWithoutCurrentSDKRows(t *testing.T) {
	sqlDB, client, db, config, f, _ := wholeJointNativeFixture(t, false)
	c, index := wholeJointNativeCoordinator(t, f, 2)
	var journals []*WholeSourceJointPage
	for {
		page, err := c.NextPage(t.Context())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		sources, err := page.Events()
		if err != nil {
			t.Fatal(err)
		}
		err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
			joint, err := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, sqlBatch, mongoBatch.global)
			if err != nil {
				return err
			}
			if len(joint.sources) != 4 || len(joint.resolved) != 2 || len(joint.bindings) != 2 {
				t.Logf("native_joint_related_sources=%d resolved=%d bindings=%d", len(joint.sources), len(joint.resolved), len(joint.bindings))
				for _, q := range joint.cross.qualification {
					l := q.Local()
					t.Logf("native_joint_mongo_type=%s terminal=%t block_categories=%v", l.EventType, l.OwnerLocalTerminal, l.BlockingReasons)
				}
				return ErrWholeSourceJoint
			}
			for _, handle := range joint.sources {
				candidate, err := joint.Candidate(ctx, handle)
				if err != nil {
					return err
				}
				if !candidate.LocalQualified || len(candidate.BlockingReasons) != 0 {
					t.Logf("native_local_type=%s blocking_categories=%v", candidate.EventType, candidate.BlockingReasons)
					return ErrWholeSourceJoint
				}
				if candidate.EventType == "evaluation.requested" && candidate.ActualOriginalRun != nil {
					return ErrWholeSourceJoint
				}
			}
			if joint.Summary().WholeFourSourceCoverageBound || joint.Summary().DropReady {
				return ErrWholeSourceJoint
			}
			if err = c.QualifyWholeSourceJointPage(ctx, page, joint); err != nil {
				return err
			}
			if err = c.QualifyWholeSourceJointPage(ctx, page, joint); err == nil {
				return ErrWholeSourceJoint
			}
			journals = append(journals, joint)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	values, err := c.CandidateRange(0, 128)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 4 || len(journals) != 2 || c.Receipt().ConsumedRecords != [4]uint64{2, 0, 0, 2} || c.Receipt().DropReady || c.Receipt().CASComplete {
		t.Fatal("whole source responsibility was confused with production cleanup authorization")
	}
	for _, candidate := range values {
		if !candidate.LocalQualified || len(candidate.BlockingReasons) != 0 || len(candidate.RequiredAdapters) == 0 {
			t.Fatal("compatible original versions did not share exact closed responsibility")
		}
	}
	for _, journal := range journals {
		s := journal.Summary()
		if !s.WholeFourSourceCoverageBound || s.ConsumedOriginalEvents != 2 || s.ResolvedOriginalObservations != 2 || s.DropReady || !s.FinalFreshRequired || !s.ExternalOriginRequired || !s.AIInboxRequired || !s.WriterFenceRequired || !s.CASRequired {
			t.Fatal("final source receipt/gates lost")
		}
		if _, err = json.Marshal(journal); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("payload-bearing private joint reader serialized", err)
		}
	}
	t.Log("compatible_original_versions=4 exact_provisional_rows=2 current_sdk_rows=0 original_requested_run_missing=true drop_ready=false")
}

func TestWholeSourceJointNativeCachedGettersIssueNoSQLOrMongoReads(t *testing.T) {
	sqlDB, client, db, config, f, _ := wholeJointNativeFixture(t, false)
	if err := db.RunCommand(t.Context(), bson.D{{Key: "profile", Value: 2}}).Err(); err != nil {
		t.Fatal(err)
	}
	c, index := wholeJointNativeCoordinator(t, f, 128)
	page, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sources, err := page.Events()
	if err != nil {
		t.Fatal(err)
	}
	err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
		joint, err := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, sqlBatch, mongoBatch.global)
		if err != nil {
			return err
		}
		filter := bson.D{{Key: "ns", Value: bson.D{{Key: "$in", Value: bson.A{db.Name() + ".answersheets", db.Name() + ".report_generations", db.Name() + ".interpret_report_artifacts", db.Name() + ".interpretation_runs", db.Name() + ".rm_outbox", db.Name() + ".qs_rm_replay_requests"}}}}}
		before, err := db.Collection("system.profile").CountDocuments(t.Context(), filter)
		if err != nil {
			return err
		}
		tx, err := mysql.RequireTx(ctx)
		if err != nil {
			return err
		}
		var beforeSQL, afterSQL uint64
		var metric string
		if err = tx.Raw("SHOW SESSION STATUS LIKE 'Com_select'").Row().Scan(&metric, &beforeSQL); err != nil {
			return err
		}
		for i := 0; i < 10000; i++ {
			candidate, e := joint.Candidate(ctx, sources[i%len(sources)])
			if e != nil {
				return e
			}
			if !candidate.LocalQualified || len(candidate.BlockingReasons) != 0 {
				return ErrWholeSourceJoint
			}
			candidate.RequiredAdapters[0] = "edited defensive copy"
			if candidate.ActualOriginalRun != nil {
				candidate.ActualOriginalRun.RunID = "edited"
			}
		}
		if err = tx.Raw("SHOW SESSION STATUS LIKE 'Com_select'").Row().Scan(&metric, &afterSQL); err != nil {
			return err
		}
		after, err := db.Collection("system.profile").CountDocuments(t.Context(), filter)
		if err != nil {
			return err
		}
		if before != after || beforeSQL != afterSQL {
			return ErrWholeSourceJoint
		}
		t.Logf("joint_cached_getters=10000 new_sql_selects=%d new_mongo_business_or_ledger_reads=%d related_sources=%d", afterSQL-beforeSQL, after-before, len(joint.sources))
		return joint.ValidateBorrowedSnapshot(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWholeSourceJointNativeUncoveredConflictPendingAndOrphansRemainBlocked(t *testing.T) {
	for _, kind := range []string{"uncovered_original_source", "unknown_original_run", "conflicting_original_run", "cross_org_wire", "same_id_other_type", "current_pending", "current_lease", "global_orphan", "authorized_replay_without_current_claim"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB, client, db, config, f, events := wholeJointNativeFixture(t, kind == "uncovered_original_source")
			var err error
			switch kind {
			case "unknown_original_run":
				_, err = db.Collection("interpretation_runs").DeleteOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(11)}})
			case "conflicting_original_run":
				_, err = db.Collection("interpret_report_artifacts").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(20)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "interpretation_run_id", Value: int64(12)}}}})
			case "cross_org_wire":
				err = sqlDB.Exec("UPDATE retry_event_hold SET payload_json=REPLACE(payload_json,'\"org_id\":7','\"org_id\":8') WHERE event_id=?", events[1].EventID()).Error
			case "same_id_other_type":
				err = sqlDB.Exec("UPDATE retry_event_hold SET payload_json=REPLACE(payload_json,'interpretation.report.generated','answersheet.submitted') WHERE event_id=?", events[1].EventID()).Error
			case "current_pending":
				err = sqlDB.Exec("UPDATE retry_event_hold SET status='held',replayed_at=NULL WHERE event_id=?", events[1].EventID()).Error
			case "current_lease":
				err = sqlDB.Exec("UPDATE runtime_checkpoint SET lease_expires_at=DATE_ADD(UTC_TIMESTAMP(3),INTERVAL 1 HOUR) WHERE scope='evaluation_run' AND resource_id='42:1'").Error
			case "global_orphan":
				row := mongoLocalSheet()
				row.DomainID = 10043
				body, e := mongoSubmissionPayload(row)
				if e != nil {
					t.Fatal(e)
				}
				_, err = db.Collection("rm_outbox").InsertOne(t.Context(), mongoLocalStandardFixture(t, body, "whole-joint-unbound-current", "published"))
			case "authorized_replay_without_current_claim":
				crossMongoNativeReplay(t, sqlDB, events[1].EventID(), "mongo-domain-events", true, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			c, index := wholeJointNativeCoordinator(t, f, 128)
			page, err := c.NextPage(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			sources, err := page.Events()
			if err != nil {
				t.Fatal(err)
			}
			blocked := false
			err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
				joint, e := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, sqlBatch, mongoBatch.global)
				if e != nil {
					blocked = true
					return nil
				}
				for _, source := range sources {
					candidate, e := joint.Candidate(ctx, source)
					if e != nil {
						blocked = true
						continue
					}
					if !candidate.LocalQualified || len(candidate.BlockingReasons) != 0 {
						blocked = true
					}
				}
				if kind == "uncovered_original_source" && len(joint.resolved) > 0 {
					t.Fatal("uncovered related original graph allowed a provisional proof")
				}
				return nil
			})
			if err != nil {
				blocked = true
			}
			if !blocked {
				t.Fatal("uncovered/conflicting/unclosed original obligation became qualified")
			}
		})
	}
}

func TestWholeSourceJointNativeFreshDualEpochAndFullSourceEOF(t *testing.T) {
	for _, kind := range []string{"unchanged", "same_epoch", "mongo_business_changed", "sql_business_changed", "new_responsibility", "source_changed"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB, client, db, config, f, events := wholeJointNativeFixture(t, false)
			c, index := wholeJointNativeCoordinator(t, f, 128)
			page, err := c.NextPage(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			sources, err := page.Events()
			if err != nil {
				t.Fatal(err)
			}
			var baseline *WholeSourceJointPage
			err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
				var e error
				baseline, e = c.PrepareWholeSourceJointPage(ctx, page, index, catalog, sqlBatch, mongoBatch.global)
				if e != nil {
					return e
				}
				if kind == "same_epoch" {
					if e = baseline.RecheckFresh(ctx, wholeJointCopies(f), catalog, sqlBatch, mongoBatch.global); e == nil {
						return ErrWholeSourceJoint
					}
					return nil
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "same_epoch" {
				return
			}
			switch kind {
			case "mongo_business_changed":
				_, err = db.Collection("report_generations").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(1)}}, bson.D{{Key: "$inc", Value: bson.D{{Key: "version", Value: int64(1)}}}})
			case "sql_business_changed":
				err = sqlDB.Exec("UPDATE assessment SET version=version+1 WHERE id=42").Error
			case "new_responsibility":
				crossMongoNativeWire(t, sqlDB, events[1], "held", "replayed", "additional", nil)
			case "source_changed":
				f.raw[3][len(f.raw[3])-2] ^= 1
			}
			if err != nil {
				t.Fatal(err)
			}
			err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
				return baseline.RecheckFresh(ctx, wholeJointCopies(f), catalog, sqlBatch, mongoBatch.global)
			})
			if kind == "unchanged" {
				if err != nil {
					t.Fatal("actual fresh epoch rejected unchanged originals", err)
				}
			} else if err == nil {
				t.Fatal("actual new epoch or source mutation undetected")
			}
		})
	}
}

func TestWholeSourceJointNativeRejectWrongPageIndexSnapshotAndBinding(t *testing.T) {
	sqlDB, client, db, config, f, _ := wholeJointNativeFixture(t, false)
	c, index := wholeJointNativeCoordinator(t, f, 128)
	page, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sources, err := page.Events()
	if err != nil {
		t.Fatal(err)
	}
	err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
		joint, e := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, sqlBatch, mongoBatch.global)
		if e != nil {
			return e
		}
		fake := &WholeSourceJointPage{owner: c, page: page, index: index, sql: sqlBatch, mongo: mongoBatch}
		if e = c.QualifyWholeSourceJointPage(ctx, page, fake); e == nil {
			return ErrWholeSourceJoint
		}
		other, _, _ := coordinatorCrossNativeFirst(t, f)
		if e = other.QualifyWholeSourceJointPage(ctx, page, joint); e == nil {
			return ErrWholeSourceJoint
		}
		if _, e = joint.Candidate(context.Background(), sources[0]); e == nil {
			return ErrWholeSourceJoint
		}
		values, e := joint.Candidate(ctx, sources[0])
		if e != nil {
			return e
		}
		if values.ActualOriginalRun != nil && strings.Contains(values.ActualOriginalRun.RunID, "edited") {
			return ErrWholeSourceJoint
		}
		limits := index.limits
		index.limits.MaxRelatedSources = 1
		if _, e = c.PrepareWholeSourceJointPage(ctx, page, index, catalog, sqlBatch, mongoBatch.global); !errors.Is(e, ErrWholeSourceJointBounds) {
			return ErrWholeSourceJoint
		}
		index.limits = limits
		return c.QualifyWholeSourceJointPage(ctx, page, joint)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.CandidateRange(0, 128); !errors.Is(err, ErrCoordinatorIncomplete) {
		t.Fatal("source coverage exposed before own new EOF")
	}
	if _, err = c.NextPage(t.Context()); err != io.EOF {
		t.Fatal(err)
	}
	if c.Receipt().DropReady {
		t.Fatal("local qualification authorized destructive action")
	}
	t.Logf("actual_epoch_bound=true four_source_eof_bound=true receipt_time=%s drop_ready=false", time.Now().UTC().Format(time.RFC3339))
}

// The original page carries two unrelated native owners. It is consumed once;
// its pure fragments are split by the actual owner/replay reads, never by page.
func TestWholeSourceJointNativeOwnerComponentsSplitPageWithoutConsumingTwice(t *testing.T) {
	for _, tc := range []struct {
		name            string
		present         bool
		pageSize, pages int
		records         [4]uint64
	}{
		{"two_actual_assessments", true, 3, 2, [4]uint64{3, 0, 0, 2}},
		{"actual_assessment_and_empty_sheet", false, 128, 1, [4]uint64{2, 0, 0, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, client, db, config, _, mongoEvents := wholeJointNativeFixture(t, false)
			mongoCASNativeHistoryIndexes(t, db)
			at := mongoLocalSheet().FilledAt
			if tc.present {
				if err := sqlDB.Exec("INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,evaluation_model_kind,evaluation_model_algorithm,evaluation_model_code,evaluation_model_version,created_at,updated_at,submitted_at,version) SELECT 43,org_id,testee_id,questionnaire_code,questionnaire_version,10043,origin_type,'submitted',evaluation_model_kind,evaluation_model_algorithm,evaluation_model_code,evaluation_model_version,created_at,updated_at,submitted_at,version FROM assessment WHERE id=42").Error; err != nil {
					t.Fatal(err)
				}
			}
			secondSheet := mongoBatchNativeAssessmentSheet()
			if !tc.present {
				// A positive independent-questionnaire purpose and actual native
				// SQL absence are both required; absence never invents a purpose.
				secondSheet = mongoLocalSheet()
			}
			secondSheet.ID = primitive.NewObjectID()
			secondSheet.DomainID = meta.FromUint64(10043)
			insertMongoLocalSheet(t, db, secondSheet)
			var sqlEvents []event.DomainEvent
			// Reuse the fixture's real event contracts, while the two original SQL
			// source records remain authenticated by the existing original encoders.
			requested := eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, QuestionnaireCode: "Q", QuestionnaireVer: "1.0", AnswerSheetID: "10042", ModelKind: "scale", ModelCode: "M", ModelVersion: "1.0", RequestedAt: at}
			committed := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: "whole-joint-original-outcome", EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: at}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:1", CommittedAt: at}}
			second := requested
			second.AssessmentID = 43
			second.AnswerSheetID = "10043"
			sqlEvents = append(sqlEvents, mongoBatchNativeEvent(t, requested, "evaluation.requested", "whole-joint-original-requested"), committed)
			if tc.present {
				sqlEvents = append(sqlEvents, mongoBatchNativeEvent(t, second, "evaluation.requested", "whole-joint-second-owner-requested"))
			} else {
				submitted, err := mongoSubmissionPayload(secondSheet)
				if err != nil {
					t.Fatal(err)
				}
				secondSource := mongoBatchNativeEvent(t, submitted, "answersheet.submitted", "whole-joint-actual-empty-sheet")
				mongoEvents = append(mongoEvents, secondSource)
				crossMongoNativeWire(t, sqlDB, secondSource, "held", "replayed", "actual-empty-sheet", nil)
			}
			copies := coordinatorNativeFixtureCopies(t, sqlEvents, mongoEvents)
			c, index := wholeJointNativeCoordinator(t, copies, tc.pageSize)
			file, err := os.CreateTemp(t.TempDir(), "owner-recipes-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if file.Close() != nil {
					t.Error("owned spool close")
				}
			})
			if file.Chmod(0o600) != nil {
				t.Fatal("owned spool mode")
			}
			spool, err := sqlevaluation.NewSQLHistoricalCASSpool(file, 64<<20, 16<<20)
			if err != nil {
				t.Fatal(err)
			}
			var inputs []*HistoricalCASComponentInput
			pageCount := 0
			for {
				page, err := c.NextPage(t.Context())
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				sources, err := page.Events()
				if err != nil {
					t.Fatal(err)
				}
				err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, batch *SQLBusinessOwnerBatch, globalBatch *MongoHistoricalOwnerBatch) error {
					joint, e := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, batch, globalBatch.global)
					if e != nil {
						return e
					}
					baseline, e := sqlevaluation.SealSQLHistoricalCASReadBaseline(ctx, joint.sql.facts)
					if e != nil {
						return e
					}
					var attachments []MongoHistoricalBatchAttachment
					for _, handle := range joint.sources {
						facts, e := handle.Facts()
						if e != nil {
							return e
						}
						if facts.Source.Database == "mongodb" {
							attachments = append(attachments, MongoHistoricalBatchAttachment{Source: handle, Entry: mongoCASNativeEntry(t, ctx, joint.mongo, handle)})
						}
					}
					mongoPlan, e := PrepareMongoHistoricalBatchCAS(ctx, joint.mongo, c.authenticated, attachments)
					if e != nil {
						return e
					}
					fragments, e := FreezeHistoricalCASOwnerComponentInputs(ctx, joint, nil, baseline, mongoPlan, spool)
					if e != nil {
						return e
					}
					if pageCount == 0 && (len(fragments) != 2 || fragments[0].partitions != 2 || fragments[0].sequence != fragments[1].sequence) {
						return ErrHistoricalCASComponents
					}
					if !tc.present {
						found := false
						for _, fragment := range fragments {
							scope, e := fragment.sqlRecipe.OriginalSelectors()
							if e != nil {
								return e
							}
							for _, owner := range scope.MongoOwners {
								if owner.Kind == "AnswerSheet" && owner.ID == "10043" && len(scope.AssessmentIDs) == 0 && len(scope.EventIDs) == 1 && scope.EventIDs[0] == "whole-joint-actual-empty-sheet" {
									found = true
								}
							}
						}
						if !found {
							return ErrHistoricalCASComponents
						}
					}
					if joint.consumed || page.consumed {
						return ErrCoordinatorPage
					}
					for _, f := range fragments {
						if f.seal != f.digest() || f.mongoRead == nil || len(f.sources) == 0 {
							return ErrHistoricalCASComponents
						}
					}
					inputs = append(inputs, fragments...)
					if e = c.QualifyWholeSourceJointPage(ctx, page, joint); e != nil {
						return e
					}
					if c.QualifyWholeSourceJointPage(ctx, page, joint) == nil {
						return ErrCoordinatorPage
					}
					return nil
				})
				if err != nil {
					t.Fatal("real owner fragment factory: ", err)
				}
				pageCount++
			}
			components, err := PrepareHistoricalCASComponents(t.Context(), index, inputs, DefaultHistoricalCASComponentLimits())
			if err != nil || pageCount != tc.pages || len(components.Components()) != 2 || c.Receipt().ConsumedRecords != tc.records || c.Receipt().DropReady {
				t.Fatal("complete original owner partitions: ", err)
			}
			t.Log("actual_two_owners_same_page_split=2 original_joint_consumed_once=true related_sources_and_negative_ranges_retained=true cas_authority=false drop_ready=false")
		})
	}
}
