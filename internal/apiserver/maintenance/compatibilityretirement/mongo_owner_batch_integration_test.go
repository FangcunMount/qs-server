//go:build integration

package retirement

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

func mongoBatchNativeIndexes(t *testing.T, db *mongo.Database) {
	t.Helper()
	for _, name := range mongoBatchBusinessCollections {
		if _, err := db.Collection(name).Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("native_domain_unique").SetUnique(true).SetCollation(&options.Collation{Locale: "simple"})}); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []struct {
		name, field, index string
		unique             bool
	}{{"report_generations", "outcome_id", "native_outcome", false}, {"interpret_report_artifacts", "outcome_id", "native_outcome", false}, {"interpret_report_artifacts", "generation_id", "uk_artifact_generation_id", true}, {"interpretation_runs", "generation_id", "native_generation", false}} {
		if _, err := db.Collection(v.name).Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: v.field, Value: 1}}, Options: options.Index().SetName(v.index).SetUnique(v.unique).SetCollation(&options.Collation{Locale: "simple"})}); err != nil {
			t.Fatal(err)
		}
	}
}

func mongoBatchNativeEvent(t *testing.T, p any, kind, id string) event.DomainEvent {
	t.Helper()
	base := event.BaseEvent{ID: id, EventTypeValue: kind, OccurredAtValue: mongoLocalSheet().FilledAt.Add(time.Second)}
	switch value := p.(type) {
	case eventpayload.AnswerSheetSubmittedData:
		base.AggregateTypeValue = "AnswerSheet"
		base.AggregateIDValue = value.AnswerSheetID
		return event.Event[eventpayload.AnswerSheetSubmittedData]{BaseEvent: base, Data: value}
	case eventoutcome.ReportGeneratedPayload:
		base.AggregateTypeValue = "ReportGeneration"
		base.AggregateIDValue = value.GenerationID
		return event.Event[eventoutcome.ReportGeneratedPayload]{BaseEvent: base, Data: value}
	case eventpayload.EvaluationRequestedData:
		base.AggregateTypeValue = "Evaluation"
		base.AggregateIDValue = strconv.FormatInt(value.AssessmentID, 10)
		return event.Event[eventpayload.EvaluationRequestedData]{BaseEvent: base, Data: value}
	case eventpayload.EvaluationFailedData:
		base.AggregateTypeValue = "Evaluation"
		base.AggregateIDValue = strconv.FormatInt(value.AssessmentID, 10)
		return event.Event[eventpayload.EvaluationFailedData]{BaseEvent: base, Data: value}
	}
	t.Fatal("unsupported producer")
	return nil
}

