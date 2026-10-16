package main

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// A graph selector is a frozen, explicit subset of the owner-confirmed roots.
// A subsequent execution window must name its own exact subset; pagination is
// only an I/O bound and never creates a new maintenance budget.
type cohortSelector struct {
	Format           int       `json:"format_version"`
	Kind             string    `json:"kind"`
	Operation        string    `json:"operation_id"`
	OwnerEvidenceSHA string    `json:"owner_evidence_sha256"`
	ProtectionSHA    string    `json:"protected_manifest_sha256"`
	Binding          binding   `json:"binding"`
	Selection        selection `json:"selector"`
	TesteeIDs        []string  `json:"testee_ids"`
	RetainPlanIDs    []string  `json:"retain_plan_ids"`
}

type graphObject struct {
	Object     object                            `json:"object"`
	Links      map[string]string                 `json:"links"`
	References []eventevidence.StandardReference `json:"standard_references"`
	RootIDs    []string                          `json:"root_testee_ids"`
}

type graphShard struct {
	File        string `json:"file"`
	SHA         string `json:"sha256"`
	Bytes       int    `json:"bytes"`
	Count       int    `json:"count"`
	Store       string `json:"store"`
	Name        string `json:"name"`
	RestoreRank int    `json:"restore_rank"`
}

type graphPage struct {
	Format  int           `json:"format_version"`
	Kind    string        `json:"kind"`
	Objects []graphObject `json:"objects"`
}

type graphIndex struct {
	Format            int               `json:"format_version"`
	Kind              string            `json:"kind"`
	Operation         string            `json:"operation_id"`
	SelectorSHA       string            `json:"selector_sha256"`
	ProtectionSHA     string            `json:"protected_manifest_sha256"`
	OwnerEvidenceSHA  string            `json:"owner_evidence_sha256"`
	Source            string            `json:"compiled_source"`
	Binding           binding           `json:"binding"`
	Definitions       map[string]string `json:"namespace_definition_sha256"`
	Shards            []graphShard      `json:"shards"`
	Counts            map[string]int64  `json:"counts"`
	Retained          map[string]string `json:"retained_objects"`
	HoldShards        []graphHoldShard  `json:"component_hold_shards"`
	Holds             map[string]int64  `json:"component_holds"`
	RequiredActions   map[string]int64  `json:"required_actions"`
	ReadOnly          bool              `json:"database_read_only"`
	DiscoveryComplete bool              `json:"discovery_complete"`
	WritePending      bool              `json:"full_graph_write_implementation_pending"`
	Released          bool              `json:"resource_release_proven"`
}

func canonicalIDs(ids []string, allowEmpty bool) error {
	if ids == nil || !allowEmpty && len(ids) == 0 {
		return errors.New("explicit typed ID array required")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		n, e := strconv.ParseUint(id, 10, 64)
		if e != nil || n == 0 || n > math.MaxInt64 || strconv.FormatUint(n, 10) != id || seen[id] {
			return errors.New("unique positive canonical int64-compatible IDs required")
		}
		seen[id] = true
	}
	return nil
}

func validateSelector(c cohortSelector, p protected) error {
	if c.Format != 1 || c.Kind != "mock_cohort_selector" || !idPattern.MatchString(c.Operation) || !hashPattern.MatchString(c.OwnerEvidenceSHA) || !hashPattern.MatchString(c.ProtectionSHA) {
		return errors.New("invalid exact cohort selector authority")
	}
	if c.Binding.SQLVersion != 101 || c.Binding.MongoVersion != 39 || c.Binding.SQLUUID == "" || c.Binding.SQLDatabase == "" || !hashPattern.MatchString(c.Binding.MongoIdentity) || c.Binding.MongoDatabase == "" {
		return errors.New("explicit clean 101/39 identities required")
	}
	if e := canonicalIDs(c.TesteeIDs, false); e != nil {
		return e
	}
	if e := canonicalIDs(c.RetainPlanIDs, true); e != nil {
		return e
	}
	if c.Selection.Source == "" || c.Selection.Evidence == "" {
		return errors.New("confirmed cohort source and evidence required")
	}
	if e := canonicalIDs([]string{c.Selection.OrgID}, false); e != nil {
		return e
	}
	from, e := time.Parse(time.RFC3339, c.Selection.From)
	if e != nil {
		return e
	}
	through, e := time.Parse(time.RFC3339, c.Selection.Through)
	if e != nil || !from.Before(through) || !strings.HasSuffix(c.Selection.From, "+08:00") || !strings.HasSuffix(c.Selection.Through, "+08:00") {
		return errors.New("explicit Shanghai root window required")
	}
	if p.Format != 1 || p.Kind != "mock_cohort_protected" || len(p.QS.System) == 0 || len(p.QS.Clack) == 0 || p.QS.Other == nil {
		return errors.New("explicit shared system/clack protection required")
	}
	roots := map[string]bool{}
	for _, id := range c.TesteeIDs {
		roots[id] = true
	}
	for _, o := range append(append(append([]object{}, p.QS.System...), p.QS.Clack...), p.QS.Other...) {
		if e := validateObject(o); e != nil {
			return e
		}
		if o.Store == "mysql" && o.Name == "testee" && len(o.Key) == 1 && o.Key[0].Name == "id" {
			if o.Key[0].Type != "uint64" {
				return errors.New("protected testee must use actual unsigned PK type")
			}
			if roots[string(o.Key[0].Raw)] {
				return errors.New("explicit root intersects protected testee")
			}
		}
	}
	return nil
}

