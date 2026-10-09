package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestSourceOriginAnchorNoDTOConstructorGraphsOrAuthority(t *testing.T) {
	for _, epoch := range []*SourceOriginEpoch{nil, {}, {hash: "editable-report"}} {
		if anchor, err := epoch.FreezeFreshRecheckAnchor(t.Context()); err == nil || anchor != nil {
			t.Fatal("unvalidated epoch minted original anchor")
		}
	}
	for _, anchor := range []*FreshRecheckAnchor{nil, {}} {
		if proof, err := anchor.Recheck(t.Context(), nil, nil, nil); err == nil || proof != nil {
			t.Fatal("zero/DTO anchor minted fresh proof")
		}
		if _, err := json.Marshal(anchor); anchor != nil && !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private anchor serialized as JSON")
		}
		if _, err := bson.Marshal(anchor); anchor != nil && !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private anchor serialized as BSON")
		}
		if strings.Contains(anchor.String(), "identity") {
			t.Fatal("diagnostic string exposed private facts")
		}
	}
	var decoded FreshRecheckAnchor
	if err := json.Unmarshal([]byte(`{"first_epoch":"editable-summary"}`), &decoded); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("JSON DTO accepted as original capability")
	}
	raw, err := bson.Marshal(bson.D{{Key: "first_epoch", Value: "editable-summary"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bson.Unmarshal(raw, &decoded); !errors.Is(err, ErrSourceSerialization) {
		t.Fatal("BSON DTO accepted as original capability")
	}
	typ := reflect.TypeFor[FreshRecheckAnchor]()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.IsExported() || strings.Contains(field.Type.String(), "SQLHistoricalResponsibilityCycle") || strings.Contains(field.Type.String(), "MongoResponsibilitySnapshot") || strings.Contains(field.Type.String(), "SourceOriginEpoch") {
			t.Fatal("anchor exposes editable facts or retains old row graph", field.Name)
		}
	}
	r := (&SourceOriginRecheckProof{firstAnchor: &FreshRecheckAnchor{}, second: &SourceOriginEpoch{}}).Report()
	if r.ActualOriginMatched || r.SourceFilesMatched || r.IndependentEpochRechecked || r.CASAuthorized || r.DropReady || !r.IndependentApprovalRequired || !r.WriterFenceRequired || !r.HostProcessBudgetRequired || !r.FirstAuthMetadataContinuityUnproven {
		t.Fatal("zero anchor promoted authority or removed gaps")
	}
}

func TestSourceOriginAnchorRequiresActualScopeAndPreservesBindingExpiry(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	copies, err := VerifySourceCopies(t.Context(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BindOriginCopies(t.Context(), copies, f.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		t.Fatal(err)
	}
	e := &SourceOriginEpoch{binding: binding, hash: "supplied-summary"}
	if a, err := e.FreezeFreshRecheckAnchor(t.Context()); err == nil || a != nil {
		t.Fatal("authenticated files alone replaced actual borrowed snapshots")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if a, err := e.FreezeFreshRecheckAnchor(ctx); err == nil || a != nil {
		t.Fatal("cancelled scope minted an anchor")
	}
	if !binding.started.Equal(e.binding.started) || binding.limits != DefaultSourceOriginLimits() {
		t.Fatal("freezing renewed the original binding lifetime")
	}
}