// Actual producer bytes go through all four complete-copy authenticators to
// clean EOF. Expectations are explicit test-preparer approvals; this is not a
// claim of production source origin or a business-close proof.
func mongoBatchNativeSources(t *testing.T, events ...event.DomainEvent) []*VerifiedSourceEvent {
	t.Helper()
	var f authFixture
	sqlRows := [][][]byte{fixtureSQLRow(t, wireFixture(t, "evaluation.failed"), "1")}
	var mongoRows [][]byte
	for i, evt := range events {
		body, err := domainwire.EncodeEvent(evt)
		if err != nil {
			t.Fatal(err)
		}
		if evt.EventType() == "answersheet.submitted" || evt.EventType() == "interpretation.report.generated" {
			row := fixtureMongoRow(t, body, primitive.NewObjectID())
			row = setMongoField(row, "org_id", int64(7))
			raw, err := bson.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			mongoRows = append(mongoRows, raw)
		} else {
			row := fixtureSQLRow(t, body, strconv.Itoa(i+2))
			row[5] = []byte("7")
			sqlRows = append(sqlRows, row)
		}
	}
	f.raw[0], f.expected[0] = fixtureSQLCopy(t, sqlRows, nil)
	f.raw[1], f.expected[1] = aiFixtureCopy(t, AIBridgeCommandSource, nil, nil)
	f.raw[2], f.expected[2] = aiFixtureCopy(t, AILegacyCommandSource, nil, nil)
	f.raw[3], f.expected[3] = fixtureMongoCopy(t, mongoRows)
	approved, err := VerifySourceCopies(t.Context(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{}
	for _, evt := range events {
		wanted[evt.EventID()] = true
	}
	byID := map[string]*VerifiedSourceEvent{}
	sr, err := NewSQLSourceReader(bytes.NewReader(f.raw[0]), f.expected[0])
	if err != nil {
		t.Fatal(err)
	}
	for range sqlRows {
		source, err := sr.Next()
		if err != nil {
			t.Fatal(err)
		}
		if wanted[source.EventID] {
			handle, err := approved.BindEvent(source)
			if err != nil {
				t.Fatal(err)
			}
			byID[source.EventID] = handle
		}
	}
	mr, err := NewMongoSourceReader(bytes.NewReader(f.raw[3]), f.expected[3])
	if err != nil {
		t.Fatal(err)
	}
	for range mongoRows {
		source, err := mr.Next()
		if err != nil {
			t.Fatal(err)
		}
		handle, err := approved.BindEvent(source)
		if err != nil {
			t.Fatal(err)
		}
		byID[source.EventID] = handle
	}
	result := make([]*VerifiedSourceEvent, len(events))
	for i, evt := range events {
		result[i] = byID[evt.EventID()]
		if result[i] == nil {
			t.Fatal("verified event missing")
		}
	}
	return result
}

func mongoBatchNativePair(t *testing.T, sqlDB *gorm.DB, client *mongo.Client, db *mongo.Database, config MongoOwnerConfig, sources []*VerifiedSourceEvent, fn func(context.Context, *MongoHistoricalOwnerBatch, *MongoResponsibilitySnapshot, *sqlevaluation.SQLHistoricalOwnerBatch) error) error {
	t.Helper()
	var uuid, database string
	if err := sqlDB.Raw("SELECT @@server_uuid,DATABASE()").Row().Scan(&uuid, &database); err != nil {
		t.Fatal(err)
	}
	expected := mongoOwnerHashParts("mysql_database_identity_v1", uuid, database)
	return mongoCycleNativeTx(t, client, func(mongoCtx mongo.SessionContext) error {
		return sqlDB.Transaction(func(tx *gorm.DB) error {
			ctx := mongo.NewSessionContext(hostmysql.WithTx(mongoCtx, tx), mongo.SessionFromContext(mongoCtx))
			cycle, err := sqlevaluation.PrepareSQLHistoricalResponsibilityCycle(ctx, expected, sqlevaluation.DefaultSQLResponsibilityLimits())
			if err != nil {
				return err
			}
			request, err := MongoHistoricalSQLBatchSelectors(sources)
			if err != nil {
				return err
			}
			sqlBatch, err := sqlevaluation.PrepareSQLHistoricalOwnerBatch(ctx, cycle, request, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			global, err := PrepareMongoResponsibilitySnapshot(ctx, db, config, mongoCycleTestLimits())
			if err != nil {
				return err
			}
			batch, err := PrepareMongoHistoricalOwnerBatch(ctx, global, sqlBatch, sources, DefaultMongoHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			if err = fn(ctx, batch, global, sqlBatch); err != nil {
				return err
			}
			return batch.ValidateBorrowedSnapshot(ctx)
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	})
}

func mongoBatchNativeAssessmentSheet() sheetmongo.AnswerSheetPO {
	row := mongoLocalSheet()
	row.Admission.Purpose = "assessment"
	row.Admission.ModelKind = "scale"
	row.Admission.ModelAlgorithm = string(modelcatalog.AlgorithmScaleDefault)
	row.Admission.ModelCode = "M"
	row.Admission.ModelVersion = "1.0"
	row.Admission.ModelTitle = "Model"
	return row
}

func TestMongoBatchNativeActualPairedRRROAndCachedLookups(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	mongoBatchNativeIndexes(t, db)
	row := mongoBatchNativeAssessmentSheet()
	insertMongoLocalSheet(t, db, row)
	p, err := mongoSubmissionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	sources := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, p, "answersheet.submitted", "batch-original-sheet"))
	if err = db.RunCommand(t.Context(), bson.D{{Key: "profile", Value: 2}}).Err(); err != nil {
		t.Fatal(err)
	}
	if err = mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
		profileFilter := bson.D{{Key: "ns", Value: bson.D{{Key: "$in", Value: bson.A{db.Name() + ".answersheets", db.Name() + ".report_generations", db.Name() + ".interpret_report_artifacts", db.Name() + ".interpretation_runs", db.Name() + ".rm_outbox", db.Name() + ".qs_rm_replay_requests"}}}}}
		before, err := db.Collection("system.profile").CountDocuments(t.Context(), profileFilter)
		if err != nil {
			return err
		}
		tx, _ := hostmysql.TxFromContext(ctx)
		var beforeSQL uint64
		var metric string
		if err = tx.Raw("SHOW SESSION STATUS LIKE 'Com_select'").Row().Scan(&metric, &beforeSQL); err != nil || metric != "Com_select" {
			return err
		}
		for i := 0; i < 10000; i++ {
			q, e := b.ResolveSource(ctx, sources[0])
			if e != nil {
				return e
			}
			v := q.Local()
			if !v.OwnerLocalTerminal || v.AssessmentID != 42 || v.SQLSubmissionClock == nil || v.SQLSubmissionClock.ExactBusinessMilliseconds || !containsString(v.Gaps, "storage_precision_gap") {
				return ErrMongoBatchConflict
			}
			if _, e = b.ResponsibilitiesForSource(ctx, sources[0]); e != nil {
				return e
			}
		}
		var afterSQL uint64
		if err = tx.Raw("SHOW SESSION STATUS LIKE 'Com_select'").Row().Scan(&metric, &afterSQL); err != nil || metric != "Com_select" {
			return err
		}
		after, err := db.Collection("system.profile").CountDocuments(t.Context(), profileFilter)
		if err != nil {
			return err
		}
		if after != before || afterSQL != beforeSQL {
			return ErrMongoBatchConflict
		}
		r := b.Report()
		if r.Sources != 1 || r.UniqueBusinessRows != 1 || r.Queries != 3 || !r.SourceCopyFactsBound || !r.ExternalOriginAuthenticationRequired || r.DropReady {
			return ErrMongoBatchConflict
		}
		t.Logf("batch_cached_lookups=10000 mongo_business_or_rm_new_ops=%d sql_new_selects=%d queries=%d rows=%d drop_ready=false", after-before, afterSQL-beforeSQL, r.Queries, r.UniqueBusinessRows)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMongoBatchNativeOriginalGeneratedAndSharedBusinessVerifier(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	p, _ := mongoLocalGeneratedFixture(t, db)
	mongoBatchNativeIndexes(t, db)
	row := mongoBatchNativeAssessmentSheet()
	insertMongoLocalSheet(t, db, row)
	source := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, p, "interpretation.report.generated", "batch-original-generated"))
	if err := mongoBatchNativePair(t, sqlDB, client, db, config, source, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
		q, err := b.ResolveSource(ctx, source[0])
		if err != nil {
			return err
		}
		local := q.Local()
		if !local.OwnerLocalTerminal || local.OriginalRun == nil || local.OriginalRun.RunID != "11" || local.OriginalRun.Attempt != 1 || local.ReportID != 20 || local.OutcomeID != 9001 || local.BusinessBindingSHA256 == "" {
			return ErrMongoBatchConflict
		}
		if _, err = bson.Marshal(q); !errors.Is(err, ErrSourceSerialization) {
			return ErrMongoBatchConflict
		}
		// An Assessment-selected SQL capability cannot be upgraded to the
		// exclusive AnswerSheet association required by the submission path.
		sheetPayload, err := mongoSubmissionPayload(row)
		if err != nil {
			return err
		}
		sheetSource := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, sheetPayload, "answersheet.submitted", "batch-association-not-selected"))
		if upgraded, err := PrepareMongoHistoricalOwnerBatch(ctx, g, s, sheetSource, DefaultMongoHistoricalOwnerBatchLimits()); err == nil || upgraded != nil {
			return ErrMongoBatchConflict
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMongoBatchNativeGlobalResponsibilitiesUsedWithoutPointFallback(t *testing.T) {
	for _, kind := range []string{"linked_current_pending", "unrelated_current_pending"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoCycleNativeDB(t)
			mongoBatchNativeIndexes(t, db)
			row := mongoBatchNativeAssessmentSheet()
			insertMongoLocalSheet(t, db, row)
			p, err := mongoSubmissionPayload(row)
			if err != nil {
				t.Fatal(err)
			}
			sources := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, p, "answersheet.submitted", "batch-old-source"))
			messagePayload := p
			if kind == "unrelated_current_pending" {
				other := mongoLocalSheet()
				other.DomainID = meta.FromUint64(30042)
				insertMongoLocalSheet(t, db, other)
				messagePayload, err = mongoSubmissionPayload(other)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Collection("rm_outbox").InsertOne(t.Context(), mongoLocalStandardFixture(t, messagePayload, "batch-current-live-event", "pending")); err != nil {
				t.Fatal(err)
			}
			if err = mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
				q, err := b.ResolveSource(ctx, sources[0])
				if err != nil {
					return err
				}
				local := q.Local()
				if kind == "linked_current_pending" {
					if local.OwnerLocalTerminal || local.CurrentResponsibilityCount != 1 || len(local.BlockingReasons) == 0 {
						return ErrMongoBatchConflict
					}
				} else if !local.OwnerLocalTerminal || local.CurrentResponsibilityCount != 0 {
					return ErrMongoBatchConflict
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMongoBatchNativeIndependentRequiresPositiveAdmissionAndActualUniqueSQLAbsence(t *testing.T) {
	for _, kind := range []string{"independent_absent", "independent_sql_conflict", "assessment_sql_absent"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoCycleNativeDB(t)
			mongoBatchNativeIndexes(t, db)
			row := mongoLocalSheet()
			if kind != "independent_sql_conflict" {
				row.DomainID = meta.FromUint64(20042)
			}
			if kind == "assessment_sql_absent" {
				row = mongoBatchNativeAssessmentSheet()
				row.DomainID = meta.FromUint64(20042)
			}
			insertMongoLocalSheet(t, db, row)
			p, err := mongoSubmissionPayload(row)
			if err != nil {
				t.Fatal(err)
			}
			sources := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, p, "answersheet.submitted", "batch-independent-original"))
			err = mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
				q, err := b.ResolveSource(ctx, sources[0])
				if err != nil {
					return err
				}
				if !q.Local().OwnerLocalTerminal || !containsString(q.Local().Gaps, "independent_admission_unique_sql_absence_observed_external_coverage_required") {
					return ErrMongoBatchConflict
				}
				return nil
			})
			if (err == nil) != (kind == "independent_absent") {
				t.Fatal("positive Admission/actual SQL association requirement bypassed", err)
			}
		})
	}
}

