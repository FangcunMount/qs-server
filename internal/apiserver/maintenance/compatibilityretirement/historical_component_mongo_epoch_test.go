package retirement

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestMongoHistoricalComponentEpochCannotImportAuthority(t *testing.T) {
	if _, err := PrepareMongoHistoricalComponentObservation(context.Background(), nil, &HistoricalCASComponent{}, &MongoSnapshotInputEpoch{}, time.Second); err == nil {
		t.Fatal("unminted physical inputs produced transaction authority")
	}
	o := &MongoHistoricalComponentObservation{}
	if _, err := json.Marshal(o); err == nil {
		t.Fatal("private physical observation serialized")
	}
	r := o.Summary()
	if r.ActualTransactionRead || r.FrozenBaselinesMatched || r.CASAuthorized || r.HostCommitVerified || r.DropReady || !r.FullSourcesRequired || !r.SQLQualificationRequired || !r.AIClosureRequired {
		t.Fatal("zero physical observation claimed completion", r)
	}
	if o.VerifyIndependentRead(context.Background(), o) == nil {
		t.Fatal("same unminted object became server readback")
	}
}

func TestMongoHistoricalComponentRecipeMutationInvalidatesFrozenInput(t *testing.T) {
	_, frames := componentFixture(t, 1)
	f := frames[0]
	f.mongoRead = &mongoHistoricalComponentReadRecipe{selection: mongoBatchSelection{sheets: map[uint64]bool{42: true}}, hints: map[string]string{"answersheets:domain_id": "real-index"}, limits: DefaultMongoHistoricalOwnerBatchLimits()}
	f.seal = f.digest()
	for _, mutate := range []func(){func() { f.mongoRead.selection.sheets[99] = true }, func() { f.mongoRead.hints["answersheets:domain_id"] = "another-index" }, func() { f.mongoRead.original.number++ }} {
		old := f.seal
		mutate()
		if old == f.digest() {
			t.Fatal("mutable range/native selection did not change frozen seal")
		}
		f.seal = f.digest()
	}
}

func TestMongoHistoricalComponentPhysicalRangeRejectsMissingNewAndChangedRows(t *testing.T) {
	raw, err := bson.Marshal(bson.D{{Key: "domain_id", Value: int64(42)}, {Key: "status", Value: "terminal"}})
	if err != nil {
		t.Fatal(err)
	}
	f := &HistoricalCASComponentInput{rows: []historicalCASRowInput{{historicalCASRowKey{"mongodb", "answersheets", 42}, historicalSpoolSHA(raw), uint64(len(raw)), false}}}
	if err := mongoHistoricalComponentRowsMatch(f, map[string][]bson.Raw{"answersheets": {raw}}); err != nil {
		t.Fatal(err)
	}
	changed, err := bson.Marshal(bson.D{{Key: "domain_id", Value: int64(42)}, {Key: "status", Value: "pending"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []map[string][]bson.Raw{{"answersheets": nil}, {"answersheets": {changed}}, {"answersheets": {raw, raw}}, {"answersheets": {raw}, "report_generations": {raw}}} {
		if mongoHistoricalComponentRowsMatch(f, data) == nil {
			t.Fatal("missing/new/changed actual range was accepted")
		}
	}
}
