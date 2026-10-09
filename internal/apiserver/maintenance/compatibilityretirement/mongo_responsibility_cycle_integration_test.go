//go:build integration

package retirement

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
)

func mongoCycleNativeDB(t *testing.T) (*mongo.Client, *mongo.Database, MongoOwnerConfig) {
	t.Helper()
	client, db, cfg := mongoLocalNativeDB(t)
	// The point-reader adversarial helper uses casefold collections. The
	// cycle's typed binary-token paging protocol requires a real simple _id
	// index. Recreate only this test's six empty collections, preserving its
	// migration UUID/approved identity. Non-simple rejection is tested below.
	for _, name := range []string{"answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts", "rm_outbox", "qs_rm_replay_requests"} {
		if e := db.Collection(name).Drop(t.Context()); e != nil {
			t.Fatal(e)
		}
		if e := db.CreateCollection(t.Context(), name); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range mongoCycleCollections {
		if _, ok := map[string]bool{"answersheets": true, "report_generations": true, "interpretation_runs": true, "interpret_report_artifacts": true, "rm_outbox": true, "qs_rm_replay_requests": true}[name]; ok {
			continue
		}
		if e := db.CreateCollection(t.Context(), name); e != nil {
			t.Fatal(e)
		}
	}
	return client, db, cfg
}

func mongoCycleNativeTx(t *testing.T, client *mongo.Client, fn func(mongo.SessionContext) error) error {
	t.Helper()
	session, e := client.StartSession()
	if e != nil {
		t.Fatal(e)
	}
	defer session.EndSession(t.Context())
	if e = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); e != nil {
		t.Fatal(e)
	}
	ctx := mongo.NewSessionContext(t.Context(), session)
	err := fn(ctx)
	if e = session.AbortTransaction(t.Context()); e != nil {
		t.Fatal(e)
	}
	return err
}

func mongoCycleNativeSheet(t *testing.T, db *mongo.Database, id uint64, eventID string, state string) {
	t.Helper()
	row := mongoLocalSheet()
	row.ID = primitive.NewObjectID()
	row.DomainID = meta.ID(id)
	if eventID != "" {
		row.DurableAcceptance = &sheetmongo.DurableAcceptancePO{SchemaVersion: 1, EventID: eventID, AcceptedAt: row.FilledAt}
		p, e := mongoSubmissionPayload(row)
		if e != nil {
			t.Fatal(e)
		}
		insert := mongoLocalStandardFixture(t, p, eventID, state)
		if _, e = db.Collection("rm_outbox").InsertOne(t.Context(), insert); e != nil {
			t.Fatal(e)
		}
	}
	insertMongoLocalSheet(t, db, row)
}

