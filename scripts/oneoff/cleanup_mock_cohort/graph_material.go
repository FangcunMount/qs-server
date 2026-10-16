package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

type graphBinding struct {
	ArchiveDirectory string    `json:"archive_directory"`
	IndexSHA         string    `json:"index_sha256"`
	IndexSource      string    `json:"index_source"`
	SelectorSHA      string    `json:"selector_sha256"`
	RootIDs          []string  `json:"root_testee_ids"`
	WindowStarted    time.Time `json:"window_started_at"`
	WindowExpires    time.Time `json:"window_expires_at"`
}

type graphArchive struct {
	Format         int               `json:"format_version"`
	Kind           string            `json:"kind"`
	Operation      string            `json:"operation_id"`
	GroupSHA       string            `json:"group_manifest_sha256"`
	ProtectionSHA  string            `json:"protected_manifest_sha256"`
	Source         string            `json:"compiled_source"`
	Binding        binding           `json:"binding"`
	Graph          graphBinding      `json:"graph"`
	Namespaces     map[string][]byte `json:"namespace_definitions"`
	Targets        []graphShard      `json:"target_archives"`
	Retained       []graphShard      `json:"retained_archives"`
	Guards         []graphShard      `json:"protected_archives"`
	Counts         map[string]int64  `json:"target_counts"`
	ProtectedCount int               `json:"protected_count"`
}

type graphArchivePage struct {
	Format  int      `json:"format_version"`
	Kind    string   `json:"kind"`
	Records []record `json:"records"`
}

func readGraphInputs(c config, g group, p protected) (graphIndex, cohortSelector, error) {
	var index graphIndex
	var selector cohortSelector
	if g.QS.Graph == nil {
		return index, selector, errors.New("full graph authority required")
	}
	b := g.QS.Graph
	if !filepath.IsAbs(b.ArchiveDirectory) || filepath.Clean(b.ArchiveDirectory) != b.ArchiveDirectory || c.archive != b.ArchiveDirectory {
		return index, selector, errors.New("approved graph requires its one canonical archive directory")
	}
	if c.selectorSHA != b.SelectorSHA {
		return index, selector, errors.New("selector CLI hash differs from group authority")
	}
	if !hashPattern.MatchString(b.IndexSHA) || !hashPattern.MatchString(b.SelectorSHA) || !sourcePattern.MatchString(b.IndexSource) || len(g.QS.Objects) != 0 || len(g.QS.Exclusions) != 0 {
		return index, selector, errors.New("full graph cannot mix primitive objects or exclusions")
	}
	if e := canonicalIDs(b.RootIDs, false); e != nil {
		return index, selector, e
	}
	if e := privateDirectory(c.scopeDir); e != nil {
		return index, selector, e
	}
	if e := readExact(filepath.Join(c.scopeDir, "graph-index.json"), b.IndexSHA, 16*1024*1024, &index); e != nil {
		return index, selector, e
	}
	if e := readExact(c.selector, b.SelectorSHA, 16*1024*1024, &selector); e != nil {
		return index, selector, e
	}
	if e := validateSelector(selector, p); e != nil {
		return index, selector, e
	}
	if index.Format != 2 || index.Kind != "mock_cohort_graph_index" || index.Source != b.IndexSource || index.SelectorSHA != b.SelectorSHA || index.ProtectionSHA != g.ProtectedSHA || selector.ProtectionSHA != g.ProtectedSHA || index.OwnerEvidenceSHA != selector.OwnerEvidenceSHA || !index.DiscoveryComplete || !index.ReadOnly || !index.Released || !reflect.DeepEqual(index.Binding, g.QS.Binding) || !reflect.DeepEqual(selector.Binding, g.QS.Binding) || !reflect.DeepEqual(selector.Selection, g.QS.Selector) {
		return index, selector, errors.New("original readonly graph/selector/group identity mismatch")
	}
	// Reuse the literal common owner/IAM/protection contract without granting the
	// primitive path any additional writable table. Every actual root is checked.
	legacy := g
	legacy.QS.Graph = nil
	legacy.QS.Objects = []object{keyRoot(b.RootIDs[0])}
	if e := validateGroup(legacy, p); e != nil {
		return index, selector, e
	}
	declared := map[string]bool{}
	for _, id := range selector.TesteeIDs {
		declared[id] = true
	}
	selected := map[string]bool{}
	for _, id := range b.RootIDs {
		if !declared[id] {
			return index, selector, errors.New("window subset expands owner-confirmed roots")
		}
		selected[id] = true
	}
	window, e := graphWindowSelector(selector, b.RootIDs)
	if e != nil {
		return index, selector, e
	}
	if e = validateGraphIAMWindow(window, g.IAM); e != nil {
		return index, selector, e
	}
	for _, o := range append(append(append([]object{}, p.QS.System...), p.QS.Clack...), p.QS.Other...) {
		if o.Store == "mysql" && o.Name == "testee" && len(o.Key) == 1 && selected[string(o.Key[0].Raw)] {
			return index, selector, errors.New("window subset intersects protected Testee")
		}
	}
	if b.WindowStarted.IsZero() || !b.WindowStarted.Before(b.WindowExpires) || b.WindowExpires.Sub(b.WindowStarted) > 30*time.Minute {
		return index, selector, errors.New("one fixed maintenance window of at most thirty minutes required")
	}
	if e := validateGraphHolds(c.scopeDir, index, selected); e != nil {
		return index, selector, e
	}
	return index, selector, nil
}