func TestMongoBatchNativeMissingSourceLinksUseActualOutcomeReverseGraph(t *testing.T) {
	for _, kind := range []string{"evaluation.requested", "evaluation.failed"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoCycleNativeDB(t)
			p, _ := mongoLocalGeneratedFixture(t, db)
			mongoBatchNativeIndexes(t, db)
			row := mongoBatchNativeAssessmentSheet()
			insertMongoLocalSheet(t, db, row)
			if _, err := db.Collection("report_generations").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(1)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "generating"}}}}); err != nil {
				t.Fatal(err)
			}
			var body any
			if kind == "evaluation.requested" {
				body = eventpayload.EvaluationRequestedData{AssessmentID: 42, OrgID: 7, TesteeID: 21, AnswerSheetID: "10042", QuestionnaireCode: "Q", QuestionnaireVer: "1.0", ModelKind: "scale", ModelAlgorithm: string(modelcatalog.AlgorithmScaleDefault), ModelCode: "M", ModelVersion: "1.0", RequestedAt: mongoLocalSheet().FilledAt}
			} else {
				body = eventpayload.EvaluationFailedData{AssessmentID: 42, OrgID: 7, TesteeID: 21, FailedAt: mongoLocalSheet().FilledAt, Reason: "native_reason"}
			}
			sources := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, body, kind, "batch-old-unlinked-"+kind))
			if err := mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
				view, err := b.ResponsibilitiesForSource(ctx, sources[0])
				if err != nil {
					return err
				}
				found := false
				for _, o := range view.Observations {
					if o.Collection == "report_generations" && o.GenerationID == 1 && o.OutcomeID == 9001 {
						found = true
					}
				}
				if !found || len(view.BlockingReasons) == 0 {
					return ErrMongoBatchConflict
				}
				facts, err := sources[0].Facts()
				if err != nil {
					return err
				}
				if facts.OriginalRun.RunID != "" || facts.Generated != nil || p.RunID != "11" {
					return ErrMongoBatchConflict
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMongoBatchNativeFreshBusinessAndPairedSnapshotBoundaries(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	mongoBatchNativeIndexes(t, db)
	row := mongoBatchNativeAssessmentSheet()
	insertMongoLocalSheet(t, db, row)
	p, err := mongoSubmissionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	sources := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, p, "answersheet.submitted", "batch-fresh-original"))
	var baseline *MongoHistoricalOwnerBatch
	if err = mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
		baseline = b
		if err = b.RecheckBusiness(ctx, g, s); err != ErrMongoBatchInvalid {
			return ErrMongoBatchConflict
		}
		if _, err = b.ResolveSource(t.Context(), sources[0]); err == nil {
			return ErrMongoBatchConflict
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
		return baseline.RecheckBusiness(ctx, g, s)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Collection("answersheets").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(10042)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "questionnaire_title", Value: "actual_native_changed"}}}}); err != nil {
		t.Fatal(err)
	}
	if err = mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
		return baseline.RecheckBusiness(ctx, g, s)
	}); err != ErrMongoBatchConflict {
		t.Fatal("fresh raw business mutation accepted", err)
	}
}

