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

func TestMongoHistoricalSnapshotRecipeRetainsInputOnlyOrigin(t *testing.T) {
	ctx := t.Context()
	for _, input := range []*MongoSnapshotInputEpoch{nil, {}} {
		if r, err := freezeMongoSnapshotOwnerComponentReadRecipe(ctx, input, &MongoSnapshotOwnerFootprint{}); r != nil || err == nil {
			t.Fatal("unminted snapshot input produced a read recipe")
		}
	}
	r := &mongoHistoricalComponentReadRecipe{original: mongoCycleTxn{session: bson.Raw("old-native-session"), number: 7}}
	if r.matchesOriginalInput(ctx, nil, r.original) || !r.matchesOriginalInput(ctx, nil, mongoCycleTxn{session: r.original.session, number: 8}) {
		t.Fatal("original native transaction tuple guard changed")
	}
	r.snapshotEpoch, r.snapshotInput, r.snapshotOwner = historicalSpoolSHA([]byte("actual-snapshot")), historicalSpoolSHA([]byte("actual-input")), historicalSpoolSHA([]byte("actual-owner"))
	if r.matchesOriginalInput(ctx, &MongoSnapshotInputEpoch{}, mongoCycleTxn{}) {
		t.Fatal("snapshot recipe invented an original transaction")
	}
	r.original = mongoCycleTxn{}
	original := &MongoSnapshotInputEpoch{dev: 1, ino: 2, complete: true}
	original.self = original
	r.snapshotOriginalInput, r.snapshotDev, r.snapshotIno = original, original.dev, original.ino
	_, frames := componentFixture(t, 1)
	f := frames[0]
	f.mongoRead = r
	f.mongoRead.limits = DefaultMongoHistoricalOwnerBatchLimits()
	f.seal = f.digest()
	c := &HistoricalCASComponent{inputs: frames}
	if mongoHistoricalComponentInputSeal(c) == "" {
		t.Fatal("pure read recipe lost graph footprint")
	}
	f.mongoCAS = &mongoHistoricalComponentCASRecipe{}
	f.seal = f.digest()
	if mongoHistoricalComponentInputSeal(c) != "" {
		t.Fatal("snapshot-only recipe entered the old physical write path")
	}
}

func TestMongoHistoricalSnapshotFootprintSealIncludesRowsRangesFactsAndBounds(t *testing.T) {
	fixture := sourceAuthFixture(t, false, true)
	facts := fixture.mongo
	key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
	if err != nil {
		t.Fatal(err)
	}
	factsSHA, err := privateFactsSHA(facts)
	if err != nil {
		t.Fatal(err)
	}
	original := &MongoSnapshotInputEpoch{dev: 1, ino: 2, complete: true}
	original.self = original
	f := &MongoSnapshotOwnerFootprint{originalInput: original, originalDev: original.dev, originalIno: original.ino, limits: DefaultMongoHistoricalOwnerBatchLimits(), sources: map[verifiedSourceKey]*DecodedSourceEvent{key: facts}, sourceFactsSHA: map[verifiedSourceKey][32]byte{key: factsSHA}, sqlAbsent: map[verifiedSourceKey]bool{key: true}, selection: mongoCASCloneSelection(mongoBatchSelection{}), data: map[string][]bson.Raw{}}
	for _, name := range mongoBatchBusinessCollections {
		f.data[name] = nil
	}
	f.self, f.inputSHA = f, f.digest()
	if f.InputSHA256() == "" {
		t.Fatal("private algorithm fixture failed to seal")
	}
	other := *original
	other.self = &other
	if f.matchesOriginalInput(&other) {
		t.Fatal("different input instance with equal snapshot/native hashes and FD numbers accepted")
	}
	other = *original
	if f.matchesOriginalInput(&other) {
		t.Fatal("copied original input retained native producer identity")
	}
	original.ino++
	if f.InputSHA256() != "" {
		t.Fatal("replaced original FD retained footprint seal")
	}
	original.ino--
	copy := *f
	if copy.InputSHA256() != "" {
		t.Fatal("copied object retained original producer identity")
	}
	for _, mutate := range []func(){func() { f.limits.MaxRows-- }, func() { f.selection.sheets[42] = true }, func() { f.data["answersheets"] = []bson.Raw{bson.Raw("changed-physical-input")} }, func() { f.sources[key].Submitted.QuestionnaireCode += "changed" }} {
		mutate()
		if f.InputSHA256() != "" {
			t.Fatal("changed input bytes/selectors/source/bounds retained frozen seal")
		}
		f.inputSHA = f.digest()
		if f.inputSHA == "" {
			break // A source-facts conflict cannot be resealed by the producer.
		}
	}
}
