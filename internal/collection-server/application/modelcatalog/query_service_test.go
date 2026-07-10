package modelcatalog

import "testing"

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
