package eventruntime

import (
	_ "embed"
	"encoding/json"
	"testing"
)

// Captured using scripts/testing/capture-retired-wire-contracts.sh.
// Raw transport and domain bytes remain strings so fixture formatting cannot
// silently normalize them while decoding the outer capture document.
//
//go:embed testdata/retired-wire-contracts.json
var retiredWireFixture []byte

type retiredWireCase struct {
	Payload, Wire, RelayWire string
}

type retiredDecodeCase struct {
	Input    string
	Value    string
	Metadata map[string]string
	Error    string
}

func retiredWireContracts(t *testing.T) (map[string]retiredWireCase, map[string]retiredDecodeCase) {
	t.Helper()
	var fixture struct {
		Source   string
		Wires    map[string]retiredWireCase
		Decoders map[string]retiredDecodeCase
	}
	if err := json.Unmarshal(retiredWireFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source != "96fc4df42f6af9f6ce22e2184394b1a07df2ffdd/component-base@v0.6.11" || len(fixture.Wires) != 15 || len(fixture.Decoders) != 11 {
		t.Fatal("historical fixture source or inventory changed")
	}
	return fixture.Wires, fixture.Decoders
}
