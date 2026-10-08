//go:build integration

package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func coordinatorCrossNativeFirst(t *testing.T, f authFixture) (*HistoricalCoordinator, *HistoricalSourcePage, []*VerifiedSourceEvent) {
	t.Helper()
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	handles, err := p.Events()
	if err != nil {
		t.Fatal(err)
	}
	return c, p, handles
}

func coordinatorCrossNativeMongoHandles(t *testing.T, all []*VerifiedSourceEvent) []*VerifiedSourceEvent {
	t.Helper()
	var selected []*VerifiedSourceEvent
	for _, handle := range all {
		facts, err := handle.Facts()
		if err != nil {
			t.Fatal(err)
		}
		if facts.Source.Database == "mongodb" {
			selected = append(selected, handle)
		}
	}
	return selected
}

func coordinatorCrossNativeComplete(t *testing.T, c *HistoricalCoordinator, journal *HistoricalCoordinatorCrossStorePage) []HistoricalCandidate {
	t.Helper()
	if journal.Summary().WholeFourSourceCoverageBound {
		t.Fatal("qualification confused with clean four-copy second EOF")
	}
	if _, err := journal.ObservationBindingRange(0, 128); !errors.Is(err, ErrCoordinatorIncomplete) {
		t.Fatal("body-free binding escaped before source coverage", err)
	}
	if _, err := c.CandidateRange(0, 128); !errors.Is(err, ErrCoordinatorIncomplete) {
		t.Fatal("candidate escaped before source coverage", err)
	}
	if p, err := c.NextPage(t.Context()); p != nil || err != io.EOF {
		t.Fatal("exact four second-pass receipts not completed", err)
	}
	values, err := c.CandidateRange(0, 128)
	if err != nil {
		t.Fatal(err)
	}
	summary := journal.Summary()
	if !summary.WholeFourSourceCoverageBound || summary.DropReady || !summary.ExternalOriginRequired || !summary.AIInboxRequired || !summary.GlobalUnboundRequired || !summary.FinalFreshRequired || !summary.WriterFenceRequired || !summary.CASRequired || !evidenceHash(summary.CandidateSHA256) || !evidenceHash(summary.ObservationBindingSHA256) {
		t.Fatal("local journal claimed production authorization")
	}
	return values
}

func coordinatorCrossNativeGenerated(t *testing.T) (*gorm.DB, *mongo.Client, *mongo.Database, MongoOwnerConfig, event.DomainEvent, authFixture, *HistoricalCoordinator, *HistoricalSourcePage, []*VerifiedSourceEvent) {
	t.Helper()
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	payload, _ := mongoLocalGeneratedFixture(t, db)
	mongoBatchNativeIndexes(t, db)
	evt := mongoBatchNativeEvent(t, payload, "interpretation.report.generated", "coordinator-cross-original-generated")
	f := coordinatorNativeFixtureCopies(t, nil, []event.DomainEvent{evt})
	c, p, sources := coordinatorCrossNativeFirst(t, f)
	return sqlDB, client, db, config, evt, f, c, p, sources
}

