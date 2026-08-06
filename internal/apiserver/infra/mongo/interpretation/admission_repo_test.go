package interpretation

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestAdmissionFailureUpsertOperatorsDoNotOverlap(t *testing.T) {
	t.Parallel()

	update, err := admissionFailureUpsertDocument(&AdmissionFailurePO{
		DomainID: 1, Fingerprint: "fingerprint", Attempt: 3,
		TraceID: "trace", LastFailedAt: time.Unix(100, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	onInsert := update["$setOnInsert"].(bson.M)
	inc := update["$inc"].(bson.M)
	set := update["$set"].(bson.M)
	for operator, fields := range map[string]bson.M{"$inc": inc, "$set": set} {
		for field := range fields {
			if _, exists := onInsert[field]; exists {
				t.Fatalf("field %q overlaps $setOnInsert and %s", field, operator)
			}
		}
	}
	for field := range inc {
		if _, exists := set[field]; exists {
			t.Fatalf("field %q overlaps $inc and $set", field)
		}
	}
	if inc["attempt"] != 1 || set["trace_id"] != "trace" {
		t.Fatalf("update = %#v", update)
	}
}