// Scope shards contain only typed identities, preimage fingerprints and narrow
// technical links. Full business values are never accumulated in the index.
var regexpGraphShard = regexp.MustCompile(`^scope-[0-9]{8}\.json$`)

type graphWriter struct {
	dir          string
	index        *graphIndex
	current      []graphObject
	currentBytes int
	scope        string
	rank         int
	protected    map[string]bool
}

func (w *graphWriter) add(o graphObject, rank int) error {
	o.RootIDs = graphRoots(o)
	if e := validateObject(o.Object); e != nil {
		return e
	}
	if !hashPattern.MatchString(o.Object.Fingerprint) {
		return errors.New("scope object missing complete original preimage fingerprint")
	}
	if w.protected[objectID(o.Object)] {
		return errors.New("discovered physical target intersects protection")
	}
	scope := o.Object.Store + "/" + o.Object.Name
	limit := 1000
	if o.Object.Store == "mongo" {
		limit = 500
	}
	if len(w.current) > 0 && (scope != w.scope || rank != w.rank || len(w.current) >= limit) {
		if e := w.flush(); e != nil {
			return e
		}
	}
	w.scope = scope
	w.rank = rank
	w.currentBytes += len(encode(o)) + 1
	w.current = append(w.current, o)
	if w.currentBytes+128 > 64*1024*1024 {
		return errors.New("scope shard exceeds fixed 64MiB")
	}
	w.index.Counts[scope]++
	return nil
}
func (w *graphWriter) flush() error {
	if len(w.current) == 0 {
		return nil
	}
	p := graphPage{1, "mock_cohort_scope_shard", w.current}
	raw := encode(p)
	name := fmt.Sprintf("scope-%08d.json", len(w.index.Shards))
	if e := writeNew(filepath.Join(w.dir, name), raw); e != nil {
		return e
	}
	w.index.Shards = append(w.index.Shards, graphShard{name, digest(raw), len(raw), len(w.current), w.current[0].Object.Store, w.current[0].Object.Name, w.rank})
	w.current = nil
	w.currentBytes = 0
	return nil
}
func readGraphPage(dir string, s graphShard) (graphPage, error) {
	var p graphPage
	if !regexpGraphShard.MatchString(s.File) || s.Bytes <= 0 || s.Bytes > 64*1024*1024 || s.Count <= 0 || s.Count > 1000 || s.Store == "mongo" && s.Count > 500 {
		return p, errors.New("invalid bounded scope shard")
	}
	if e := readExact(filepath.Join(dir, s.File), s.SHA, int64(s.Bytes), &p); e != nil {
		return p, e
	}
	if p.Format != 1 || p.Kind != "mock_cohort_scope_shard" || len(p.Objects) != s.Count {
		return p, errors.New("scope shard cardinality changed")
	}
	seen := map[string]bool{}
	for _, o := range p.Objects {
		if e := validateObject(o.Object); e != nil {
			return p, e
		}
		id := objectID(o.Object)
		if o.Object.Store != s.Store || o.Object.Name != s.Name || seen[id] || !hashPattern.MatchString(o.Object.Fingerprint) {
			return p, errors.New("scope shard object identity mismatch")
		}
		seen[id] = true
	}
	return p, nil
}
func graphIDs(dir string, index *graphIndex, store, name, link string, limit int, fn func([]string) error) error {
	// Only one bounded scope page and one ID batch are held at a time. The same
	// logical key can occur in many child rows, so each caller traverses a parent
	// namespace with unique IDs rather than inferring ownership from equal IDs.
	batch := []string{}
	for _, s := range index.Shards {
		if s.Store != store || s.Name != name {
			continue
		}
		p, e := readGraphPage(dir, s)
		if e != nil {
			return e
		}
		for _, o := range p.Objects {
			id := o.Links[link]
			if id == "" {
				return errors.New("required typed parent link missing")
			}
			batch = append(batch, id)
			if len(batch) == limit {
				if e = fn(batch); e != nil {
					return e
				}
				batch = nil
			}
		}
	}
	if len(batch) > 0 {
		return fn(batch)
	}
	return nil
}
func sortedRootIDs(ids []string) []string {
	out := append([]string{}, ids...)
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.ParseUint(out[i], 10, 64)
		b, _ := strconv.ParseUint(out[j], 10, 64)
		return a < b
	})
	return out
}