func TestMongoBatchNativeIndexBudgetAndCrossOrganizationRefused(t *testing.T) {
	for _, kind := range []string{"missing_domain_index", "partial_domain_index", "rows_cap", "bytes_cap", "cross_org", "source_not_in_page"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoCycleNativeDB(t)
			mongoBatchNativeIndexes(t, db)
			row := mongoBatchNativeAssessmentSheet()
			insertMongoLocalSheet(t, db, row)
			p, err := mongoSubmissionPayload(row)
			if err != nil {
				t.Fatal(err)
			}
			sources := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, p, "answersheet.submitted", "batch-boundary-original"))
			if kind == "rows_cap" {
				second := mongoLocalSheet()
				second.DomainID = meta.FromUint64(20042)
				insertMongoLocalSheet(t, db, second)
				secondBody, e := mongoSubmissionPayload(second)
				if e != nil {
					t.Fatal(e)
				}
				sources = mongoBatchNativeSources(t, mongoBatchNativeEvent(t, p, "answersheet.submitted", "batch-boundary-original"), mongoBatchNativeEvent(t, secondBody, "answersheet.submitted", "batch-boundary-second"))
			}
			var expected bool
			if kind == "missing_domain_index" || kind == "partial_domain_index" {
				if _, err = db.Collection("answersheets").Indexes().DropOne(t.Context(), "native_domain_unique"); err != nil {
					t.Fatal(err)
				}
				if kind == "partial_domain_index" {
					if _, err = db.Collection("answersheets").Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("partial_only").SetPartialFilterExpression(bson.D{{Key: "domain_id", Value: bson.D{{Key: "$gt", Value: 0}}}})}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if kind == "cross_org" {
				if _, err = db.Collection("answersheets").UpdateOne(t.Context(), bson.D{{Key: "domain_id", Value: int64(10042)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "org_id", Value: int64(8)}}}}); err != nil {
					t.Fatal(err)
				}
			}
			err = mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
				switch kind {
				case "rows_cap", "bytes_cap":
					limits := DefaultMongoHistoricalOwnerBatchLimits()
					if kind == "rows_cap" {
						limits.MaxRows = 1
					} else {
						limits.MaxBytes = 1
					}
					partial, err := PrepareMongoHistoricalOwnerBatch(ctx, g, s, sources, limits)
					if err != ErrMongoBatchBounds || partial != nil {
						return ErrMongoBatchConflict
					}
					expected = true
					return nil
				case "source_not_in_page":
					source := mongoBatchNativeSources(t, mongoBatchNativeEvent(t, p, "answersheet.submitted", "not_in_batch"))
					_, err := b.ResolveSource(ctx, source[0])
					if err == nil {
						return ErrMongoBatchConflict
					}
					expected = true
					return nil
				default:
					_, err := b.ResolveSource(ctx, sources[0])
					return err
				}
			})
			if kind == "rows_cap" || kind == "bytes_cap" || kind == "source_not_in_page" {
				if err != nil || !expected {
					t.Fatal("explicit budget/source gate failed", err)
				}
			} else if err == nil {
				t.Fatal("unknown index/cross-org accepted")
			}
		})
	}
}