func TestMongoCycleNativeSnapshotFullPagingAndFresh(t *testing.T) {
	client, db, cfg := mongoCycleNativeDB(t)
	for i := uint64(1); i <= 7; i++ {
		mongoCycleNativeSheet(t, db, 10_000+i, "", "")
	}
	var snapshot *MongoResponsibilitySnapshot
	err := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		var e error
		snapshot, e = PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
		if e != nil {
			return e
		}
		if _, e = snapshot.RecheckFresh(ctx); !errors.Is(e, ErrMongoCycleTransaction) {
			t.Fatal("original transaction accepted as fresh", e)
		}
		if snapshot.ValidateBorrowedSnapshot(context.Background()) == nil {
			t.Fatal("hostless lookup accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r := snapshot.Report()
	if !r.Complete || !r.LocalGraphChecked || r.Rows != 7 || r.ClassifiedRows != 7 || r.DropReady || r.Collections[0].Pages < 4 {
		t.Fatal("complete global cycle counts/bounds", r)
	}
	err = mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		r, e := snapshot.RecheckFresh(ctx)
		if e == nil && (!r.FreshActualTransaction || !r.OldBoundRowsUnchanged || !r.NoNewRows || !r.EntireSnapshotUnchanged || r.DropReady) {
			t.Fatal("fresh gate", r)
		}
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.Ping(t.Context(), nil) != nil {
		t.Fatal("borrowed client was closed")
	}
}

func TestMongoCycleNativeFreshDetectsAllMutationClasses(t *testing.T) {
	for _, kind := range []string{"within_update", "within_delete", "above_upper", "below_lower", "uuid_recreate", "index_change", "options_change", "head_dirty", "unknown_namespace", "new_type"} {
		t.Run(kind, func(t *testing.T) {
			client, db, cfg := mongoCycleNativeDB(t)
			mongoCycleNativeSheet(t, db, 100, "", "")
			mongoCycleNativeSheet(t, db, 101, "", "")
			var snapshot *MongoResponsibilitySnapshot
			if e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
				var e error
				snapshot, e = PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
				return e
			}); e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "within_update":
				_, e := db.Collection("answersheets").UpdateOne(t.Context(), bson.M{"domain_id": int64(100)}, bson.M{"$set": bson.M{"questionnaire_title": "changed"}})
				if e != nil {
					t.Fatal(e)
				}
			case "within_delete":
				_, e := db.Collection("answersheets").DeleteOne(t.Context(), bson.M{"domain_id": int64(100)})
				if e != nil {
					t.Fatal(e)
				}
			case "above_upper", "below_lower":
				row := mongoLocalSheet()
				row.DomainID = meta.ID(102)
				if kind == "below_lower" {
					row.ID = primitive.ObjectID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
				} else {
					row.ID = primitive.ObjectID{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 254}
				}
				insertMongoLocalSheet(t, db, row)
			case "uuid_recreate":
				if e := db.Collection("qs_rm_replay_requests").Drop(t.Context()); e != nil {
					t.Fatal(e)
				}
				if e := db.CreateCollection(t.Context(), "qs_rm_replay_requests"); e != nil {
					t.Fatal(e)
				}
			case "index_change":
				if _, e := db.Collection("answersheets").Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "org_id", Value: 1}}, Options: options.Index().SetName("changed")}); e != nil {
					t.Fatal(e)
				}
			case "options_change":
				if e := db.RunCommand(t.Context(), bson.D{{Key: "collMod", Value: "answersheets"}, {Key: "validator", Value: bson.D{{Key: "org_id", Value: bson.D{{Key: "$type", Value: "long"}}}}}}).Err(); e != nil {
					t.Fatal(e)
				}
			case "head_dirty":
				if _, e := db.Collection("schema_migrations").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"dirty": true}}); e != nil {
					t.Fatal(e)
				}
			case "unknown_namespace":
				if e := db.CreateCollection(t.Context(), "future_runtime_inbox"); e != nil {
					t.Fatal(e)
				}
			case "new_type":
				if _, e := db.Collection("answersheets").InsertOne(t.Context(), bson.M{"_id": "different_bson_type", "domain_id": int64(102)}); e != nil {
					t.Fatal(e)
				}
			}
			if e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error { _, e := snapshot.RecheckFresh(ctx); return e }); e == nil {
				t.Fatal("fresh mutation accepted")
			}
		})
	}
}

func TestMongoCycleNativeLegalOtherCurrentPendingAndSourceLinkedResponsibility(t *testing.T) {
	client, db, cfg := mongoCycleNativeDB(t)
	mongoCycleNativeSheet(t, db, 10042, "live-current", "pending")
	var snapshot *MongoResponsibilitySnapshot
	if e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		var e error
		snapshot, e = PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if len(snapshot.Report().BlockingReasons) != 0 {
		t.Fatal("legal current pending globally blocked", snapshot.Report())
	}
	p, e := mongoSubmissionPayload(mongoLocalSheet())
	if e != nil {
		t.Fatal(e)
	}
	source := mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "old-submission")
	view, e := snapshot.ForUntrustedSource(t.Context(), source)
	if e != nil || len(view.BlockingReasons) == 0 {
		t.Fatal("linked old-owner responsibility ignored", e)
	}
	source.AggregateID = "999999"
	source.BusinessIDs = map[string]string{"answer_sheet_id": "999999"}
	view, e = snapshot.ForUntrustedSource(t.Context(), source)
	if e != nil || len(view.BlockingReasons) != 0 {
		t.Fatal("unrelated pending must not block other source", e)
	}
}

