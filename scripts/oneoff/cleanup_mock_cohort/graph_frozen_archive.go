package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
)

// This header admission is used only after the original successful capture
// stdout+actual exit proof has bound material.json. Capture itself still checks
// full physical coverage and original fingerprints before producing success.
func graphFrozenArchiveHeaders(m graphArchive, g group, p protected, index graphIndex) error {
	if err := validateGraphArchive(m, g, m.GroupSHA); err != nil {
		return err
	}
	counts := map[string]int64{}
	used := map[string]bool{}
	guards := 0
	for role, refs := range map[string][]graphShard{"t": m.Targets, "r": m.Retained, "p": m.Guards} {
		for i, ref := range refs {
			if ref.Store != "mysql" && ref.Store != "mongo" {
				return errors.New("frozen archive store changed")
			}
			if ref.File != fmt.Sprintf("%s-%08d.json", role, i) || !hashPattern.MatchString(ref.SHA) || ref.Bytes < 1 || ref.Bytes > graphBatchBytes || ref.Count < 1 || ref.Count > 1000 || ref.Store == "mongo" && ref.Count > 500 {
				return errors.New("frozen archive shard header changed")
			}
			scope := ref.Store + "/" + ref.Name
			definition, ok := m.Namespaces[scope]
			if !ok || len(definition) == 0 {
				return errors.New("frozen archive namespace missing")
			}
			rank, known := graphSQLTables[ref.Name]
			if ref.Store == "mongo" {
				rank, known = graphMongoTables[ref.Name]
			} else if ref.Store != "mysql" {
				known = false
			}
			if role == "t" && (!known || rank <= 0 || rank != ref.RestoreRank) || role == "r" && (!known || rank != 0 || ref.RestoreRank != 0) {
				return errors.New("frozen archive writable or retained rank changed")
			}
			if role != "p" && (!hashPattern.MatchString(index.Definitions[scope]) || index.Definitions[scope] != digest(definition)) {
				return errors.New("frozen archive namespace differs from its successful capture scope")
			}
			if role == "t" {
				counts[scope] += int64(ref.Count)
			}
			if role == "p" {
				guards += ref.Count
			}
			used[scope] = true
		}
	}
	if !reflect.DeepEqual(counts, m.Counts) || guards != m.ProtectedCount || guards != len(graphGuardObjects(p)) || len(used) != len(m.Namespaces) {
		return errors.New("frozen archive header coverage changed")
	}
	return nil
}

// The entire original file is still SHA-bound to the successful capture. Decode
// only its physical identity fields; Columns/BSON and their base64 payloads stay
// in the immutable original archive for default/unknown/restore consumers.
func readGraphFrozenKeys(dir string, m graphArchive, ref graphShard) ([]record, error) {
	if !regexpArchiveShard.MatchString(ref.File) || ref.Bytes < 1 || ref.Bytes > graphBatchBytes || ref.Count < 1 || ref.Count > 1000 || ref.Store == "mongo" && ref.Count > 500 || !hashPattern.MatchString(ref.SHA) {
		return nil, errors.New("invalid bounded frozen key shard")
	}
	raw, err := privateRead(filepath.Join(dir, ref.File), int64(ref.Bytes))
	if err != nil {
		return nil, err
	}
	if len(raw) != ref.Bytes || digest(raw) != ref.SHA {
		return nil, errors.New("frozen key shard raw SHA changed")
	}
	var page struct {
		Format  int    `json:"format_version"`
		Kind    string `json:"kind"`
		Records []struct {
			Object object `json:"object"`
		} `json:"records"`
	}
	// This is not a replacement for strictDecode on authority or new archives.
	// The same bytes already passed it in the successful capture execution.
	if err = json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	if page.Format != 1 || page.Kind != "mock_cohort_original_values" || len(page.Records) != ref.Count {
		return nil, errors.New("frozen key shard format or cardinality changed")
	}
	definition, ok := m.Namespaces[ref.Store+"/"+ref.Name]
	if !ok || len(definition) == 0 {
		return nil, errors.New("frozen key namespace definition missing")
	}
	out := make([]record, len(page.Records))
	seen := map[string]bool{}
	for i, row := range page.Records {
		o := row.Object
		if err = validateObject(o); err != nil {
			return nil, err
		}
		if o.Store != ref.Store || o.Name != ref.Name || !hashPattern.MatchString(o.Fingerprint) || seen[objectID(o)] {
			return nil, errors.New("frozen key physical identity changed")
		}
		seen[objectID(o)] = true
		out[i] = record{Object: o, Definition: definition}
	}
	return out, nil
}

func graphFrozenMutationObjects(records []record) ([]object, error) {
	objects := make([]object, len(records))
	for i, r := range records {
		if len(r.Definition) == 0 {
			return nil, errors.New("frozen delete namespace missing")
		}
		objects[i] = r.Object
	}
	return objects, graphBatchObjects(objects, true)
}
func graphAcknowledgedAbsent(prior journalEntry) bool {
	return prior.Action == "apply" && (prior.State == "committed" || prior.State == "reconciled_absent")
}
func graphAllTargetsAcknowledged(m graphArchive, j *graphJournal) bool {
	for _, ref := range m.Targets {
		entry, found := j.latest[graphChunkID(ref)]
		if !found || !graphAcknowledgedAbsent(entry) {
			return false
		}
	}
	return len(m.Targets) > 0
}
