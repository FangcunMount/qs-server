package topology

import "testing"

func TestSourceCatalogCoversOnlyFixedFirstVersionTopologies(t *testing.T) {
	want := []string{"questionnaire", "published-model", "assessment-detail", "assessment-access"}
	got := Sources()
	if len(got) != len(want) {
		t.Fatalf("sources = %#v", got)
	}
	for index, group := range want {
		if got[index].TopologyGroup != group || got[index].ReadModel == "" || got[index].SourceKind == "" {
			t.Fatalf("sources[%d] = %#v", index, got[index])
		}
	}
	if _, ok := Lookup("typology"); ok {
		t.Fatal("conditional typology path must not be represented as fixed topology")
	}
}