func TestMongoCycleNativeStrictGlobalUnknownOrOrphan(t *testing.T) {
	for _, kind := range []string{"rm_unknown_field", "rm_reordered_key", "rm_fingerprint", "rm_orphan", "org_conflict", "duplicate_domain", "budget_rows", "budget_bytes", "budget_graph", "heterogeneous_id", "future_owner_field", "future_generation_schema", "missing_owner_admission"} {
		t.Run(kind, func(t *testing.T) {
			client, db, cfg := mongoCycleNativeDB(t)
			mongoCycleNativeSheet(t, db, 10042, "native-standard", "published")
			limits := mongoCycleTestLimits()
			switch kind {
			case "rm_unknown_field":
				_, e := db.Collection("rm_outbox").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"future_transport_semantics": true}})
				if e != nil {
					t.Fatal(e)
				}
			case "rm_reordered_key":
				var row bson.D
				if e := db.Collection("rm_outbox").FindOne(t.Context(), bson.M{}).Decode(&row); e != nil {
					t.Fatal(e)
				}
				if _, e := db.Collection("rm_outbox").DeleteMany(t.Context(), bson.M{}); e != nil {
					t.Fatal(e)
				}
				for i, v := range row {
					if v.Key == "_id" {
						d := v.Value.(bson.D)
						row[i].Value = bson.D{d[1], d[0], d[2]}
					}
				}
				if _, e := db.Collection("rm_outbox").InsertOne(t.Context(), row); e != nil {
					t.Fatal(e)
				}
			case "rm_fingerprint":
				_, e := db.Collection("rm_outbox").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"fingerprint": make([]byte, 32)}})
				if e != nil {
					t.Fatal(e)
				}
			case "rm_orphan":
				_, e := db.Collection("answersheets").DeleteMany(t.Context(), bson.M{})
				if e != nil {
					t.Fatal(e)
				}
			case "org_conflict":
				_, e := db.Collection("answersheets").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"org_id": int64(8)}})
				if e != nil {
					t.Fatal(e)
				}
			case "duplicate_domain":
				row := mongoLocalSheet()
				row.ID = primitive.NewObjectID()
				insertMongoLocalSheet(t, db, row)
			case "budget_rows":
				limits.MaxRows = 1
			case "budget_bytes":
				limits.MaxBytes = 10
			case "budget_graph":
				limits.MaxGraphEntries = 1
			case "heterogeneous_id":
				if _, e := db.Collection("answersheets").InsertOne(t.Context(), bson.M{"_id": int64(10), "domain_id": int64(10)}); e != nil {
					t.Fatal(e)
				}
			case "future_owner_field":
				_, e := db.Collection("answersheets").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"future_business_semantics": "unknown"}})
				if e != nil {
					t.Fatal(e)
				}
			case "future_generation_schema":
				mongoLocalGeneratedFixture(t, db)
				if _, e := db.Collection("report_generations").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"transaction_schema_version": int32(2)}}); e != nil {
					t.Fatal(e)
				}
			case "missing_owner_admission":
				_, e := db.Collection("answersheets").UpdateMany(t.Context(), bson.M{}, bson.M{"$unset": bson.M{"admission": ""}})
				if e != nil {
					t.Fatal(e)
				}
			}
			var snapshot *MongoResponsibilitySnapshot
			e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
				var err error
				snapshot, err = PrepareMongoResponsibilitySnapshot(ctx, db, cfg, limits)
				return err
			})
			if e == nil && len(snapshot.Report().BlockingReasons) == 0 {
				t.Fatal("unknown/conflict silently accepted", kind)
			}
		})
	}
}

