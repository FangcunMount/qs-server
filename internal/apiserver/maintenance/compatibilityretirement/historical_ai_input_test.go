package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestAIHistoricalInputHasNoImportedAuthorityAndPreservesRawNull(t *testing.T) {
	for _, v := range []any{&AIHistoricalInputEpoch{}, &AIHistoricalInputPair{}} {
		if _, err := json.Marshal(v); err == nil {
			t.Fatal("private input serialized")
		}
	}
	r := (*AIHistoricalInputEpoch)(nil).Summary()
	if r.CompleteInput || r.CASAuthority || r.DropReady || !r.FreshComponentQualificationRequired || !r.StoredWireAuthenticationRequired {
		t.Fatal("absent input advertised authority", r)
	}
	if _, err := CompareIndependentAIHistoricalInputs(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("unproven inputs accepted")
	}
	in := []aiReverseRow{{"null": nil, "empty": {}, "binary": {0xff, 0x00, 0x80}}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out []aiReverseRow
	if json.Unmarshal(raw, &out) != nil || out[0]["null"] != nil || out[0]["empty"] == nil || !bytes.Equal(out[0]["binary"], in[0]["binary"]) {
		t.Fatal("raw SQL null/empty/binary collapsed")
	}
	p := &AIHistoricalInputPair{}
	if p.ValidateFrozen(t.Context()) == nil {
		t.Fatal("imported pair accepted")
	}
}

func TestAIHistoricalComponentIndexIsPureBoundedAndDefensive(t *testing.T) {
	s := aiReverseUnitGraph(t)
	s.inputPage = 1
	s.metadata = make([]aiReverseMetadata, len(aiReverseSpecs))
	for i, spec := range aiReverseSpecs {
		s.metadata[i] = aiReverseMetadata{columns: append([]string(nil), spec.columns...), schema: "schema-" + spec.table, pk: "pk-" + spec.table, sourceColumns: SQLColumns{{ptrString("original")}}}
	}
	e := &AIHistoricalInputEpoch{epoch: "actual-input-fixture", limits: DefaultAIReverseLimits(), pages: []historicalSpoolRef{{0, 1, "row-page"}}}
	if err := e.freezeComponentIndex(s); err != nil {
		t.Fatal(err)
	}
	request := s.byTable["ai_bridge_requests"]
	for id, r := range request {
		if len(e.componentPages["request:"+id]) != 1 {
			t.Fatal("request members were dropped or page was duplicated")
		}
		for _, assessment := range r.assessments {
			if !reflect.DeepEqual(e.componentRequests["assessment:"+assessment], []string{id}) || !reflect.DeepEqual(e.componentRequests["sheet:9"], []string{id}) {
				t.Fatal("actual owner relationship missing")
			}
		}
	}
	before := e.componentIndexSHA
	s.metadata[0].columns[0] = "changed"
	*s.metadata[0].sourceColumns[0][0] = "changed"
	s.nodes[0].request = "changed"
	if before == "" || e.componentIndexDigest() != before {
		t.Fatal("original live graph retained by index")
	}
	e.metadata[0].columns[0] = "tampered"
	if e.componentIndexDigest() == before {
		t.Fatal("index metadata mutation not detected at full boundary")
	}
	tiny := &AIHistoricalInputEpoch{epoch: "bounded", limits: DefaultAIReverseLimits(), pages: e.pages}
	tiny.limits.MaxRetainedBytes = 1
	if !errors.Is(tiny.freezeComponentIndex(s), ErrAIReverseBounds) {
		t.Fatal("compact index reservation was not bounded")
	}
}