func TestMongoBatchNativeBatchedIndexedReadDoesNotDependOnUnrelatedPopulation(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	mongoBatchNativeIndexes(t, db)
	docs := make([]any, 4096)
	for i := range docs {
		row := mongoLocalSheet()
		row.DomainID = meta.FromUint64(uint64(20000 + i))
		docs[i] = row
	}
	if _, err := db.Collection("answersheets").InsertMany(t.Context(), docs); err != nil {
		t.Fatal(err)
	}
	row := mongoBatchNativeAssessmentSheet()
	insertMongoLocalSheet(t, db, row)
	p, err := mongoSubmissionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	var events []event.DomainEvent
	for i := 0; i < 32; i++ {
		events = append(events, mongoBatchNativeEvent(t, p, "answersheet.submitted", fmt.Sprintf("batch-index-original-%d", i)))
	}
	sources := mongoBatchNativeSources(t, events...)
	var explain struct {
		ExecutionStats struct{ TotalKeysExamined, TotalDocsExamined, NReturned int64 }
	}
	cmd := bson.D{{Key: "explain", Value: bson.D{{Key: "find", Value: "answersheets"}, {Key: "filter", Value: bson.D{{Key: "domain_id", Value: bson.D{{Key: "$in", Value: bson.A{int64(10042)}}}}}}, {Key: "hint", Value: "native_domain_unique"}, {Key: "sort", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "collation", Value: bson.D{{Key: "locale", Value: "simple"}}}}}, {Key: "verbosity", Value: "executionStats"}}
	raw := db.RunCommand(t.Context(), cmd)
	var data bson.M
	if err = raw.Decode(&data); err != nil {
		t.Fatal(err)
	}
	stats := data["executionStats"].(bson.M)
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(encoded, &explain.ExecutionStats); err != nil {
		t.Fatal(err)
	}
	if explain.ExecutionStats.TotalDocsExamined != 1 || explain.ExecutionStats.TotalKeysExamined != 1 || explain.ExecutionStats.NReturned != 1 {
		t.Fatal("real indexed page degraded to global scan")
	}
	if err = mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, b *MongoHistoricalOwnerBatch, g *MongoResponsibilitySnapshot, s *sqlevaluation.SQLHistoricalOwnerBatch) error {
		if b.Report().UniqueBusinessRows != 1 || b.Report().Queries != 3 || b.Report().Sources != 32 || g.Report().Rows < 4097 {
			return ErrMongoBatchConflict
		}
		for _, source := range sources {
			if _, err := b.ResolveSource(ctx, source); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("business_batch_index_plan keys=%d docs=%d returned=%d unrelated_rows=4096", explain.ExecutionStats.TotalKeysExamined, explain.ExecutionStats.TotalDocsExamined, explain.ExecutionStats.NReturned)
}

// The original native fixture exercises the same indexed owner expansion in
// a transaction and in two distinct read-only snapshot sessions. No write,
// global capability or production source authorization is minted here.
func TestMongoBatchNativeSnapshotInputOwnerFootprintMatchesOriginalFD(t *testing.T) {
	sqlDB, client, db, config, _, events := wholeJointNativeFixture(t, false)
	sources := mongoBatchNativeSources(t, events...)
	var original map[string][]bson.Raw
	if err := mongoBatchNativePair(t, sqlDB, client, db, config, sources, func(_ context.Context, b *MongoHistoricalOwnerBatch, _ *MongoResponsibilitySnapshot, _ *sqlevaluation.SQLHistoricalOwnerBatch) error {
		original = map[string][]bson.Raw{}
		for name, rows := range b.data {
			for _, raw := range rows {
				original[name] = append(original[name], append(bson.Raw(nil), raw...))
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var footprints []*MongoSnapshotOwnerFootprint
	var inputs []*MongoSnapshotInputEpoch
	var reads []*mongoHistoricalComponentReadRecipe
	for round := range 2 {
		session := snapshotInputNativeSession(t, client)
		err := historicalSourceInputNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, sqlInput *SQLResponsibilitySnapshot, input *MongoSnapshotInputEpoch, _ *gorm.DB) error {
			request, e := MongoHistoricalSQLBatchSelectors(sources)
			if e != nil {
				return e
			}
			sqlBatch, e := sqlevaluation.PrepareSQLHistoricalOwnerBatch(ctx, sqlInput.cycle, request, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
			if e != nil {
				return e
			}
			if b, e := PrepareMongoHistoricalOwnerBatch(ctx, nil, sqlBatch, sources, DefaultMongoHistoricalOwnerBatchLimits()); b != nil || e == nil {
				t.Fatal("snapshot input relaxed original global/transaction guard")
			}
			f, e := PrepareMongoSnapshotOwnerFootprint(ctx, input, sqlBatch, sources, DefaultMongoHistoricalOwnerBatchLimits())
			if e != nil {
				return e
			}
			for _, handle := range sources {
				facts, e := handle.Facts()
				if e != nil {
					return e
				}
				key, e := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
				if e != nil || !reflect.DeepEqual(f.sources[key], facts) {
					return ErrMongoBatchConflict
				}
			}
			read, e := freezeMongoSnapshotOwnerComponentReadRecipe(ctx, input, f)
			if e != nil {
				return e
			}
			if read.original.number != 0 || len(read.original.session) != 0 || read.snapshotOwner != f.InputSHA256() || read.snapshotEpoch != input.Summary().NativeEpochSHA256 || read.snapshotOriginalInput != input || read.snapshotDev != input.dev || read.snapshotIno != input.ino || !reflect.DeepEqual(read.selection, f.selection) || len(read.hints) == 0 {
				t.Fatal("snapshot recipe lost actual origin, complete range or index proof")
			}

			if f.InputSHA256() == "" || len(f.sources) != 2 || len(f.sqlOwners) != 2 || len(f.selection.outcomes) == 0 {
				t.Fatal("actual owner footprint missing source or negative range")
			}
			for _, name := range mongoBatchBusinessCollections {
				if len(f.data[name]) != len(original[name]) {
					t.Fatal("shared expansion lost original rows", name)
				}
				for i, raw := range f.data[name] {
					if !bytes.Equal(raw, original[name][i]) {
						t.Fatal("snapshot input differed from original business bytes", name)
					}
				}
			}
			if round == 1 {
				// Both inputs are genuine captured epochs. Equal public fingerprints
				// never substitute their actual original instance or original FD.
				t.Logf("actual_two_input_instances=true native_hash_equal=%t snapshot_hash_equal=%t", footprints[0].nativeSHA == f.nativeSHA, footprints[0].snapshotSHA == f.snapshotSHA)
				if old, e := freezeMongoSnapshotOwnerComponentReadRecipe(ctx, input, footprints[0]); old != nil || e == nil {
					t.Fatal("another actual input accepted the original footprint")
				}
				// Only private negative-probe headers are equalized; no native
				// session/time or stored frames are changed to mint a qualification.
				foreign := *footprints[0]
				foreign.self = &foreign
				foreign.nativeSHA, foreign.snapshotSHA = f.nativeSHA, f.snapshotSHA
				foreign.inputSHA = foreign.digest()
				if foreign.InputSHA256() == "" {
					t.Fatal("private equal-header rejection probe did not seal")
				}
				if bad, e := freezeMongoSnapshotOwnerComponentReadRecipe(ctx, input, &foreign); bad != nil || e == nil {
					t.Fatal("equal snapshot/native hashes replaced actual input and original FD")
				}
				foreignRead := *reads[0]
				foreignRead.snapshotEpoch, foreignRead.snapshotInput = read.snapshotEpoch, read.snapshotInput
				if foreignRead.matchesOriginalInput(ctx, input, mongoCycleTxn{}) {
					t.Fatal("equal read fingerprints replaced original input instance")
				}

				ranges := input.ownerPages["answersheets"]
				if len(ranges) == 0 {
					t.Fatal("actual owner page missing")
				}
				ref := input.pages[ranges[0].page]
				one := make([]byte, 1)
				if _, e = input.file.ReadAt(one, ref.Offset); e != nil {
					return e
				}
				saved := one[0]
				one[0] ^= 1
				if _, e = input.file.WriteAt(one, ref.Offset); e != nil {
					return e
				}
				if bad, e := PrepareMongoSnapshotOwnerFootprint(ctx, input, sqlBatch, sources, DefaultMongoHistoricalOwnerBatchLimits()); bad != nil || e == nil {
					t.Fatal("same-inode initial FD tamper became valid footprint")
				}
				if bad, e := freezeMongoSnapshotOwnerComponentReadRecipe(ctx, input, f); bad != nil || e == nil {
					t.Fatal("same-inode original FD tamper became a read recipe")
				}

				one[0] = saved
				if _, e = input.file.WriteAt(one, ref.Offset); e != nil {
					return e
				}
			}
			reads = append(reads, read)
			footprints = append(footprints, f)
			inputs = append(inputs, input)
			return nil
		})
		session.EndSession(t.Context())
		if err != nil {
			t.Fatal(err)
		}
	}
	if inputs[0].CompareFreshInput(t.Context(), inputs[1]) != nil {
		t.Fatal("independent initial snapshots did not match")
	}
	for i, input := range inputs {
		if r, e := freezeMongoSnapshotOwnerComponentReadRecipe(mongo.NewSessionContext(t.Context(), input.session), input, footprints[i]); r != nil || e == nil {
			t.Fatal("ended original snapshot produced a new read recipe")
		}
	}

	for _, f := range footprints {
		rows := 0
		if err := f.VisitRows(t.Context(), func(_ string, raw bson.Raw) error { rows++; raw[0] ^= 1; return nil }); err != nil || rows == 0 {
			t.Fatal("pure footprint lost rows after native scopes ended", err)
		}
		copy := *f
		if copy.InputSHA256() != "" {
			t.Fatal("copied input acquired original identity")
		}
		if _, err := json.Marshal(f); err == nil {
			t.Fatal("immutable input serialized as authority")
		}
	}
}
