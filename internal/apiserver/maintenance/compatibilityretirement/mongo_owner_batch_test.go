package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"go.mongodb.org/mongo-driver/bson"
)

var _ mongoSQLBusinessFacts = (*sqlevaluation.SQLHistoricalBatchOwnerFacts)(nil)
var _ mongoSQLBusinessFacts = (*sqlevaluation.SQLHistoricalOwnerFacts)(nil)

func TestMongoBatchLimitsNeverAuthorizeTruncation(t *testing.T) {
	limits := DefaultMongoHistoricalOwnerBatchLimits()
	if !limits.valid() {
		t.Fatal("valid explicit limits refused")
	}
	for _, change := range []func(*MongoHistoricalOwnerBatchLimits){func(l *MongoHistoricalOwnerBatchLimits) { l.MaxSources = 0 }, func(l *MongoHistoricalOwnerBatchLimits) { l.MaxSources = 513 }, func(l *MongoHistoricalOwnerBatchLimits) { l.MaxRows = 0 }, func(l *MongoHistoricalOwnerBatchLimits) { l.MaxRows = 131073 }, func(l *MongoHistoricalOwnerBatchLimits) { l.MaxBytes = 0 }, func(l *MongoHistoricalOwnerBatchLimits) { l.MaxBytes = 256<<20 + 1 }, func(l *MongoHistoricalOwnerBatchLimits) { l.MaxDuration = 0 }, func(l *MongoHistoricalOwnerBatchLimits) { l.MaxDuration = 2*time.Minute + 1 }} {
		copy := limits
		change(&copy)
		if copy.valid() {
			t.Fatal("unsafe or unbounded limits accepted")
		}
	}
}

func TestMongoBatchSelectorsRequireVerifiedFactsAndPreserveLargeIDs(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	index, err := VerifySourceCopies(context.Background(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	a, err := index.BindEvent(f.sql)
	if err != nil {
		t.Fatal(err)
	}
	b, err := index.BindEvent(f.mongo)
	if err != nil {
		t.Fatal(err)
	}
	request, err := MongoHistoricalSQLBatchSelectors([]*VerifiedSourceEvent{a, a})
	if err != nil {
		t.Fatal(err)
	}
	if len(request.AssessmentIDs) != 1 || len(request.AnswerSheetIDs) != 0 || request.AssessmentIDs[0] <= 1<<53 {
		t.Fatal("original integer IDs were rounded or duplicated")
	}
	if _, err = MongoHistoricalSQLBatchSelectors([]*VerifiedSourceEvent{b}); err == nil {
		t.Fatal("noncanonical domain selector accepted")
	}
	if _, err = MongoHistoricalSQLBatchSelectors([]*VerifiedSourceEvent{nil}); err == nil {
		t.Fatal("unverified empty handle accepted")
	}
}

func TestMongoBatchOpaqueQualificationCannotExposeOldRecheck(t *testing.T) {
	for _, value := range []any{&MongoHistoricalOwnerBatch{}, &MongoHistoricalBatchOwnerQualification{}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private business page serialized")
		}
		if _, err := bson.Marshal(value); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private BSON serialized")
		}
	}
	typeOf := reflect.TypeOf(&MongoHistoricalBatchOwnerQualification{})
	if _, exists := typeOf.MethodByName("RecheckSQL"); exists {
		t.Fatal("legacy FOR UPDATE-capable recheck exposed")
	}
	q := &MongoHistoricalBatchOwnerQualification{local: MongoLocalResolution{BlockingReasons: []string{"private_reason"}, SQLSubmissionClock: &MongoSQLSubmissionClockFact{Precision: 0}}}
	v := q.Local()
	v.BlockingReasons[0] = "changed"
	v.SQLSubmissionClock.Precision = 6
	if q.Local().BlockingReasons[0] != "private_reason" || q.Local().SQLSubmissionClock.Precision != 0 {
		t.Fatal("qualification clone aliased private baseline")
	}
}

func TestMongoBatchSourceFactsHashRejectsTypedTampering(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	index, err := VerifySourceCopies(context.Background(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	handle, err := index.BindEvent(f.mongo)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := handle.Facts()
	if err != nil {
		t.Fatal(err)
	}
	key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := privateFactsSHA(facts)
	if err != nil {
		t.Fatal(err)
	}
	b := &MongoHistoricalOwnerBatch{sources: map[verifiedSourceKey]*DecodedSourceEvent{key: facts}, sourceFactsSHA: map[verifiedSourceKey][32]byte{key: digest}}
	if _, _, err = b.source(handle); err != nil {
		t.Fatal(err)
	}
	changed, err := handle.Facts()
	if err != nil {
		t.Fatal(err)
	}
	changed.Submitted.QuestionnaireCode = "private_changed"
	if _, _, err = b.source(&VerifiedSourceEvent{facts: changed}); err != ErrMongoBatchConflict {
		t.Fatal("same digest but changed typed source accepted")
	}
}

func TestMongoBatchRequiresActualCompleteUncollatedIndexes(t *testing.T) {
	for _, kind := range []string{"valid", "partial", "sparse", "collated", "not_unique", "composite_unique"} {
		t.Run(kind, func(t *testing.T) {
			idx := bson.D{{Key: "name", Value: "domain_index"}, {Key: "key", Value: bson.D{{Key: "domain_id", Value: 1}}}, {Key: "unique", Value: true}}
			switch kind {
			case "partial":
				idx = append(idx, bson.E{Key: "partialFilterExpression", Value: bson.D{{Key: "domain_id", Value: bson.D{{Key: "$gt", Value: 0}}}}})
			case "sparse":
				idx = append(idx, bson.E{Key: "sparse", Value: true})
			case "collated":
				idx = append(idx, bson.E{Key: "collation", Value: bson.D{{Key: "locale", Value: "en"}}})
			case "not_unique":
				idx[2].Value = false
			case "composite_unique":
				idx[1].Value = bson.D{{Key: "domain_id", Value: 1}, {Key: "org_id", Value: 1}}
			}
			raw, err := bson.Marshal(idx)
			if err != nil {
				t.Fatal(err)
			}
			b := &MongoHistoricalOwnerBatch{global: &MongoResponsibilitySnapshot{metadata: mongoCycleMetadata{definitions: map[string]mongoCycleDefinition{"answersheets": {indexes: []bson.Raw{raw}}}}}}
			_, err = b.index("answersheets", "domain_id", true)
			if (err == nil) != (kind == "valid") {
				t.Fatal("metadata index coverage was guessed")
			}
		})
	}
}