func TestMongoCycleNativeUnknownNamespaceAndBorrowedActualSnapshotRequired(t *testing.T) {
	client, db, cfg := mongoCycleNativeDB(t)
	if e := db.CreateCollection(t.Context(), "ai_explanation_generations"); e != nil {
		t.Fatal(e)
	}
	if _, e := PrepareMongoResponsibilitySnapshot(t.Context(), db, cfg, mongoCycleTestLimits()); !errors.Is(e, ErrMongoCycleTransaction) {
		t.Fatal("host transaction absent", e)
	}
	if e := mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
		_, e := PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
		if !errors.Is(e, ErrMongoCycleTransaction) {
			t.Fatal("default transaction upgraded to snapshot", e)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		s, e := PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
		if e != nil {
			return e
		}
		if !strings.Contains(strings.Join(s.Report().CoverageGaps, ","), "unknown_catalog_namespace") {
			t.Fatal("retired AI namespace silently current")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}

func TestMongoCycleNativeScaleIndependentOfHistoricalSourceCount(t *testing.T) {
	if testing.Short() {
		t.Fatal("scale native must run; short is not acceptance")
	}
	client, db, cfg := mongoCycleNativeDB(t)
	const count = 250_123
	for start := 0; start < count; start += 1000 {
		end := start + 1000
		if end > count {
			end = count
		}
		docs := make([]any, 0, end-start)
		for i := start; i < end; i++ {
			row := sheetmongo.AnswerSheetPO{BaseDocument: base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.ID(uint64(1_000_000 + i))}, OrgID: 7, TesteeID: 9}
			docs = append(docs, row)
		}
		if _, e := db.Collection("answersheets").InsertMany(t.Context(), docs); e != nil {
			t.Fatal(e)
		}
	}
	limits := MongoResponsibilityLimits{PageRows: 2048, MaxRows: 1_000_000, MaxBytes: 512 << 20, MaxPages: 1000, MaxGraphEntries: 1_000_000, MaxGraphBytes: 512 << 20, MaxDuration: 2 * time.Minute}
	var snapshot *MongoResponsibilitySnapshot
	if e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		var e error
		snapshot, e = PrepareMongoResponsibilitySnapshot(ctx, db, cfg, limits)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	r := snapshot.Report()
	if r.Rows != count || r.ClassifiedRows != count || !r.Complete || r.DropReady {
		t.Fatal("scale omitted rows or manufactured approval", r)
	}
	t.Logf("safe_scale_counts rows=%d source_lookups=100000 bytes=%d pages=%d graph_entries=%d graph_bytes_budgeted=%d drop_ready=false", r.Rows, r.Bytes, r.Pages, r.GraphEntries, r.GraphBytes)
	for i := 0; i < 100_000; i++ {
		source := &DecodedSourceEvent{EventID: "old-not-present", EventType: "answersheet.submitted", OrgID: 7, AggregateType: "AnswerSheet", AggregateID: strconvForMongoCycleScale(i)}
		if _, e := snapshot.ForUntrustedSource(t.Context(), source); e != nil {
			t.Fatal(e)
		}
	}
	if e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error { _, e := snapshot.RecheckFresh(ctx); return e }); e != nil {
		t.Fatal(e)
	}
}
func strconvForMongoCycleScale(i int) string { return mongoCycleKey(uint64(1_000_000 + i)) }

func mongoCycleNativeMessage(t *testing.T, evt event.DomainEvent, state string) bson.D {
	t.Helper()
	cfg, e := eventcatalog.Load("../../../../configs/events.yaml")
	if e != nil {
		t.Fatal(e)
	}
	intents, e := standard.PrepareIntents([]event.DomainEvent{evt}, eventcatalog.NewCatalog(cfg), "api-server")
	if e != nil || len(intents) != 1 {
		t.Fatal("real RM producer", e)
	}
	in := intents[0].Message.Input()
	fp := intents[0].Message.Fingerprint()
	return bson.D{{Key: "_id", Value: bson.D{{Key: "producer", Value: in.Producer}, {Key: "message_id", Value: in.ID}, {Key: "destination", Value: in.Destination}}}, {Key: "producer", Value: in.Producer}, {Key: "message_id", Value: in.ID}, {Key: "destination", Value: in.Destination}, {Key: "event_type", Value: in.EventType}, {Key: "schema_version", Value: in.SchemaVersion}, {Key: "scope", Value: in.Scope}, {Key: "content_type", Value: in.ContentType}, {Key: "occurred_at", Value: in.OccurredAt}, {Key: "payload", Value: in.Payload}, {Key: "fingerprint", Value: fp[:]}, {Key: "state", Value: state}, {Key: "version", Value: int64(3)}, {Key: "transport_confirmed_at", Value: evt.OccurredAt().Add(time.Second)}, {Key: "failure_count", Value: int64(1)}}
}