func TestHistoricalCoordinatorCrossStoreNativeSixTypesFullEOFAndOriginalJournal(t *testing.T) {
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
	retry := requested
	retry.ExpectedAttempt, retry.AttemptOrigin, retry.Mode = 1, "automatic", "next_attempt"
	failed := eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, FailedAt: at, Reason: "native missing old original Run"}
	committed := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: "coordinator-cross-outcome", EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: at}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:1", CommittedAt: at}}
	mongoEvents := []event.DomainEvent{mongoBatchNativeEvent(t, submitted, "answersheet.submitted", "coordinator-cross-submitted"), mongoBatchNativeEvent(t, generated, "interpretation.report.generated", "coordinator-cross-generated")}
	f := coordinatorNativeFixtureCopies(t, []event.DomainEvent{mongoBatchNativeEvent(t, requested, "evaluation.requested", "coordinator-cross-requested"), mongoBatchNativeEvent(t, retry, "evaluation.retry.requested", "coordinator-cross-retry"), mongoBatchNativeEvent(t, failed, "evaluation.failed", "coordinator-cross-failed"), committed}, mongoEvents)
	for i, evt := range mongoEvents {
		crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", strconv.Itoa(i), nil)
	}
	c, page, sources := coordinatorCrossNativeFirst(t, f)
	var journal *HistoricalCoordinatorCrossStorePage
	err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
		mongoSources := coordinatorCrossNativeMongoHandles(t, sources)
		cross, err := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, catalog, sqlBatch, mongoBatch, mongoSources)
		if err != nil {
			return err
		}
		// The opaque cross adapter reruns precisely its one provisional owner;
		// the original batch stays blocked and is never modified by a DTO.
		original, err := mongoBatch.ResolveSource(ctx, mongoSources[1])
		if err != nil || original.Local().OwnerLocalTerminal {
			t.Fatal("fixture did not retain actual provisional ownership", err)
		}
		journal, err = c.QualifyCrossStorePage(ctx, page, sqlBatch, mongoBatch, cross)
		if err != nil {
			return err
		}
		if len(journal.bindings) != 2 || len(journal.bindings[1].ProvisionalOwnerResolvedObservationKeys) != 1 {
			t.Fatal("original observation resolution journal lost")
		}
		if _, err := c.QualifyCrossStorePage(ctx, page, sqlBatch, mongoBatch, cross); err == nil {
			t.Fatal("source page consumed twice")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := coordinatorCrossNativeComplete(t, c, journal)
	if len(rows) != 6 || c.Receipt().ConsumedRecords != [4]uint64{4, 0, 0, 2} || c.Receipt().CASComplete || c.Receipt().DropReady {
		t.Fatal("six original types/four physical source receipts lost")
	}
	byType := map[string]HistoricalCandidate{}
	for _, row := range rows {
		byType[row.EventType] = row
		if len(row.RequiredAdapters) < 10 {
			t.Fatal("remaining production gates lost")
		}
	}
	// This deliberately mixed page contains two old Mongo held rows. The
	// Submitted lookup sees the other Generation and cannot replace its
	// missing current SDK row with a separately authenticated old source.
	if byType["answersheet.submitted"].LocalQualified || !containsString(byType["answersheet.submitted"].BlockingReasons, "cross_store_related_mongo_event_original_owner_unknown") {
		t.Fatal("separate source's original owner gap silently disappeared")
	}
	// The frozen SQL resolver still sees the original Mongo held observations
	// as OwnerUnproven. Resolving their Mongo originals does not silently
	// change the SQL-side business classification or shared global catalog.
	for _, kind := range []string{"evaluation.requested", "evaluation.outcome.committed"} {
		if byType[kind].LocalQualified || !containsString(byType[kind].BlockingReasons, "current_message_or_replay_identity_conflict") {
			t.Fatal("Mongo-local resolution cleared the original SQL classification")
		}
	}
	if byType["evaluation.failed"].LocalQualified || byType["evaluation.retry.requested"].LocalQualified || byType["evaluation.requested"].ActualOriginalRun != nil || byType["evaluation.failed"].OriginalRunID != "" || byType["evaluation.failed"].ActualOriginalRun != nil {
		t.Fatal("missing old Run upgraded to current winner")
	}
	if original := byType["evaluation.outcome.committed"].ActualOriginalRun; original == nil || original.RunID != "42:1" || original.Attempt != 1 {
		t.Fatal("actual original SQL Run lost because responsibility remains blocked")
	}
	bindings, err := journal.ObservationBindingRange(0, 128)
	if err != nil || len(bindings) != 2 {
		t.Fatal(err)
	}
	for _, b := range bindings {
		if !evidenceHash(b.SourceFactsSHA256) || !evidenceHash(b.SQLFullRowBaselineSHA256) || !evidenceHash(b.SQLBusinessBaselineSHA256) || !evidenceHash(b.MongoBusinessBaselineSHA256) || !evidenceHash(b.ActualMongoSessionTransactionSHA256) || len(b.OwnerBoundObservationKeys) != 1 || len(b.OriginalObservationRowSHA256) != 1 {
			t.Fatal("source/cycle/full PK row hashes lost")
		}
	}
	bindings[1].OwnerBoundObservationKeys[0] = "changed"
	bindings[1].OriginalObservationRowSHA256["changed"] = "changed"
	bindings[1].ActualOriginalRun.RunID = "changed"
	again, err := journal.ObservationBindingRange(1, 1)
	if err != nil || again[0].OwnerBoundObservationKeys[0] == "changed" || len(again[0].OriginalObservationRowSHA256) != 1 || again[0].ActualOriginalRun.RunID != "11" {
		t.Fatal("exported defensive binding mutated private original", err)
	}
	if _, err := json.Marshal(journal); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("opaque journal became stored evidence")
	}
	// A same-package accidental mutation cannot keep the minted whole-copy
	// journal valid, even though the external caller cannot access this state.
	journal.bindings[0].BusinessBindingSHA256 = "tampered"
	if _, err := journal.ObservationBindingRange(0, 1); err == nil || journal.Summary().WholeFourSourceCoverageBound {
		t.Fatal("changed original binding journal accepted")
	}
	t.Logf("native_six_types=6 locally_qualified=%d blocked=%d four_second_eof=true actual_cross_store_consumed=true immutable_private_journal=true joint_scope_incomplete=true cas=false drop_ready=false", c.Receipt().LocallyQualifiedCount, c.Receipt().BlockedLocalCount)
}

