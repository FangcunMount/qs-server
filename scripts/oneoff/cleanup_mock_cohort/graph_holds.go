package main

import (
	"errors"
	"fmt"
	"path/filepath"
)

type graphHold struct {
	Reason      string   `json:"reason"`
	RootIDs     []string `json:"root_testee_ids"`
	Store       string   `json:"store"`
	Name        string   `json:"name"`
	PhysicalSHA string   `json:"physical_key_sha256"`
}
type graphHoldShard struct {
	File  string `json:"file"`
	SHA   string `json:"sha256"`
	Bytes int    `json:"bytes"`
	Count int    `json:"count"`
}
type graphHoldPage struct {
	Format int         `json:"format_version"`
	Kind   string      `json:"kind"`
	Holds  []graphHold `json:"holds"`
}

func graphRoots(o graphObject) []string {
	out := []string{}
	seen := map[string]bool{}
	ids := append([]string{}, o.RootIDs...)
	ids = append(ids, o.Links["testee_id"])
	if o.Object.Store == "mysql" && o.Object.Name == "testee" {
		ids = append(ids, o.Links["id"])
	}
	for _, id := range ids {
		if graphDecimal(id) && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out
}
func (p *cohortPlanner) holdObject(o graphObject, reason string) {
	p.writer.index.Holds[reason]++
	if p.holdError != nil {
		return
	}
	p.held = append(p.held, graphHold{reason, graphRoots(o), o.Object.Store, o.Object.Name, objectID(o.Object)})
	if len(p.held) == 1000 {
		p.holdError = p.flushHolds()
	}
}
func (p *cohortPlanner) holdRecord(r record, roots []string, reason string) {
	links := map[string]string{}
	if v, ok := graphSQLValue(r, "testee_id"); ok {
		links["testee_id"] = v
	}
	p.holdObject(graphObject{Object: r.Object, Links: links, RootIDs: roots}, reason)
}
func (p *cohortPlanner) flushHolds() error {
	if p.holdError != nil {
		return p.holdError
	}
	if len(p.held) == 0 {
		return nil
	}
	raw := encode(graphHoldPage{1, "mock_cohort_component_holds", p.held})
	if len(raw) > 4*1024*1024 {
		return errors.New("component hold shard exceeds fixed 4MiB")
	}
	file := fmt.Sprintf("holds-%08d.json", len(p.writer.index.HoldShards))
	if e := writeNew(filepath.Join(p.dir, file), raw); e != nil {
		return e
	}
	p.writer.index.HoldShards = append(p.writer.index.HoldShards, graphHoldShard{file, digest(raw), len(raw), len(p.held)})
	p.held = nil
	return nil
}