// A window may choose complete identity components, never part of a shared
// User/Profile component. The external identity mapping keeps its original SHA.
func graphWindowSelector(selector cohortSelector, roots []string) (cohortSelector, error) {
	if e := validateIdentityGroups(selector); e != nil {
		return selector, e
	}
	if e := canonicalIDs(roots, false); e != nil {
		return selector, e
	}
	selected := graphRootSet(roots)
	covered := map[string]bool{}
	groups := []cohortIdentityGroup{}
	for _, component := range selector.IdentityGroups {
		n := 0
		for _, id := range component.TesteeIDs {
			if selected[id] {
				n++
				covered[id] = true
			}
		}
		if n == 0 {
			continue
		}
		if n != len(component.TesteeIDs) {
			return selector, errors.New("window subset splits a frozen identity component")
		}
		groups = append(groups, component)
	}
	if len(covered) != len(selected) {
		return selector, errors.New("window subset expands frozen identity components")
	}
	selector.TesteeIDs = append([]string{}, roots...)
	selector.IdentityGroups = groups
	return selector, nil
}

func validateGraphIAMWindow(window cohortSelector, raw []byte) error {
	var scope iamScope
	if e := strictDecode(raw, &scope); e != nil {
		return e
	}
	for _, ids := range [][]string{scope.Users, scope.Profiles, scope.Identities} {
		if e := canonicalIDs(ids, true); e != nil {
			return e
		}
	}
	users, profiles := map[string]bool{}, map[string]bool{}
	for _, component := range window.IdentityGroups {
		for _, id := range component.UserIDs {
			users[id] = true
		}
		for _, id := range component.ProfileIDs {
			profiles[id] = true
		}
	}
	for i, ids := range [][]string{scope.Users, scope.Profiles} {
		expected := []map[string]bool{users, profiles}[i]
		if len(ids) != len(expected) {
			return errors.New("IAM scope differs from the selected complete identity components")
		}
		for _, id := range ids {
			if !expected[id] {
				return errors.New("IAM scope crosses selected identity components")
			}
		}
	}
	// Components do not carry LoginIdentity IDs. Their ownership remains the
	// separate IAM archive/handoff responsibility; equal numbers prove no link.
	return nil
}
func keyRoot(id string) object {
	return object{Store: "mysql", Name: "testee", Key: []cell{{Name: "id", Type: "uint64", Raw: []byte(id)}}}
}

var sourcePattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validateGraphHolds(dir string, index graphIndex, selected map[string]bool) error {
	if len(index.Holds) > 0 && len(index.HoldShards) == 0 {
		return errors.New("aggregate-only holds cannot qualify a subset")
	}
	counts := map[string]int64{}
	for _, s := range index.HoldShards {
		if !regexpHoldShard.MatchString(s.File) || s.Count < 1 || s.Count > 1000 || s.Bytes <= 0 || s.Bytes > 4*1024*1024 {
			return errors.New("invalid exact component hold shard")
		}
		var page graphHoldPage
		if e := readExact(filepath.Join(dir, s.File), s.SHA, int64(s.Bytes), &page); e != nil {
			return e
		}
		if page.Format != 1 || page.Kind != "mock_cohort_component_holds" || len(page.Holds) != s.Count {
			return errors.New("component hold index/count mismatch")
		}
		for _, h := range page.Holds {
			if !hashPattern.MatchString(h.PhysicalSHA) || h.Store != "mysql" && h.Store != "mongo" || !identifier.MatchString(h.Name) || h.Reason == "" {
				return errors.New("invalid physical component hold")
			}
			counts[h.Reason]++
			if len(h.RootIDs) == 0 && (strings.Contains(h.Reason, "manual_replay_responsibility") || strings.Contains(h.Reason, "requires_separate_shared_reference_resolution")) {
				return errors.New("unassigned replay/AI responsibility cannot qualify a complete window subset")
			}
			for _, root := range h.RootIDs {
				if selected[root] {
					return errors.New("selected root has an unresolved component hold; choose an independently complete subset")
				}
			}
		}
	}
	if !reflect.DeepEqual(counts, index.Holds) {
		return errors.New("component hold summary differs from its original mapping")
	}
	return nil
}

var regexpHoldShard = regexp.MustCompile(`^holds-[0-9]{8}\.json$`)
var regexpArchiveShard = regexp.MustCompile(`^[tpr]-[0-9]{8}\.json$`)

func forSelectedScope(dir string, index graphIndex, roots []string, fn func([]graphObject, int) error) error {
	return forGraphScope(dir, index, roots, false, fn)
}
func forRetainedScope(dir string, index graphIndex, roots []string, fn func([]graphObject, int) error) error {
	return forGraphScope(dir, index, roots, true, fn)
}
func forGraphScope(dir string, index graphIndex, roots []string, retained bool, fn func([]graphObject, int) error) error {
	selected := map[string]bool{}
	for _, id := range roots {
		selected[id] = true
	}
	seenRoots := map[string]bool{}
	total := 0
	for _, ref := range index.Shards {
		if (ref.RestoreRank <= 0) != retained {
			continue
		}
		page, e := readGraphPage(dir, ref)
		if e != nil {
			return e
		}
		batch := []graphObject{}
		for _, o := range page.Objects {
			owned := graphRoots(o)
			if len(owned) == 0 {
				return fmt.Errorf("scope %s/%s has no exact subset-root binding", o.Object.Store, o.Object.Name)
			}
			include := false
			for _, id := range owned {
				include = include || selected[id]
			}
			if !include {
				continue
			}
			for _, id := range owned {
				if !selected[id] {
					return errors.New("component straddles retained and selected roots")
				}
			}
			if o.Object.Store == "mysql" && o.Object.Name == "testee" {
				seenRoots[o.Links["id"]] = true
			}
			batch = append(batch, o)
			total++
		}
		if len(batch) > 0 {
			if e = fn(batch, ref.RestoreRank); e != nil {
				return e
			}
		}
	}
	if !retained && (total == 0 || len(seenRoots) != len(selected)) {
		return errors.New("subset lost one or more explicit Testee roots")
	}
	return nil
}

type graphBatchProvider interface {
	readExactBatch(context.Context, []object) ([]graphReadResult, error)
	preflightExactBatch(context.Context, []record, bool) error
	mutateExactBatch(context.Context, []record, bool) error
}

