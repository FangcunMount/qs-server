package interpretation

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestLifecycleIndexesProtectThreeObjectIdentities(t *testing.T) {
	generationIndexes := generationIndexModels()
	assertUniqueIndex(t, generationIndexes, "uk_generation_domain_id", bson.D{{Key: "domain_id", Value: 1}})
	assertUniqueIndex(t, generationIndexes, "uk_generation_key", bson.D{{Key: "outcome_id", Value: 1}, {Key: "report_type", Value: 1}, {Key: "template_version", Value: 1}})

	runIndexes := runIndexModels()
	assertUniqueIndex(t, runIndexes, "uk_interpretation_run_domain_id", bson.D{{Key: "domain_id", Value: 1}})
	assertUniqueIndex(t, runIndexes, "uk_interpretation_run_generation_attempt", bson.D{{Key: "generation_id", Value: 1}, {Key: "attempt", Value: 1}})

	artifactIndexes := reportIndexModels()
	assertUniqueIndex(t, artifactIndexes, "uk_artifact_domain_id", bson.D{{Key: "domain_id", Value: 1}})
	assertUniqueIndex(t, artifactIndexes, "uk_artifact_generation_id", bson.D{{Key: "generation_id", Value: 1}})
}

func assertUniqueIndex(t *testing.T, indexes []mongo.IndexModel, name string, keys bson.D) {
	t.Helper()
	for _, index := range indexes {
		if index.Options != nil && index.Options.Name != nil && *index.Options.Name == name {
			if index.Options.Unique == nil || !*index.Options.Unique || !reflect.DeepEqual(index.Keys, keys) {
				t.Fatalf("index %s = %#v", name, index)
			}
			return
		}
	}
	t.Fatalf("index %s not found", name)
}