func TestHistoricalCoordinatorCrossStoreNativeKeepsMixedPendingAndGlobalOrphanBlocking(t *testing.T) {
	for _, name := range []string{"closed_original", "pending_original", "lease_original", "other_unknown", "other_pending", "global_mongo_orphan", "same_id_cross_org", "same_id_other_type", "source_wire_conflict", "real_invalid_schema"} {
		t.Run(name, func(t *testing.T) {
			sqlDB, client, db, config, evt, _, c, page, sources := coordinatorCrossNativeGenerated(t)
			state := "replayed"
			if name == "pending_original" {
				state = "blocked"
			}
			var change func([]byte) []byte
			if name == "source_wire_conflict" {
				change = func(raw []byte) []byte {
					var v originalDomainEnvelope
					if err := json.Unmarshal(raw, &v); err != nil {
						t.Fatal(err)
					}
					v.OccurredAt = v.OccurredAt.Add(time.Millisecond)
					out, err := json.Marshal(v)
					if err != nil {
						t.Fatal(err)
					}
					return out
				}
			}
			crossMongoNativeWire(t, sqlDB, evt, "held", state, "original", change)
			switch name {
			case "lease_original":
				if err := sqlDB.Exec("UPDATE retry_event_hold SET claim_token='native-lease',claim_expires_at=UTC_TIMESTAMP(3)+INTERVAL 1 MINUTE").Error; err != nil {
					t.Fatal(err)
				}
			case "other_unknown", "other_pending":
				original, err := sources[0].Facts()
				if err != nil {
					t.Fatal(err)
				}
				second := mongoBatchNativeEvent(t, *original.Generated, "interpretation.report.generated", "unbound-other-original")
				state := "replayed"
				if name == "other_pending" {
					state = "blocked"
				}
				crossMongoNativeWire(t, sqlDB, second, "held", state, "other", nil)
			case "global_mongo_orphan":
				if _, err := db.Collection("rm_outbox").InsertOne(t.Context(), mongoCycleNativeMessage(t, evt, "published")); err != nil {
					t.Fatal(err)
				}
			case "same_id_cross_org", "same_id_other_type", "real_invalid_schema":
				edit := func(raw []byte) []byte {
					var v originalDomainEnvelope
					if err := json.Unmarshal(raw, &v); err != nil {
						t.Fatal(err)
					}
					switch name {
					case "same_id_other_type":
						v.EventType = "evaluation.requested"
					case "same_id_cross_org":
						var body map[string]json.RawMessage
						if err := json.Unmarshal(v.Data, &body); err != nil {
							t.Fatal(err)
						}
						body["org_id"] = json.RawMessage("8")
						v.Data, _ = json.Marshal(body)
					default:
						v.Data = json.RawMessage(`{"future":true}`)
					}
					out, err := json.Marshal(v)
					if err != nil {
						t.Fatal(err)
					}
					return out
				}
				crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", "other", edit)
			}
			wantError := name == "same_id_cross_org" || name == "same_id_other_type" || name == "source_wire_conflict" || name == "real_invalid_schema"
			var journal *HistoricalCoordinatorCrossStorePage
			err := crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
				cross, err := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, catalog, sqlBatch, mongoBatch, sources)
				if err != nil {
					return err
				}
				journal, err = c.QualifyCrossStorePage(ctx, page, sqlBatch, mongoBatch, cross)
				return err
			})
			if wantError {
				if err == nil || journal != nil || page.consumed || len(c.candidates) != 0 {
					t.Fatal("real invalid responsibility became a candidate")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			rows := coordinatorCrossNativeComplete(t, c, journal)
			if len(rows) != 1 || (name == "closed_original") != rows[0].LocalQualified {
				t.Logf("case=%s categories=%v", name, rows[0].BlockingReasons)
				t.Fatal("unrelated or unfinished responsibility cleared")
			}
			bindings, err := journal.ObservationBindingRange(0, 1)
			if err != nil {
				t.Fatal(err)
			}
			resolved := name == "closed_original" || name == "other_unknown" || name == "other_pending" || name == "global_mongo_orphan"
			if (len(bindings[0].ProvisionalOwnerResolvedObservationKeys) == 1) != resolved {
				t.Fatal("observation-specific provisional cause was not retained")
			}
			if name != "closed_original" && len(rows[0].BlockingReasons) == 0 {
				t.Fatal("unresolved global responsibility hidden by successful original")
			}
			if name == "global_mongo_orphan" {
				refs := bindings[0].CurrentStandardReferences
				if len(refs) != 1 || refs[0].EventID != evt.EventID() || !evidenceHash(refs[0].Fingerprint) || refs[0].Fingerprint == bindings[0].Source.Digest.SHA256 {
					t.Fatal("actual SDK reference lost or replaced by source-row digest")
				}
			}
		})
	}
}