func TestMongoCycleNativeReportGraphAndReverseCatalog(t *testing.T) {
	for _, kind := range []string{"valid", "artifact_orphan", "artifact_cross_org", "artifact_original_clock", "payload_model_conflict", "generation_wrong_current_artifact", "duplicate_attempt", "run_failed", "catalog_orphan", "catalog_cross_org", "native_ref_and_message_absent", "pending_generation"} {
		t.Run(kind, func(t *testing.T) {
			client, db, cfg := mongoCycleNativeDB(t)
			payload, _ := mongoLocalGeneratedFixture(t, db)
			id := "current-generated"
			if kind == "payload_model_conflict" {
				payload.Model.Title = "changed"
			}
			evt := event.Event[eventoutcome.ReportGeneratedPayload]{BaseEvent: event.BaseEvent{ID: id, EventTypeValue: "interpretation.report.generated", AggregateTypeValue: "ReportGeneration", AggregateIDValue: "1", OccurredAtValue: payload.GeneratedAt.Add(time.Second)}, Data: payload}
			if _, e := db.Collection("rm_outbox").InsertOne(t.Context(), mongoCycleNativeMessage(t, evt, "published")); e != nil {
				t.Fatal(e)
			}
			if _, e := db.Collection("report_generations").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"generated_event_id": id, "transaction_schema_version": int32(1)}}); e != nil {
				t.Fatal(e)
			}
			catalog := interpretmongo.ReportCatalogPO{AssessmentID: 42, OrgID: 7, TesteeID: 21, OutcomeID: 9001, GenerationID: 1, SourceKind: "artifact", SourceID: 20, SortReportID: 20, SortAt: payload.GeneratedAt, UpdatedAt: payload.GeneratedAt}
			if kind == "catalog_orphan" {
				catalog.SourceID = 999
			}
			if kind == "catalog_cross_org" {
				catalog.OrgID = 8
			}
			if _, e := db.Collection("report_query_catalog").InsertOne(t.Context(), catalog); e != nil {
				t.Fatal(e)
			}
			set := func(name string, changes bson.M) {
				if _, e := db.Collection(name).UpdateMany(t.Context(), bson.M{}, bson.M{"$set": changes}); e != nil {
					t.Fatal(e)
				}
			}
			switch kind {
			case "artifact_orphan":
				set("interpret_report_artifacts", bson.M{"interpretation_run_id": int64(99)})
			case "artifact_cross_org":
				set("interpret_report_artifacts", bson.M{"org_id": int64(8)})
			case "artifact_original_clock":
				set("interpret_report_artifacts", bson.M{"generated_at": payload.GeneratedAt.Add(time.Second)})
			case "generation_wrong_current_artifact":
				set("report_generations", bson.M{"report_id": int64(99)})
			case "native_ref_and_message_absent":
				set("report_generations", bson.M{"generated_event_id": ""})
				if _, e := db.Collection("rm_outbox").DeleteMany(t.Context(), bson.M{}); e != nil {
					t.Fatal(e)
				}
			case "run_failed":
				set("interpretation_runs", bson.M{"status": "failed"})
			case "pending_generation":
				set("report_generations", bson.M{"status": "generating"})
			case "duplicate_attempt":
				var run interpretmongo.InterpretationRunPO
				if e := db.Collection("interpretation_runs").FindOne(t.Context(), bson.M{}).Decode(&run); e != nil {
					t.Fatal(e)
				}
				run.ID = primitive.NewObjectID()
				run.DomainID = meta.ID(12)
				if _, e := db.Collection("interpretation_runs").InsertOne(t.Context(), run); e != nil {
					t.Fatal(e)
				}
			}
			var snapshot *MongoResponsibilitySnapshot
			e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
				var e error
				snapshot, e = PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
				return e
			})
			if e != nil {
				t.Fatal(e)
			}
			if kind == "valid" {
				if len(snapshot.Report().BlockingReasons) != 0 || !snapshot.Report().SQLAIInboxCoverageRequired {
					t.Fatal("local graph or SQL boundary", snapshot.Report())
				}
			} else if kind == "pending_generation" {
				view, e := snapshot.ForUntrustedSource(t.Context(), mongoLocalNativeSource(t, payload, "interpretation.report.generated", "ReportGeneration", "old-generated"))
				if e != nil || len(view.BlockingReasons) == 0 {
					t.Fatal("pending original owner responsibility ignored")
				}
			} else if len(snapshot.Report().BlockingReasons) == 0 {
				t.Fatal("bad original/standard reverse graph accepted", kind)
			}
		})
	}
}

