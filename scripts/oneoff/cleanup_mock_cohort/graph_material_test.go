package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestGraphWindowKeepsCompleteIdentityGroups(t *testing.T) {
	selector := cohortSelector{Format: 2, IdentityLinksSHA: strings.Repeat("a", 64), TesteeIDs: []string{"1", "2", "3"}, IdentityGroups: []cohortIdentityGroup{
		{UserIDs: []string{"10"}, ProfileIDs: []string{"20", "21"}, TesteeIDs: []string{"1", "2"}},
		{UserIDs: []string{"11"}, ProfileIDs: []string{"22"}, TesteeIDs: []string{"3"}},
	}}
	for _, roots := range [][]string{{"1"}, {"2", "3"}, {"4"}, {"1", "1"}} {
		if _, e := graphWindowSelector(selector, roots); e == nil {
			t.Fatal("partial, duplicate or undeclared component admitted", roots)
		}
	}
	window, e := graphWindowSelector(selector, []string{"1", "2"})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(window.TesteeIDs, []string{"1", "2"}) || !reflect.DeepEqual(window.IdentityGroups, selector.IdentityGroups[:1]) || window.IdentityLinksSHA != selector.IdentityLinksSHA {
		t.Fatal("window changed typed identity ownership or its external mapping SHA")
	}
	if !reflect.DeepEqual(selector.TesteeIDs, []string{"1", "2", "3"}) || len(selector.IdentityGroups) != 2 {
		t.Fatal("window selection mutated the original owner selector")
	}
}

func TestGraphInputsBindSelectedIdentityGroupToIAMScope(t *testing.T) {
	_, g, p, index, _, scope, archive := graphEngineFixture(t)
	g.QS.Graph.WindowStarted = time.Now().Add(-time.Minute)
	g.QS.Graph.WindowExpires = g.QS.Graph.WindowStarted.Add(30 * time.Minute)
	g.QS.Binding = binding{SQLUUID: "fixed-mysql-server", SQLDatabase: "qs", MongoIdentity: strings.Repeat("a", 64), MongoDatabase: "qs", SQLVersion: 101, MongoVersion: 39}
	selector := cohortSelector{Format: 2, Kind: "mock_cohort_selector", Operation: g.Operation, OwnerEvidenceSHA: strings.Repeat("c", 64), ProtectionSHA: g.ProtectedSHA, Binding: g.QS.Binding, Selection: g.QS.Selector, TesteeIDs: []string{"1", "2"}, RetainPlanIDs: []string{}, IdentityLinksSHA: strings.Repeat("e", 64), IdentityGroups: []cohortIdentityGroup{
		{UserIDs: []string{"10"}, ProfileIDs: []string{"20", "21"}, TesteeIDs: []string{"1"}},
		{UserIDs: []string{"11"}, ProfileIDs: []string{"22"}, TesteeIDs: []string{"2"}},
	}}
	selectorRaw := encode(selector)
	selectorFile := filepath.Join(privateTestDir(t), "selector.json")
	if e := writeNew(selectorFile, selectorRaw); e != nil {
		t.Fatal(e)
	}
	index.Kind, index.Source, index.SelectorSHA = "mock_cohort_graph_index", strings.Repeat("d", 40), digest(selectorRaw)
	index.ProtectionSHA, index.OwnerEvidenceSHA, index.Binding = g.ProtectedSHA, selector.OwnerEvidenceSHA, g.QS.Binding
	index.DiscoveryComplete, index.ReadOnly, index.Released = true, true, true
	index.Holds = map[string]int64{}
	indexRaw := encode(index)
	if e := writeNew(filepath.Join(scope, "graph-index.json"), indexRaw); e != nil {
		t.Fatal(e)
	}
	g.QS.Graph.IndexSHA, g.QS.Graph.IndexSource, g.QS.Graph.SelectorSHA = digest(indexRaw), index.Source, digest(selectorRaw)
	c := config{scopeDir: scope, selector: selectorFile, selectorSHA: digest(selectorRaw), archive: archive}
	for _, tc := range []struct {
		name  string
		iam   string
		valid bool
	}{
		{"whole_selected_group", `{"user_ids":["10"],"profile_ids":["21","20"],"login_identity_ids":["99"]}`, true},
		{"other_group", `{"user_ids":["11"],"profile_ids":["22"],"login_identity_ids":["99"]}`, false},
		{"missing_profile", `{"user_ids":["10"],"profile_ids":["20"],"login_identity_ids":["99"]}`, false},
		{"extra_user", `{"user_ids":["10","11"],"profile_ids":["20","21"],"login_identity_ids":["99"]}`, false},
		{"foreign_profile", `{"user_ids":["10"],"profile_ids":["20","22"],"login_identity_ids":["99"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := g
			current.IAM = json.RawMessage(tc.iam)
			_, _, e := readGraphInputs(c, current, p)
			if (e == nil) != tc.valid {
				t.Fatalf("selected complete group and IAM material disagree: %v", e)
			}
		})
	}
}
