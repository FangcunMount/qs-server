package retirement

import (
	"bytes"
	"context"
	"encoding/json"
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