func TestMongoCycleNativeReplayAllGlobalRows(t *testing.T) {
	for _, kind := range []string{"authorized_closed", "authorized_pending", "authorized_orphan", "denied_not_found", "wrong_org", "wrong_hash", "duplicate_event_target", "missing_authorized", "unknown_field"} {
		t.Run(kind, func(t *testing.T) {
			client, db, cfg := mongoCycleNativeDB(t)
			mongoCycleNativeSheet(t, db, 10042, "current-replay", "published")
			request := standard.ReplayRequest{OrgID: 7, RequestID: "request-1", Store: "mongo-domain-events", Reason: "native_test", Targets: []standard.ReplayTarget{{EventID: "current-replay", ExpectedFailureCount: 1}}}
			if kind == "authorized_orphan" || kind == "denied_not_found" {
				request.Targets[0].EventID = "old-absent"
			}
			if kind == "wrong_org" {
				request.OrgID = 8
			}
			if kind == "duplicate_event_target" {
				request.Targets = append(request.Targets, request.Targets[0])
			}
			h, e := request.Fingerprint()
			if kind == "duplicate_event_target" {
				if e == nil {
					t.Fatal("real producer allowed duplicate replay")
				}
				request.Targets = request.Targets[:1]
				h, e = request.Fingerprint()
			}
			if e != nil {
				t.Fatal(e)
			}
			if kind == "wrong_hash" {
				h[0] ^= 1
			}
			item := bson.M{"event_id": request.Targets[0].EventID, "expected_failure_count": int64(1), "authorized": kind != "denied_not_found", "reason": "native_result"}
			if kind == "missing_authorized" {
				delete(item, "authorized")
			}
			items := bson.A{item}
			if kind == "duplicate_event_target" {
				items = append(items, item)
			}
			doc := bson.M{"_id": mongoCycleKey(uint64(request.OrgID)) + ":" + request.RequestID, "org_id": request.OrgID, "request_id": request.RequestID, "store_name": request.Store, "reason": request.Reason, "input_hash": h[:], "items": items, "created_at": time.Now().UTC()}
			if kind == "unknown_field" {
				doc["future_replay_contract"] = true
			}
			if _, e = db.Collection("qs_rm_replay_requests").InsertOne(t.Context(), doc); e != nil {
				t.Fatal(e)
			}
			if _, e = db.Collection("rm_outbox").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"manual_replay_request_id": "request-1", "manual_replay_version": int64(1)}}); e != nil {
				t.Fatal(e)
			}
			if kind == "authorized_pending" {
				if _, e = db.Collection("rm_outbox").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"state": "retry_wait"}}); e != nil {
					t.Fatal(e)
				}
			}
			var snapshot *MongoResponsibilitySnapshot
			e = mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
				var e error
				snapshot, e = PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
				return e
			})
			if kind == "authorized_closed" || kind == "denied_not_found" {
				if e != nil || len(snapshot.Report().BlockingReasons) != 0 {
					t.Fatal("closed replay or denied fact", e)
				}
			} else if kind == "authorized_pending" {
				if e != nil {
					t.Fatal(e)
				}
				p, _ := mongoSubmissionPayload(mongoLocalSheet())
				view, e := snapshot.ForUntrustedSource(t.Context(), mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "old-replay-owner"))
				if e != nil || len(view.BlockingReasons) == 0 {
					t.Fatal("linked authorized replay unfinished")
				}
			} else if e == nil && len(snapshot.Report().BlockingReasons) == 0 {
				t.Fatal("invalid/unbound replay accepted", kind)
			}
		})
	}
}

