package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"reflect"
	"testing"
)

func TestMongoBatchCASRawOnlySlotPreservesMissingNullTypesOrder(t *testing.T) {
	original, e := bson.Marshal(bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "n32", Value: int32(7)}, {Key: "n64", Value: int64(7)}, {Key: "physical_null", Value: nil}, {Key: "standard", Value: bson.D{{Key: "event_id", Value: "standard-original"}}}, {Key: "expression_like", Value: "$field"}})
	if e != nil {
		t.Fatal(e)
	}
	next, e := mongoCASReplace(original, "legacy_submission_evidence", bson.D{{Key: "version", Value: 1}})
	if e != nil {
		t.Fatal(e)
	}
	oldElements, _ := bson.Raw(original).Elements()
	newElements, _ := next.Elements()
	if len(newElements) != len(oldElements)+1 {
		t.Fatal("replacement rewrote PO")
	}
	for i, old := range oldElements {
		if !bytes.Equal(old, newElements[i]) {
			t.Fatal("non-maintenance BSON changed")
		}
	}
	clone := mongoCASCloneData(map[string][]bson.Raw{"answersheets": {original}, "interpretation_runs": nil})
	clone["answersheets"][0][0] ^= 1
	if bytes.Equal(clone["answersheets"][0], original) || clone["interpretation_runs"] != nil {
		t.Fatal("baseline aliases or changes absence")
	}
	filter := mongoCASFilter(original)
	raw, e := bson.Marshal(filter)
	if e != nil {
		t.Fatal(e)
	}
	literal := bson.Raw(raw).Lookup("$expr").Document().Lookup("$eq").Array().Index(1).Value().Document().Lookup("$literal")
	if !bytes.Equal(literal.Document(), original) {
		t.Fatal("server expression not literal full raw")
	}
}
func TestMongoBatchCASSchemaMalformedIndexNeverPanics(t *testing.T) {
	for _, doc := range []bson.D{{{Key: "key", Value: "wrong"}}, {{Key: "key", Value: bson.D{{Key: "legacy_submission_evidence.entries.event_id", Value: 1}}}, {Key: "unique", Value: true}}, {{Key: "key", Value: bson.D{{Key: "legacy_submission_evidence.entries.event_id", Value: 1}}}, {Key: "unique", Value: true}, {Key: "partialFilterExpression", Value: true}}} {
		raw, e := bson.Marshal(doc)
		if e != nil {
			t.Fatal(e)
		}
		m := mongoCycleMetadata{definitions: map[string]mongoCycleDefinition{"answersheets": {indexes: []bson.Raw{raw}}}}
		if mongoCASHistoryIndex(m, "answersheets", "legacy_submission_evidence") {
			t.Fatal("invalid index accepted")
		}
	}
}
func TestMongoBatchCASNoEmptyCapabilityOrSerialization(t *testing.T) {
	if p, e := PrepareMongoHistoricalBatchCAS(context.Background(), nil, nil, nil); p != nil || e == nil {
		t.Fatal("empty prepare accepted")
	}
	var p *MongoHistoricalBatchCASPlan
	if s, e := p.Apply(context.Background()); s != nil || e == nil {
		t.Fatal("empty plan accepted")
	}
	var s *MongoHistoricalBatchCASStatement
	r := s.Report()
	if r.StatementApplied || r.HostCommitVerified || r.DropReady || !r.RangeAbsenceRecheckRequired {
		t.Fatal(r)
	}
	for _, value := range []any{&MongoHistoricalBatchCASPlan{}, &MongoHistoricalBatchCASStatement{}} {
		if _, e := json.Marshal(value); e == nil {
			t.Fatal("private handle serialized")
		}
	}
	if !reflect.DeepEqual(mongoCASCloneSelection(mongoBatchSelection{map[uint64]bool{1: true}, map[uint64]bool{}, map[uint64]bool{}, map[uint64]bool{}, map[uint64]bool{}}).sheets, map[uint64]bool{1: true}) {
		t.Fatal("selection")
	}
}
