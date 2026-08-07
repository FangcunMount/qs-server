package modelcatalog

import (
	"encoding/json"
	"testing"
)

func TestCatalogListSummaryOmitsDefinition(t *testing.T) {
	t.Parallel()

	service := &QueryService{}
	detail := service.modelResponse(&CatalogModel{
		Code: "SCALE_A", Version: "v2", Category: "adhd",
		Definition: json.RawMessage(`{"Measure":{"Factors":[{"Code":"TOTAL"}]}}`),
	})
	value := (&ListResponse{Models: []ModelResponse{*detail}, Total: 1, Page: 1, PageSize: 20}).Summary()
	payload, err := json.Marshal(value.Models[0])
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, exists := fields["definition"]; exists {
		t.Fatalf("catalog list summary must not expose definition: %s", payload)
	}
	if fields["version"] != "v2" || fields["category"] != "adhd" {
		t.Fatalf("catalog list summary = %s", payload)
	}
}

func TestVisibleFactorCodesFromDefinition(t *testing.T) {
	t.Parallel()

	visible, configured, err := visibleFactorCodesFromDefinition([]byte(`{
		"ReportMap":{"Sections":[
			{"Kind":"summary","SourceRefs":["total"]},
			{"Kind":"factor_scores","SourceRefs":["f1","f2"]}
		]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !configured || len(visible) != 2 || !visible["f1"] || !visible["f2"] {
		t.Fatalf("visible factor codes = (%#v, %v)", visible, configured)
	}
}

func TestVisibleFactorCodesFromDefinitionKeepsAbsentMappingDistinct(t *testing.T) {
	t.Parallel()

	visible, configured, err := visibleFactorCodesFromDefinition([]byte(`{"ReportMap":{"Sections":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if visible != nil || configured {
		t.Fatalf("absent mapping = (%#v, %v)", visible, configured)
	}
}
