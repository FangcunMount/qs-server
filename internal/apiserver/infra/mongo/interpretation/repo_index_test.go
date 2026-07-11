package interpretation

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestReportIndexesProtectSingleLifecycleAndGeneratedQueries(t *testing.T) {
	indexes := reportIndexModels()
	if len(indexes) != 2 {
		t.Fatalf("indexes = %d", len(indexes))
	}
	if !reflect.DeepEqual(indexes[0].Keys, bson.D{{Key: "domain_id", Value: 1}, {Key: "deleted_at", Value: 1}}) || indexes[0].Options == nil || indexes[0].Options.Unique == nil || !*indexes[0].Options.Unique {
		t.Fatalf("lifecycle identity index = %#v", indexes[0])
	}
	if !reflect.DeepEqual(indexes[1].Keys, bson.D{{Key: "status", Value: 1}, {Key: "testee_id", Value: 1}, {Key: "created_at", Value: -1}}) {
		t.Fatalf("status query index = %#v", indexes[1])
	}
}
