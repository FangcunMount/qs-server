//go:build integration

package retirement

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"gorm.io/gorm"
)

// Full real framing/authentication: callers cannot set a completion boolean.
func mongoCASNativeCopies(t *testing.T, events ...event.DomainEvent) (*VerifiedSourceCopies, []*VerifiedSourceEvent) {
	t.Helper()
	var fixture authFixture
	var rows [][]byte
	for _, evt := range events {
		body, e := domainwire.EncodeEvent(evt)
		if e != nil {
			t.Fatal(e)
		}
		row := setMongoField(fixtureMongoRow(t, body, primitive.NewObjectID()), "org_id", int64(7))
		raw, e := bson.Marshal(row)
		if e != nil {
			t.Fatal(e)
		}
		rows = append(rows, raw)
	}
	fixture.raw[0], fixture.expected[0] = fixtureSQLCopy(t, [][][]byte{fixtureSQLRow(t, wireFixture(t, "evaluation.failed"), "1")}, nil)
	fixture.raw[1], fixture.expected[1] = aiFixtureCopy(t, AIBridgeCommandSource, nil, nil)
	fixture.raw[2], fixture.expected[2] = aiFixtureCopy(t, AILegacyCommandSource, nil, nil)
	fixture.raw[3], fixture.expected[3] = fixtureMongoCopy(t, rows)
	copies, e := VerifySourceCopies(t.Context(), fixture.inputs())
	if e != nil {
		t.Fatal(e)
	}
	reader, e := NewMongoSourceReader(bytes.NewReader(fixture.raw[3]), fixture.expected[3])
	if e != nil {
		t.Fatal(e)
	}
	handles := make([]*VerifiedSourceEvent, len(events))
	for i := range events {
		f, e := reader.Next()
		if e != nil {
			t.Fatal(e)
		}
		handles[i], e = copies.BindEvent(f)
		if e != nil {
			t.Fatal(e)
		}
	}
	return copies, handles
}
func mongoCASNativeHistoryIndexes(t *testing.T, db *mongo.Database) {
	t.Helper()
	for _, v := range []struct{ name, slot string }{{"answersheets", "legacy_submission_evidence"}, {"report_generations", "historical_generated_evidence"}} {
		key := v.slot + ".entries.event_id"
		_, e := db.Collection(v.name).Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: key, Value: 1}}, Options: options.Index().SetName("native_history_unique").SetUnique(true).SetPartialFilterExpression(bson.D{{Key: key, Value: bson.D{{Key: "$type", Value: "string"}}}}).SetCollation(&options.Collation{Locale: "simple"})})
		if e != nil {
			t.Fatal(e)
		}
	}
}
func mongoCASNativeEntry(t *testing.T, ctx context.Context, b *MongoHistoricalOwnerBatch, handle *VerifiedSourceEvent) evidence.HistoricalReferenceEntryV1 {
	t.Helper()
	source, e := handle.Facts()
	if e != nil {
		t.Fatal(e)
	}
	q, e := b.ResolveSource(ctx, handle)
	if e != nil {
		t.Fatal(e)
	}
	local := q.Local()
	class, reason := evidence.RetiredVerified, ""
	if strings.Contains(strings.Join(local.Gaps, ","), "storage_precision_gap") {
		class, reason = evidence.Unverifiable, "storage_precision_gap"
	}
	entry := evidence.HistoricalReferenceEntryV1{EventID: source.EventID, EventType: source.EventType, Source: source.Source, Run: local.OriginalRun, Proof: &evidence.EventEvidenceV1{Version: 1, Class: class, EventID: source.EventID, Digest: source.Source.Digest, BusinessBindingSHA256: local.BusinessBindingSHA256, Origin: "retirement", Verification: evidence.Verification{OperationID: "12345-1", Method: "native_compact_fact_test", Version: "v1", Reason: reason, VerifiedAt: time.Date(2026, 10, 8, 8, 0, 0, 123000000, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}}
	if e = entry.Validate(); e != nil {
		t.Fatal(e)
	}
	return entry
}
func mongoCASNativeSetup(t *testing.T, sqlDB *gorm.DB) (*mongo.Client, *mongo.Database, MongoOwnerConfig, *VerifiedSourceCopies, []*VerifiedSourceEvent, *MongoHistoricalBatchCASPlan) {
	t.Helper()
	client, db, cfg := mongoCycleNativeDB(t)
	mongoBatchNativeIndexes(t, db)
	mongoCASNativeHistoryIndexes(t, db)
	t.Log("owned_mongo_database=" + db.Name())
	row := mongoBatchNativeAssessmentSheet()
	insertMongoLocalSheet(t, db, row)
	submitted, e := mongoSubmissionPayload(row)
	if e != nil {
		t.Fatal(e)
	}
	generated, _ := mongoLocalGeneratedFixture(t, db)
	copies, handles := mongoCASNativeCopies(t, mongoBatchNativeEvent(t, submitted, "answersheet.submitted", "cas-sheet-1"), mongoBatchNativeEvent(t, submitted, "answersheet.submitted", "cas-sheet-2"), mongoBatchNativeEvent(t, generated, "interpretation.report.generated", "cas-generated-1"), mongoBatchNativeEvent(t, generated, "interpretation.report.generated", "cas-generated-2"))
	var plan *MongoHistoricalBatchCASPlan
	e = mongoBatchNativePair(t, sqlDB, client, db, cfg, handles, func(ctx context.Context, b *MongoHistoricalOwnerBatch, _ *MongoResponsibilitySnapshot, _ *sqlevaluation.SQLHistoricalOwnerBatch) error {
		inputs := make([]MongoHistoricalBatchAttachment, len(handles))
		for i, h := range handles {
			inputs[i] = MongoHistoricalBatchAttachment{Source: h, Entry: mongoCASNativeEntry(t, ctx, b, h)}
		}
		// Repeated input is idempotent; all distinct IDs remain in the same grouped write.
		inputs = append(inputs, inputs[0])
		var err error
		plan, err = PrepareMongoHistoricalBatchCAS(ctx, b, copies, inputs)
		return err
	})
	if e != nil {
		t.Fatal(e)
	}
	if len(plan.groups) != 2 || len(plan.attachments) != 4 {
		t.Fatal("sources not grouped/deduplicated")
	}
	return client, db, cfg, copies, handles, plan
}
func mongoCASNativeHostWrite(t *testing.T, client *mongo.Client, commit bool, fn func(mongo.SessionContext) error) error {
	t.Helper()
	s, e := client.StartSession()
	if e != nil {
		t.Fatal(e)
	}
	defer s.EndSession(t.Context())
	if e = s.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); e != nil {
		t.Fatal(e)
	}
	ctx := mongo.NewSessionContext(t.Context(), s)
	e = fn(ctx)
	if e == nil && commit {
		return s.CommitTransaction(t.Context())
	}
	// A server write-conflict can already have aborted the actual transaction.
	if abort := s.AbortTransaction(t.Context()); abort != nil && e == nil {
		return abort
	}
	return e
}
func mongoCASNativeVerify(t *testing.T, sqlDB *gorm.DB, client *mongo.Client, db *mongo.Database, cfg MongoOwnerConfig, handles []*VerifiedSourceEvent, s *MongoHistoricalBatchCASStatement) error {
	t.Helper()
	return mongoBatchNativePair(t, sqlDB, client, db, cfg, handles, func(ctx context.Context, b *MongoHistoricalOwnerBatch, _ *MongoResponsibilitySnapshot, _ *sqlevaluation.SQLHistoricalOwnerBatch) error {
		return s.VerifyPersisted(ctx, b)
	})
}
func mongoCASNativeCount(t *testing.T, db *mongo.Database, name, slot string) int {
	t.Helper()
	var raw bson.Raw
	if e := db.Collection(name).FindOne(t.Context(), bson.D{}).Decode(&raw); e != nil {
		t.Fatal(e)
	}
	if raw.Lookup("_retirement_batch_cas_fence").Type != 0 {
		t.Fatal("fence leaked after successful apply")
	}
	set, e := mongoCASSet(raw, slot)
	if e != nil {
		t.Fatal(e)
	}
	if set == nil {
		return 0
	}
	return len(set.Entries)
}

func TestMongoBatchCASNativeGroupedCommitIdempotenceAndFresh(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	var schema string
	if e := sqlDB.Raw("SELECT DATABASE()").Scan(&schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Log("owned_sql_database=" + schema)
	client, db, cfg, _, handles, p := mongoCASNativeSetup(t, sqlDB)
	var statement *MongoHistoricalBatchCASStatement
	if e := mongoCASNativeHostWrite(t, client, true, func(ctx mongo.SessionContext) error {
		var e error
		statement, e = p.Apply(ctx)
		if e == nil && statement.Report().HostCommitVerified {
			t.Fatal("statement claims commit")
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if mongoCASNativeCount(t, db, "answersheets", "legacy_submission_evidence") != 2 || mongoCASNativeCount(t, db, "report_generations", "historical_generated_evidence") != 2 {
		t.Fatal("lost original source ID")
	}
	if e := mongoCASNativeVerify(t, sqlDB, client, db, cfg, handles, statement); e != nil {
		t.Fatal(e)
	}
	if r := statement.Report(); !r.StatementApplied || len(r.Locations) != 4 || r.HostCommitVerified || r.BusinessClosureVerified || r.DropReady || !r.JointSQLRecheckRequired || !r.RangeAbsenceRecheckRequired {
		t.Fatal("unproven claims")
	}
	if e := mongoCASNativeHostWrite(t, client, true, func(ctx mongo.SessionContext) error { var e error; statement, e = p.Apply(ctx); return e }); e != nil {
		t.Fatal("exact expected-state replay", e)
	}
	for _, location := range statement.Report().Locations {
		if !location.AlreadyMatched || location.ReferenceAppended || !location.FenceWritePerformed {
			t.Fatal("idempotence receipt")
		}
	}
	if e := mongoCASNativeVerify(t, sqlDB, client, db, cfg, handles, statement); e != nil {
		t.Fatal(e)
	}
	if e := client.Ping(t.Context(), nil); e != nil {
		t.Fatal("borrowed client closed", e)
	}
}
func TestMongoBatchCASNativeRollbackAndUnknownCommit(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	var schema string
	if e := sqlDB.Raw("SELECT DATABASE()").Scan(&schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Log("owned_sql_database=" + schema)
	client, db, cfg, _, handles, p := mongoCASNativeSetup(t, sqlDB)
	var statement *MongoHistoricalBatchCASStatement
	if e := mongoCASNativeHostWrite(t, client, false, func(ctx mongo.SessionContext) error { var e error; statement, e = p.Apply(ctx); return e }); e != nil {
		t.Fatal(e)
	}
	if mongoCASNativeCount(t, db, "answersheets", "legacy_submission_evidence") != 0 || mongoCASNativeCount(t, db, "report_generations", "historical_generated_evidence") != 0 {
		t.Fatal("host rollback left proof")
	}
	if e := mongoCASNativeVerify(t, sqlDB, client, db, cfg, handles, statement); e == nil {
		t.Fatal("statement mistaken for persisted/committed")
	}
	if statement.Report().HostCommitVerified {
		t.Fatal("unknown commit promoted")
	}
}
func TestMongoBatchCASNativeBaselineAndDependencyConflict(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	var schema string
	if e := sqlDB.Raw("SELECT DATABASE()").Scan(&schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Log("owned_sql_database=" + schema)
	for _, kind := range []string{"clock_millisecond", "org", "standard_slot", "null_history", "new_field", "dependency_run", "numeric_bson_type", "index_schema", "collection_uuid"} {
		t.Run(kind, func(t *testing.T) {
			client, db, _, _, _, p := mongoCASNativeSetup(t, sqlDB)
			name, field, value := "answersheets", "filled_at", any(mongoLocalSheet().FilledAt.Add(time.Millisecond))
			switch kind {
			case "org":
				field, value = "org_id", int64(8)
			case "standard_slot":
				field, value = "durable_acceptance", bson.D{{Key: "event_id", Value: "different-standard-id"}}
			case "null_history":
				field, value = "legacy_submission_evidence", nil
			case "new_field":
				field, value = "future_unknown_field", true
			case "dependency_run":
				name, field, value = "interpretation_runs", "status", "failed"
			case "numeric_bson_type":
				field, value = "org_id", int32(7)
			case "index_schema":
				if _, e := db.Collection("answersheets").Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "filler_id", Value: 1}}, Options: options.Index().SetName("changed_schema")}); e != nil {
					t.Fatal(e)
				}
			case "collection_uuid":
				if e := db.Collection("answersheets").Drop(t.Context()); e != nil {
					t.Fatal(e)
				}
				if e := db.CreateCollection(t.Context(), "answersheets"); e != nil {
					t.Fatal(e)
				}
			}
			if kind != "index_schema" && kind != "collection_uuid" {
				if _, e := db.Collection(name).UpdateOne(t.Context(), bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: value}}}}); e != nil {
					t.Fatal(e)
				}
			}
			if e := mongoCASNativeHostWrite(t, client, false, func(ctx mongo.SessionContext) error {
				s, e := p.Apply(ctx)
				if s != nil {
					t.Fatal("changed baseline returned statement")
				}
				return e
			}); e == nil {
				t.Fatal("changed exact raw/schema accepted")
			}
		})
	}
}
func TestMongoBatchCASNativeConcurrentDependencyWriteConflict(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	var schema string
	if e := sqlDB.Raw("SELECT DATABASE()").Scan(&schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Log("owned_sql_database=" + schema)
	client, db, _, _, _, p := mongoCASNativeSetup(t, sqlDB)
	e := mongoCASNativeHostWrite(t, client, false, func(ctx mongo.SessionContext) error {
		// Establish the old real snapshot before the other host commits a Run write.
		if e := db.Collection("answersheets").FindOne(ctx, bson.D{}).Err(); e != nil {
			return e
		}
		if _, e := db.Collection("interpretation_runs").UpdateOne(t.Context(), bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "attempt", Value: int64(2)}}}}); e != nil {
			return e
		}
		s, e := p.Apply(ctx)
		if s != nil {
			t.Fatal("stale dependency snapshot wrote proof")
		}
		return e
	})
	if e == nil {
		t.Fatal("stale snapshot was sufficient CAS")
	}
	if mongoCASNativeCount(t, db, "answersheets", "legacy_submission_evidence") != 0 {
		t.Fatal("failed dependency fence committed proof")
	}
}
func TestMongoBatchCASNativeAttachmentAndSessionGuards(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	var schema string
	if e := sqlDB.Raw("SELECT DATABASE()").Scan(&schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Log("owned_sql_database=" + schema)
	client, db, cfg, copies, handles, p := mongoCASNativeSetup(t, sqlDB)
	if s, e := p.Apply(t.Context()); s != nil || e == nil {
		t.Fatal("no actual host session")
	}
	if e := mongoBatchNativePair(t, sqlDB, client, db, cfg, handles, func(ctx context.Context, b *MongoHistoricalOwnerBatch, _ *MongoResponsibilitySnapshot, _ *sqlevaluation.SQLHistoricalOwnerBatch) error {
		entries := []MongoHistoricalBatchAttachment{{Source: handles[0], Entry: mongoCASNativeEntry(t, ctx, b, handles[0])}}
		for _, kind := range []string{"source", "binding", "run", "proof_millisecond", "event_type", "same_id_different_conclusion", "not_all_four"} {
			input := entries[0]
			input.Entry = input.Entry.Clone()
			actualCopies := copies
			switch kind {
			case "source":
				facts, _ := handles[0].Facts()
				facts.OrgID = 8
				input.Source = &VerifiedSourceEvent{facts: facts}
			case "binding":
				input.Entry.Proof.BusinessBindingSHA256 = strings.Repeat("e", 64)
			case "run":
				input.Entry.Run = &evidence.HistoricalRunReferenceV1{RunID: "inferred-latest", Attempt: 1}
			case "proof_millisecond":
				input.Entry.Proof.Verification.VerifiedAt = input.Entry.Proof.Verification.VerifiedAt.Add(time.Nanosecond)
			case "event_type":
				input.Entry.EventType = "interpretation.report.generated"
			case "same_id_different_conclusion":
				input.Entry.Proof.Class = evidence.Unverifiable
				input.Entry.Proof.Verification.Reason = "different-conclusion"
			case "not_all_four":
				actualCopies = &VerifiedSourceCopies{}
			}
			inputs := []MongoHistoricalBatchAttachment{input}
			if kind == "same_id_different_conclusion" {
				inputs = append([]MongoHistoricalBatchAttachment{entries[0]}, inputs...)
			}
			if plan, e := PrepareMongoHistoricalBatchCAS(ctx, b, actualCopies, inputs); plan != nil || e == nil {
				t.Fatal("invalid attachment accepted", kind)
			}
		}
		oldPlan, e := PrepareMongoHistoricalBatchCAS(ctx, b, copies, entries)
		if e != nil {
			return e
		}
		if s, e := oldPlan.Apply(ctx); s != nil || !errors.Is(e, ErrMongoCycleTransaction) {
			t.Fatal("old snapshot reused for write", e)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	session, e := client.StartSession()
	if e != nil {
		t.Fatal(e)
	}
	defer session.EndSession(t.Context())
	if e = session.StartTransaction(); e != nil {
		t.Fatal(e)
	}
	if s, e := p.Apply(mongo.NewSessionContext(t.Context(), session)); s != nil || e == nil {
		t.Fatal("non-snapshot transaction")
	}
	if e = session.AbortTransaction(t.Context()); e != nil {
		t.Fatal(e)
	}
	p.expires = time.Now().Add(-time.Second)
	if _, e := p.Apply(t.Context()); e == nil {
		t.Fatal("expired plan accepted")
	}
}
func TestMongoBatchCASNativeServerObjectLiteralAndNumericSemantics(t *testing.T) {
	_, db, _ := mongoCycleNativeDB(t)
	t.Log("owned_mongo_database=" + db.Name())
	raw, e := bson.Marshal(bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "domain_id", Value: int64(1)}, {Key: "org_id", Value: int32(7)}, {Key: "expression_like", Value: "$not_a_field"}, {Key: "null_value", Value: nil}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Collection("answersheets").InsertOne(t.Context(), raw); e != nil {
		t.Fatal(e)
	}
	if n, e := db.Collection("answersheets").CountDocuments(t.Context(), mongoCASFilter(raw)); e != nil || n != 1 {
		t.Fatal("literal full object equality", e)
	}
	changed, e := mongoCASReplace(raw, "org_id", int64(7))
	if e != nil {
		t.Fatal(e)
	}
	if n, e := db.Collection("answersheets").CountDocuments(t.Context(), mongoCASFilter(changed)); e != nil || n != 1 {
		t.Fatal("server numeric equality behavior changed", e)
	}
	// Server object equality alone is weaker than raw type identity. The bridge
	// compares actual raw before a forced MVCC write; neither hash nor $eq alone
	// is advertised as a byte-exact server predicate.
	if bytes.Equal(changed, raw) {
		t.Fatal("type distinction missing")
	}
	nullChanged, e := mongoCASReplace(raw, "null_value", "")
	if e != nil {
		t.Fatal(e)
	}
	if n, e := db.Collection("answersheets").CountDocuments(t.Context(), mongoCASFilter(nullChanged)); e != nil || n != 0 {
		t.Fatal("NULL/empty indistinguishable", e)
	}
	reordered := bson.D{{Key: "_id", Value: bson.Raw(raw).Lookup("_id")}, {Key: "org_id", Value: int32(7)}, {Key: "domain_id", Value: int64(1)}, {Key: "expression_like", Value: "$not_a_field"}, {Key: "null_value", Value: nil}}
	rawReordered, e := bson.Marshal(reordered)
	if e != nil {
		t.Fatal(e)
	}
	if n, e := db.Collection("answersheets").CountDocuments(t.Context(), mongoCASFilter(rawReordered)); e != nil || n != 0 {
		t.Fatal("object field order unexpected", e)
	}
}

func TestMongoBatchCASNativeExistingSetMixedAppendAndDifferentProof(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	var schema string
	if e := sqlDB.Raw("SELECT DATABASE()").Scan(&schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Log("owned_sql_database=" + schema)
	client, db, cfg, copies, handles, _ := mongoCASNativeSetup(t, sqlDB)
	prepare := func(indices []int, changedProof bool) (*MongoHistoricalBatchCASPlan, error) {
		var result *MongoHistoricalBatchCASPlan
		e := mongoBatchNativePair(t, sqlDB, client, db, cfg, handles, func(ctx context.Context, b *MongoHistoricalOwnerBatch, _ *MongoResponsibilitySnapshot, _ *sqlevaluation.SQLHistoricalOwnerBatch) error {
			inputs := []MongoHistoricalBatchAttachment{}
			for _, i := range indices {
				entry := mongoCASNativeEntry(t, ctx, b, handles[i])
				if changedProof {
					entry.Proof.Verification.Method = "different_verified_method"
				}
				inputs = append(inputs, MongoHistoricalBatchAttachment{Source: handles[i], Entry: entry})
			}
			var err error
			result, err = PrepareMongoHistoricalBatchCAS(ctx, b, copies, inputs)
			return err
		})
		return result, e
	}
	first, e := prepare([]int{0, 2}, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = mongoCASNativeHostWrite(t, client, true, func(ctx mongo.SessionContext) error { _, e := first.Apply(ctx); return e }); e != nil {
		t.Fatal(e)
	}
	if p, e := prepare([]int{0}, true); p != nil || e == nil {
		t.Fatal("existing same ID different evidence overwritten")
	}
	next, e := prepare([]int{0, 1, 2, 3}, false)
	if e != nil {
		t.Fatal("valid existing set rejected", e)
	}
	var statement *MongoHistoricalBatchCASStatement
	if e = mongoCASNativeHostWrite(t, client, true, func(ctx mongo.SessionContext) error { var err error; statement, err = next.Apply(ctx); return err }); e != nil {
		t.Fatal(e)
	}
	oldCount, newCount := 0, 0
	for _, location := range statement.Report().Locations {
		if location.AlreadyMatched {
			oldCount++
		}
		if location.ReferenceAppended {
			newCount++
		}
	}
	if oldCount != 2 || newCount != 2 {
		t.Fatal("mixed receipt counts", oldCount, newCount)
	}
	if e = mongoCASNativeVerify(t, sqlDB, client, db, cfg, handles, statement); e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct{ name, slot string }{{"answersheets", "legacy_submission_evidence"}, {"report_generations", "historical_generated_evidence"}} {
		if mongoCASNativeCount(t, db, v.name, v.slot) != 2 {
			t.Fatal("old set not preserved")
		}
	}
}
func TestMongoBatchCASNativeIndependentReadbackRejectsPostCommitMutation(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	var schema string
	if e := sqlDB.Raw("SELECT DATABASE()").Scan(&schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Log("owned_sql_database=" + schema)
	client, db, cfg, _, handles, p := mongoCASNativeSetup(t, sqlDB)
	var statement *MongoHistoricalBatchCASStatement
	if e := mongoCASNativeHostWrite(t, client, true, func(ctx mongo.SessionContext) error { var e error; statement, e = p.Apply(ctx); return e }); e != nil {
		t.Fatal(e)
	}
	// A business version remains mutable under legitimate writers. Post-CAS
	// comparison still freezes it; proof fields never exempt that change.
	if _, e := db.Collection("report_generations").UpdateOne(t.Context(), bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "version", Value: int64(8)}}}}); e != nil {
		t.Fatal(e)
	}
	if e := mongoCASNativeVerify(t, sqlDB, client, db, cfg, handles, statement); e == nil {
		t.Fatal("postcommit business mutation exempted")
	}
	if statement.Report().HostCommitVerified || statement.Report().DropReady {
		t.Fatal("readback failure minted receipt")
	}
}

func TestMongoBatchCASNativeNewMongoSnapshotCannotReuseOldSQLTransaction(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	var schema string
	if e := sqlDB.Raw("SELECT DATABASE()").Scan(&schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Log("owned_sql_database=" + schema)
	client, db, cfg, copies, handles, _ := mongoCASNativeSetup(t, sqlDB)
	e := mongoBatchNativePair(t, sqlDB, client, db, cfg, handles, func(oldctx context.Context, b *MongoHistoricalOwnerBatch, _ *MongoResponsibilitySnapshot, oldSQL *sqlevaluation.SQLHistoricalOwnerBatch) error {
		tx, e := hostmysql.RequireTx(oldctx)
		if e != nil {
			return e
		}
		inputs := []MongoHistoricalBatchAttachment{{Source: handles[0], Entry: mongoCASNativeEntry(t, oldctx, b, handles[0])}}
		plan, e := PrepareMongoHistoricalBatchCAS(oldctx, b, copies, inputs)
		if e != nil {
			return e
		}
		var statement *MongoHistoricalBatchCASStatement
		e = mongoCASNativeHostWrite(t, client, true, func(ctx mongo.SessionContext) error { var err error; statement, err = plan.Apply(ctx); return err })
		if e != nil {
			return e
		}
		return mongoCycleNativeTx(t, client, func(freshctx mongo.SessionContext) error {
			ctx := mongo.NewSessionContext(hostmysql.WithTx(freshctx, tx), mongo.SessionFromContext(freshctx))
			global, e := PrepareMongoResponsibilitySnapshot(ctx, db, cfg, mongoCycleTestLimits())
			if e != nil {
				return e
			}
			fresh, e := PrepareMongoHistoricalOwnerBatch(ctx, global, oldSQL, handles, DefaultMongoHistoricalOwnerBatchLimits())
			if e != nil {
				return e
			}
			if e = statement.VerifyPersisted(ctx, fresh); !errors.Is(e, ErrMongoBatchCAS) {
				t.Fatal("old SQL snapshot accepted as independent pair", e)
			}
			return nil
		})
	})
	if e != nil {
		t.Fatal(e)
	}
}