func TestMongoCycleNativeDuplicateHistoricalOriginalReference(t *testing.T) {
	client, db, cfg := mongoCycleNativeDB(t)
	entry := evidence.HistoricalReferenceEntryV1{EventID: "old-history", EventType: "answersheet.submitted", Source: evidence.HistoricalSourceReferenceV1{Database: "mongodb", Object: "domain_event_outbox", PrimaryKeyKind: "mongodb_objectid", PrimaryKeySHA256: strings.Repeat("1", 64), Digest: evidence.SourceDigest(MongoRowDigestKind, []byte("private native source"))}}
	entry.Proof = &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: entry.EventID, Digest: entry.Source.Digest, BusinessBindingSHA256: strings.Repeat("2", 64), Origin: "retirement", Verification: evidence.Verification{OperationID: "123-1", Method: "native", Version: "v1", VerifiedAt: time.Now().UTC().Truncate(time.Millisecond), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}
	set := &evidence.HistoricalReferenceSetV1{Version: 1, Entries: []evidence.HistoricalReferenceEntryV1{entry}}
	if e := set.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, id := range []uint64{10042, 10043} {
		row := mongoLocalSheet()
		row.ID = primitive.NewObjectID()
		row.DomainID = meta.ID(id)
		row.LegacySubmissionEvidence = set
		insertMongoLocalSheet(t, db, row)
	}
	if e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
		_, e := PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
		return e
	}); !errors.Is(e, ErrMongoCycleConflict) {
		t.Fatal("duplicated original reference across real business records", e)
	}
}

func TestMongoCycleNativeActualSameSessionNewTxnAndAbsentLedgers(t *testing.T) {
	client, db, cfg := mongoCycleNativeDB(t)
	for _, name := range mongoCycleCollections {
		if e := db.Collection(name).Drop(t.Context()); e != nil {
			t.Fatal(e)
		}
	}
	session, e := client.StartSession()
	if e != nil {
		t.Fatal(e)
	}
	defer session.EndSession(t.Context())
	if e = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); e != nil {
		t.Fatal(e)
	}
	ctx := mongo.NewSessionContext(t.Context(), session)
	snapshot, e := PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
	if e != nil {
		t.Fatal(e)
	}
	if e = session.AbortTransaction(t.Context()); e != nil {
		t.Fatal(e)
	}
	if e = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); e != nil {
		t.Fatal(e)
	}
	fresh, e := snapshot.RecheckFresh(mongo.NewSessionContext(t.Context(), session))
	if e != nil || !fresh.FreshActualTransaction || !fresh.EntireSnapshotUnchanged {
		t.Fatal("same lsid new actual transaction not supported", e)
	}
	if e = session.AbortTransaction(t.Context()); e != nil {
		t.Fatal(e)
	}
	r := snapshot.Report()
	if r.Rows != 0 || r.DropReady || len(r.Collections) != len(mongoCycleCollections) {
		t.Fatal("absent ledgers promoted to deletion permission")
	}
	for _, c := range r.Collections {
		if c.Present {
			t.Fatal("absent became empty")
		}
	}
}

