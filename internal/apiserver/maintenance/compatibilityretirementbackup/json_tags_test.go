package compatibilityretirementbackup

import "testing"

func TestExactJSONUsesNamesBeforeTagOptions(t *testing.T) {
	type wire struct {
		Optional string `json:"optional,omitempty"`
		Default  string `json:",omitempty"`
		Hidden   string `json:"-"`
	}
	for _, input := range []string{`{}`, `{"optional":"present","Default":"default name"}`} {
		var decoded wire
		if err := exactJSON([]byte(input), &decoded); err != nil {
			t.Fatal("canonical optional field rejected", err)
		}
	}
	for _, input := range []string{`{"Optional":"alias"}`, `{"optional,omitempty":"tag options are not a field name"}`, `{"Hidden":"ignored field"}`, `{"optional":"first","optional":"second"}`, `{"Default":"first","default":"alias"}`} {
		if err := exactJSON([]byte(input), new(wire)); err == nil {
			t.Fatal("unknown, alias or duplicate field accepted")
		}
	}
}
