package retirement

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMongoHistoricalComponentCASNoImportedOrEmptyAuthority(t *testing.T) {
	if s, err := applyMongoHistoricalComponent(context.Background(), nil); s != nil || err == nil {
		t.Fatal("empty physical observation performed an effect")
	}
	if r, err := freezeMongoHistoricalComponentCASRecipe(context.Background(), nil, &MongoHistoricalBatchCASPlan{}); r != nil || err == nil {
		t.Fatal("empty original plan imported as frozen recipe")
	}
	var s *MongoHistoricalComponentStatement
	r := s.Summary()
	if r.StatementApplied || r.HostCommitVerified || r.BusinessClosureVerified || r.DropReady || !r.FullSourcesRequired || !r.SQLQualificationRequired || !r.AIClosureRequired || !r.HostCommitRequired || !r.IndependentReadbackRequired {
		t.Fatal("physical sub-store fabricated completion", r)
	}
	if err := s.VerifyIndependentPersisted(context.Background(), nil, 0); err == nil {
		t.Fatal("empty physical statement verified")
	}
	if _, err := json.Marshal(&MongoHistoricalComponentStatement{}); err == nil {
		t.Fatal("physical statement serialized")
	}
}

func TestMongoHistoricalComponentCASRecipeMutationRejectsComponent(t *testing.T) {
	_, frames := componentFixture(t, 1)
	f := frames[0]
	r := &mongoHistoricalComponentCASRecipe{identity: "original-physical-identity", sqlIdentity: "sql-identity", sqlRows: "sql-rows"}
	f.mongoCAS = r
	f.seal = f.digest()
	original := f.seal
	r.sqlRows = "changed-physical-baseline"
	if original == f.digest() {
		t.Fatal("frozen write recipe is absent from immutable component identity")
	}
}
