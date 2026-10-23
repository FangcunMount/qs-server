package migration

import "testing"

func TestDeferredConsumerResourcePairRemainsExplicit(t *testing.T) {
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	if r.sql.head != 110 || r.mongo.head != 43 || !r.accepts(109, 42) || !r.accepts(108, 42) || r.accepts(109, 41) || r.accepts(110, 42) {
		t.Fatal("current deferred consumer target or prior schema boundary changed")
	}
}