func TestMongoCycleNativeActualIdentityHeadAndViewRefused(t *testing.T) {
	for _, kind := range []string{"identity_mismatch", "wrong_head", "dirty_head", "view_namespace", "non_simple_id_index", "replica_set_rejects_unindexed_collection"} {
		t.Run(kind, func(t *testing.T) {
			client, db, cfg := mongoCycleNativeDB(t)
			switch kind {
			case "identity_mismatch":
				cfg.ExpectedIdentityHash = strings.Repeat("0", 64)
			case "wrong_head":
				if _, e := db.Collection("schema_migrations").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"version": int64(37)}}); e != nil {
					t.Fatal(e)
				}
			case "dirty_head":
				if _, e := db.Collection("schema_migrations").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"dirty": true}}); e != nil {
					t.Fatal(e)
				}
			case "view_namespace":
				if e := db.CreateView(t.Context(), "future_responsibility_view", "answersheets", mongo.Pipeline{}); e != nil {
					t.Fatal(e)
				}
			case "non_simple_id_index":
				if e := db.Collection("answersheets").Drop(t.Context()); e != nil {
					t.Fatal(e)
				}
				if e := db.CreateCollection(t.Context(), "answersheets", options.CreateCollection().SetCollation(&options.Collation{Locale: "en", Strength: 2})); e != nil {
					t.Fatal(e)
				}
			case "replica_set_rejects_unindexed_collection":
				if e := db.Collection("qs_rm_replay_requests").Drop(t.Context()); e != nil {
					t.Fatal(e)
				}
				err := db.RunCommand(t.Context(), bson.D{{Key: "create", Value: "qs_rm_replay_requests"}, {Key: "capped", Value: true}, {Key: "size", Value: int64(4096)}, {Key: "autoIndexId", Value: false}}).Err()
				var command mongo.CommandError
				if !errors.As(err, &command) || command.Code != 50001 {
					t.Fatal("native RS did not enforce required _id index", err)
				}
				// This real server constraint prevents constructing that invalid
				// native collection. It is not scanner acceptance or a skip.
				return
			}
			if e := mongoCycleNativeTx(t, client, func(ctx mongo.SessionContext) error {
				_, e := PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
				return e
			}); e == nil {
				t.Fatal("unproven real identity/head/schema accepted", kind)
			}
		})
	}
}

func TestMongoCycleNativeRealBSONRangePlan(t *testing.T) {
	_, db, _ := mongoCycleNativeDB(t)
	for _, name := range []string{"answersheets", "rm_outbox"} {
		docs := []any{}
		for i := 0; i < 100; i++ {
			var id any = primitive.NewObjectID()
			if name == "rm_outbox" {
				id = bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: mongoCycleKey(uint64(1000 + i))}, {Key: "destination", Value: "qs.assessment.lifecycle"}}
			}
			docs = append(docs, bson.D{{Key: "_id", Value: id}})
		}
		if _, e := db.Collection(name).InsertMany(t.Context(), docs); e != nil {
			t.Fatal(e)
		}
		cur, e := db.Collection(name).Find(t.Context(), bson.D{}, options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}))
		if e != nil {
			t.Fatal(e)
		}
		var rows []bson.Raw
		if e = cur.All(t.Context(), &rows); e != nil {
			t.Fatal(e)
		}
		filter := mongoCycleRange(rows[0].Lookup("_id"), rows[99].Lookup("_id"), rows[79].Lookup("_id"))
		var explain struct {
			Stats struct {
				Keys     int64 `bson:"totalKeysExamined"`
				Docs     int64 `bson:"totalDocsExamined"`
				Returned int64 `bson:"nReturned"`
			} `bson:"executionStats"`
		}
		command := bson.D{{Key: "explain", Value: bson.D{{Key: "find", Value: name}, {Key: "filter", Value: filter}, {Key: "sort", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "hint", Value: "_id_"}, {Key: "limit", Value: int64(3)}, {Key: "collation", Value: bson.D{{Key: "locale", Value: "simple"}}}}}, {Key: "verbosity", Value: "executionStats"}}
		if e = db.RunCommand(t.Context(), command).Decode(&explain); e != nil {
			t.Fatal(e)
		}
		if explain.Stats.Returned != 3 || explain.Stats.Keys > 5 || explain.Stats.Docs > 5 {
			t.Fatalf("fixed BSON range is not an efficient actual _id range: keys=%d docs=%d returned=%d", explain.Stats.Keys, explain.Stats.Docs, explain.Stats.Returned)
		}
		t.Logf("safe_range_plan collection=%s keys_examined=%d docs_examined=%d returned=%d", name, explain.Stats.Keys, explain.Stats.Docs, explain.Stats.Returned)
	}
}