func TestHistoricalCoordinatorCrossStoreNativeRejectsWrongCapabilityAndFactsWithoutConsumption(t *testing.T) {
	sqlDB, client, db, config, evt, fixture, c, page, sources := coordinatorCrossNativeGenerated(t)
	crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", "original", nil)
	foreign, foreignPage, _ := coordinatorCrossNativeFirst(t, fixture)
	err := crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
		cross, err := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, catalog, sqlBatch, mongoBatch, sources)
		if err != nil {
			return err
		}
		for _, name := range []string{"missing_host_transactions", "different_source_page", "same_value_source_handle", "different_sql_batch_pointer", "missing_source", "different_actual_session", "mutable_source_schema", "mutable_source_content", "mutable_source_org", "mutable_source_pk"} {
			t.Run(name, func(t *testing.T) {
				candidateCtx, coord, p, s := ctx, c, page, sqlBatch
				changed := *cross
				changed.sources = append([]*VerifiedSourceEvent(nil), cross.sources...)
				original := sources[0].facts
				switch name {
				case "missing_host_transactions":
					candidateCtx = t.Context()
				case "different_source_page":
					coord, p = foreign, foreignPage
				case "same_value_source_handle":
					changed.sources[0] = &VerifiedSourceEvent{facts: original}
				case "different_sql_batch_pointer":
					copy := *sqlBatch
					s = &copy
				case "missing_source":
					changed.sources = nil
				case "different_actual_session":
					session, err := client.StartSession()
					if err != nil {
						t.Fatal(err)
					}
					defer session.EndSession(t.Context())
					candidateCtx = mongo.NewSessionContext(ctx, session)
				default:
					mutated, err := sources[0].Facts()
					if err != nil {
						t.Fatal(err)
					}
					switch name {
					case "mutable_source_schema":
						mutated.SupportedSchema = "future/v10"
					case "mutable_source_content":
						mutated.Generated.GeneratedAt = mutated.Generated.GeneratedAt.Add(time.Millisecond)
					case "mutable_source_org":
						mutated.OrgID = 8
					case "mutable_source_pk":
						mutated.PrimaryKeyToken = "different original bson"
					}
					sources[0].facts = mutated
					defer func() { sources[0].facts = original }()
				}
				if journal, err := coord.QualifyCrossStorePage(candidateCtx, p, s, mongoBatch, &changed); journal != nil || err == nil || p.consumed || len(coord.candidates) != 0 {
					t.Fatal("wrong opaque page/session/source accepted")
				}
			})
		}
		journal, err := c.QualifyCrossStorePage(ctx, page, sqlBatch, mongoBatch, cross)
		if err != nil {
			return err
		}
		if !page.consumed || journal == nil {
			return ErrCoordinatorPage
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !page.consumed || foreignPage.consumed {
		t.Fatal("rejected capability caused cross-page consumption")
	}
}

