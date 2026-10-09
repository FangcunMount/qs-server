package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func mongoCycleTestLimits() MongoResponsibilityLimits {
	return MongoResponsibilityLimits{PageRows: 2, MaxRows: 10_000, MaxBytes: 64 << 20, MaxPages: 10_000, MaxGraphEntries: 30_000, MaxGraphBytes: 64 << 20, MaxDuration: time.Minute}
}

func TestMongoCycleLimitsAreExplicitAndHard(t *testing.T) {
	if (MongoResponsibilityLimits{}).valid() {
		t.Fatal("empty budgets accepted")
	}
	l := mongoCycleTestLimits()
	if !l.valid() {
		t.Fatal("valid host budgets refused")
	}
	for _, change := range []func(*MongoResponsibilityLimits){func(l *MongoResponsibilityLimits) { l.MaxRows = 2_000_001 }, func(l *MongoResponsibilityLimits) { l.MaxBytes = 8<<30 + 1 }, func(l *MongoResponsibilityLimits) { l.MaxPages = 0 }, func(l *MongoResponsibilityLimits) { l.PageRows = 4097 }, func(l *MongoResponsibilityLimits) { l.MaxGraphEntries = 4_000_001 }, func(l *MongoResponsibilityLimits) { l.MaxGraphBytes = 1<<30 + 1 }, func(l *MongoResponsibilityLimits) { l.MaxDuration = 6 * time.Minute }} {
		n := l
		change(&n)
		if n.valid() {
			t.Fatal("oversized or absent cap accepted")
		}
	}
}

func TestMongoCycleRawOrderedTokenIsNotStringified(t *testing.T) {
	first, _ := bson.Marshal(bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "10"}, {Key: "destination", Value: "qs.a"}})
	reordered, _ := bson.Marshal(bson.D{{Key: "message_id", Value: "10"}, {Key: "producer", Value: "qs-server"}, {Key: "destination", Value: "qs.a"}})
	a := bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: first}
	b := bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: reordered}
	if mongoCycleToken(a) == mongoCycleToken(b) {
		t.Fatal("order flattened")
	}
	id := primitive.NewObjectID()
	lower := bson.RawValue{Type: bson.TypeObjectID, Value: id[:]}
	upper := mongoCycleCloneValue(lower)
	encoded, e := bson.Marshal(mongoCycleRange(lower, upper, a))
	if e != nil {
		t.Fatal(e)
	}
	encodedID := bson.Raw(encoded).Lookup("_id").Document()
	if !bytes.Equal(encodedID.Lookup("$gt").Document(), first) {
		t.Fatal("composite cursor changed")
	}
	upper.Value[0] ^= 1
	if bytes.Equal(lower.Value, upper.Value) {
		t.Fatal("token copy aliases input")
	}
}

func TestMongoCycleOpaqueViewsCannotBeSerialized(t *testing.T) {
	for _, v := range []any{&MongoResponsibilitySnapshot{}, MongoResponsibilityObservation{}, MongoSourceResponsibilityView{}} {
		if _, e := json.Marshal(v); e == nil {
			t.Fatal("private graph serialized")
		}
	}
	r := (&MongoResponsibilitySnapshot{}).Report()
	if r.DropReady {
		t.Fatal("empty snapshot DROP ready")
	}
}

func TestMongoCycleReportReturnsDefensiveCollectionsAndCounts(t *testing.T) {
	s := &MongoResponsibilitySnapshot{report: MongoResponsibilityCycleReport{Collections: []MongoResponsibilityCollectionReport{{Collection: "answersheets"}}, ClassCounts: map[string]uint64{"current": 1}, BlockingReasons: []string{"fixed"}, CoverageGaps: []string{"sql"}}}
	r := s.Report()
	r.Collections[0].Collection = "bad"
	r.ClassCounts["current"] = 99
	r.BlockingReasons[0] = "bad"
	r.CoverageGaps[0] = "bad"
	actual := s.Report()
	if actual.Collections[0].Collection != "answersheets" || actual.ClassCounts["current"] != 1 || actual.BlockingReasons[0] != "fixed" || actual.CoverageGaps[0] != "sql" {
		t.Fatal("report aliases private snapshot")
	}
}

func TestMongoCycleLookupIsPureIndexAndStillUntrusted(t *testing.T) {
	s := &MongoResponsibilitySnapshot{report: MongoResponsibilityCycleReport{Complete: true}, observations: []MongoResponsibilityObservation{{Collection: "rm_outbox", OrgID: 7, EventID: "current", EventType: "answersheet.submitted", OwnerKind: "AnswerSheet", OwnerID: "42", Unfinished: true}}, bySheet: map[string][]int{"42": {0}}}
	// db is nil: any database call here would fail. This API only joins the
	// completed indexes; transaction validation occurs at batch boundaries.
	v, e := s.ForUntrustedSource(context.Background(), &DecodedSourceEvent{EventID: "old", EventType: "answersheet.submitted", OrgID: 7, AggregateType: "AnswerSheet", AggregateID: "42"})
	if e != nil || len(v.BlockingReasons) == 0 || !v.SourceAuthenticationRequired || !v.SQLAIInboxCoverageRequired || !v.WriterFenceRequired || !v.CASRequired {
		t.Fatal("lookup omitted linked responsibility or promoted approval", e)
	}
}