func archiveGraphPage(dir string, m *graphArchive, records []record, rank int, guard bool) error {
	if len(records) == 0 {
		return errors.New("empty full graph archive shard")
	}
	store, name := records[0].Object.Store, records[0].Object.Name
	limit := 1000
	if store == "mongo" {
		limit = 500
	}
	if len(records) > limit {
		return errors.New("archive cardinality exceeds bounded store chunk")
	}
	page := graphArchivePage{Format: 1, Kind: "mock_cohort_original_values", Records: make([]record, len(records))}
	scope := store + "/" + name
	for i, r := range records {
		if r.Object.Store != store || r.Object.Name != name {
			return errors.New("mixed namespace archive shard")
		}
		if previous, ok := m.Namespaces[scope]; ok && !bytes.Equal(previous, r.Definition) {
			return errors.New("namespace changed within complete original archive")
		}
		m.Namespaces[scope] = append([]byte{}, r.Definition...)
		r.Object.Fingerprint = fingerprint(r)
		r.Definition = nil
		page.Records[i] = r
	}
	raw := encode(page)
	if len(raw) > 64*1024*1024 {
		return errors.New("archive shard exceeds fixed 64MiB")
	}
	prefix := "t"
	seq := len(m.Targets)
	if guard {
		prefix = "p"
		seq = len(m.Guards)
	} else if rank == 0 {
		prefix = "r"
		seq = len(m.Retained)
	}
	file := fmt.Sprintf("%s-%08d.json", prefix, seq)
	if e := writeNew(filepath.Join(dir, file), raw); e != nil {
		return e
	}
	ref := graphShard{File: file, SHA: digest(raw), Bytes: len(raw), Count: len(records), Store: store, Name: name, RestoreRank: rank}
	if guard {
		m.Guards = append(m.Guards, ref)
		m.ProtectedCount += len(records)
	} else if rank == 0 {
		m.Retained = append(m.Retained, ref)
	} else {
		m.Targets = append(m.Targets, ref)
		m.Counts[scope] += int64(len(records))
	}
	return nil
}
func readGraphArchivePage(dir string, m graphArchive, ref graphShard) ([]record, error) {
	if !regexpArchiveShard.MatchString(ref.File) || ref.Bytes < 1 || ref.Bytes > 64*1024*1024 || ref.Count < 1 || ref.Count > 1000 || ref.Store == "mongo" && ref.Count > 500 {
		return nil, errors.New("invalid bounded original archive shard")
	}
	var page graphArchivePage
	if e := readExact(filepath.Join(dir, ref.File), ref.SHA, int64(ref.Bytes), &page); e != nil {
		return nil, e
	}
	if page.Format != 1 || page.Kind != "mock_cohort_original_values" || len(page.Records) != ref.Count {
		return nil, errors.New("original archive shard count mismatch")
	}
	seen := map[string]bool{}
	for i, r := range page.Records {
		if len(r.Definition) != 0 || r.Object.Store != ref.Store || r.Object.Name != ref.Name {
			return nil, errors.New("original archive namespace mismatch")
		}
		if e := validateObject(r.Object); e != nil {
			return nil, e
		}
		key := objectID(r.Object)
		if seen[key] {
			return nil, errors.New("duplicate physical object in original shard")
		}
		seen[key] = true
		definition, ok := m.Namespaces[ref.Store+"/"+ref.Name]
		if !ok || len(definition) == 0 {
			return nil, errors.New("complete original namespace definition missing")
		}
		r.Definition = append([]byte{}, definition...)
		if r.Object.Fingerprint != fingerprint(r) {
			return nil, errors.New("complete original preimage fingerprint changed")
		}
		page.Records[i] = r
	}
	return page.Records, nil
}
func graphOrderedShards(m graphArchive, restore bool) []graphShard {
	out := append([]graphShard{}, m.Targets...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RestoreRank == out[j].RestoreRank {
			return out[i].File < out[j].File
		}
		if restore {
			return out[i].RestoreRank < out[j].RestoreRank
		}
		return out[i].RestoreRank > out[j].RestoreRank
	})
	return out
}
func graphAbsoluteContext(ctx context.Context, b graphBinding) (context.Context, context.CancelFunc, error) {
	now := time.Now()
	if now.Before(b.WindowStarted) || !now.Before(b.WindowExpires) {
		return nil, nil, errors.New("fixed maintenance window is not active")
	}
	c, cancel := context.WithDeadline(ctx, b.WindowExpires.Add(-5*time.Minute))
	if e := c.Err(); e != nil {
		cancel()
		return nil, nil, errors.New("five-minute startup reserve is already due")
	}
	return c, cancel, nil
}
func validateGraphArchive(m graphArchive, g group, groupSHA string) error {
	if g.QS.Graph == nil || m.Format != 1 || m.Kind != "mock_cohort_graph_archive" || m.Operation != g.Operation || m.GroupSHA != groupSHA || m.ProtectionSHA != g.ProtectedSHA || !sourcePattern.MatchString(m.Source) || !reflect.DeepEqual(m.Binding, g.QS.Binding) || !reflect.DeepEqual(m.Graph, *g.QS.Graph) || len(m.Targets) == 0 || len(m.Guards) == 0 {
		return errors.New("full graph archive does not bind original exact group")
	}
	for _, s := range append(append(append([]graphShard{}, m.Targets...), m.Guards...), m.Retained...) {
		if !strings.HasPrefix(s.File, "t-") && !strings.HasPrefix(s.File, "p-") && !strings.HasPrefix(s.File, "r-") {
			return errors.New("invalid archive role")
		}
	}
	return nil
}