func TestHistoricalCoordinatorCrossStoreNativeAllFourNonemptyCopiesAndUnboundAIStayRequired(t *testing.T) {
	sqlDB, client, db, config, evt, f, _, _, _ := coordinatorCrossNativeGenerated(t)
	crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", "original", nil)
	// These are real typed old AI wire fixtures. They are only source-copy
	// facts; this test deliberately has no AI business capability and keeps
	// both physical AI rows blocked. No delivered flag is acceptance.
	ai := coordinatorFixture(t, 1, true)
	f.raw[0], f.expected[0] = ai.raw[0], ai.expected[0]
	f.raw[1], f.expected[1] = ai.raw[1], ai.expected[1]
	f.raw[2], f.expected[2] = ai.raw[2], ai.expected[2]
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	sqlPage, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.QualifyPage(t.Context(), sqlPage, nil, nil); err != nil {
		t.Fatal(err)
	}
	aiPage, err := c.NextPage(t.Context())
	if err != nil || aiPage.rows[0].bridge == nil || aiPage.rows[0].legacy == nil {
		t.Fatal("paired physical AI copy page lost", err)
	}
	if err := c.QualifyAIPage(t.Context(), aiPage, nil); err != nil {
		t.Fatal(err)
	}
	page, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sources, err := page.Events()
	if err != nil {
		t.Fatal(err)
	}
	var journal *HistoricalCoordinatorCrossStorePage
	err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
		cross, err := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, catalog, sqlBatch, mongoBatch, sources)
		if err != nil {
			return err
		}
		journal, err = c.QualifyCrossStorePage(ctx, page, sqlBatch, mongoBatch, cross)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := coordinatorCrossNativeComplete(t, c, journal)
	if len(rows) != 7 || c.Receipt().ConsumedRecords != [4]uint64{4, 1, 1, 1} {
		t.Fatal("four nonempty copies not consumed exactly once")
	}
	var aiCount int
	for _, row := range rows {
		if row.EventType == "ai.command.start" {
			aiCount++
			if row.LocalQualified || !containsString(row.BlockingReasons, "actual_ai_local_resolver_missing") {
				t.Fatal("blocked AI DTO accepted as business closure")
			}
		}
	}
	if aiCount != 2 || c.Receipt().DropReady || c.Receipt().BusinessClosureVerified {
		t.Fatal("two AI physical rows collapsed or production gates removed")
	}
	bindings, err := journal.ObservationBindingRange(0, 1)
	if err != nil || len(bindings) != 1 || len(bindings[0].ProvisionalOwnerResolvedObservationKeys) != 1 {
		t.Fatal("consumed cross-store journal detached from four-source coverage", err)
	}
	// Actual borrowed transactions have ended. An exported defensive result
	// cannot re-enter the qualifier as a new capability.
	if journal, err := c.QualifyCrossStorePage(t.Context(), page, nil, nil, nil); journal != nil || err == nil {
		t.Fatal("completed source consumption replayed")
	}
	t.Log("nonempty_four_source_counts=4,1,1,1 full_second_eof=true two_ai_rows_blocked=true source_origin_unproven=true drop_ready=false")
}

func TestHistoricalCoordinatorCrossStoreNativeUnknownLedgerSchemaRefusesBeforeConsume(t *testing.T) {
	sqlDB, client, db, config, evt, _, c, page, sources := coordinatorCrossNativeGenerated(t)
	crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", "original", nil)
	if err := sqlDB.Exec("ALTER TABLE retry_event_hold ADD COLUMN native_future_payload JSON NULL").Error; err != nil {
		t.Fatal(err)
	}
	err := crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sqlBatch *SQLBusinessOwnerBatch, mongoBatch *MongoHistoricalOwnerBatch) error {
		cross, err := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, catalog, sqlBatch, mongoBatch, sources)
		if err != nil {
			return err
		}
		_, err = c.QualifyCrossStorePage(ctx, page, sqlBatch, mongoBatch, cross)
		return err
	})
	if err == nil || page.consumed || len(c.candidates) != 0 || c.Receipt().SourceCoverageComplete {
		t.Fatal("future unknown ledger schema cleared as no matching responsibility")
	}
	var row bson.M
	if err := db.Collection("report_generations").FindOne(t.Context(), bson.M{"domain_id": int64(1)}).Decode(&row); err != nil {
		t.Fatal(err)
	}
	if _, wrote := row["historical_references"]; wrote {
		t.Fatal("read-only qualifier wrote historical evidence")
	}
}
