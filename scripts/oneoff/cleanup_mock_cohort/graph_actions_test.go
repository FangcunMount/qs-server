package main

import (
	"strings"
	"testing"
)

func TestGraphFrozenScopeBindsKeysOwnershipAndMetadataWithoutBodies(t *testing.T) {
	_, _, _, index, _, scope, _ := graphEngineFixture(t)
	tests := []struct {
		name   string
		change func(*graphObject, *int)
		meta   bool
		old    bool
	}{
		{name: "same technical scope"},
		{name: "changed key", change: func(o *graphObject, _ *int) {
			if o.Object.Name == "assessment_score" {
				o.Object.Key[0].Raw = []byte("999")
			}
		}},
		{name: "settled technical state", change: func(o *graphObject, _ *int) { o.Links["status"] = "closed" }},
		{name: "changed rank", change: func(_ *graphObject, rank *int) { *rank++ }},
		{name: "straddling identity group", change: func(o *graphObject, _ *int) { o.RootIDs = []string{"1", "2"} }},
		{name: "changed namespace", meta: true},
		{name: "old plan format", old: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := privateTestDir(t)
			fresh := graphIndex{Format: 2, Shards: []graphShard{}, Counts: map[string]int64{}, Definitions: map[string]string{}}
			for name, sha := range index.Definitions {
				fresh.Definitions[name] = sha
			}
			if tc.meta {
				fresh.Definitions["mysql/assessment_score"] = strings.Repeat("0", 64)
			}
			writer := graphWriter{dir: dir, index: &fresh, protected: map[string]bool{}}
			for _, ref := range index.Shards {
				page, e := readGraphPage(scope, ref)
				if e != nil {
					t.Fatal(e)
				}
				for _, object := range page.Objects {
					rank := ref.RestoreRank
					if tc.change != nil {
						tc.change(&object, &rank)
					}
					if e = writer.add(object, rank); e != nil {
						t.Fatal(e)
					}
				}
			}
			if e := writer.flush(); e != nil {
				t.Fatal(e)
			}
			if tc.old {
				fresh.Format = 1
			}
			e := compareFrozenScope(scope, index, []string{"1"}, dir, fresh)
			wantAccepted := tc.name == "same technical scope" || tc.name == "settled technical state"
			if (e == nil) != wantAccepted {
				t.Fatalf("frozen physical/ownership comparison: %v", e)
			}
		})
	}
}
