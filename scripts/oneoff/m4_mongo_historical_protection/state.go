package main

import (
	"bytes"
	"errors"
)

// A plan contains only positions in the four compiled targets, never a
// caller-selected namespace, command or new index specification.
type observation struct {
	SQLIdentity, MongoIdentity string
	SQLVersion                 uint64
	SQLDirty                   bool
	Collections                [2]namespaceProof
	Ledger                     ledgerProof
}

func indexSpecs(n namespaceProof) map[string][]byte {
	m := make(map[string][]byte, len(n.Indexes))
	for _, spec := range n.Indexes {
		name, _ := indexName(spec)
		m[name] = spec
	}
	return m
}
func targetPosition(collection, name string) int {
	for i, target := range targets {
		if target.collection == collection && target.name == name {
			return i
		}
	}
	return -1
}
func planRestore(m material, current observation) ([]int, error) {
	if e := validateMaterial(m, m.ToolSource, m.SQLName, m.MongoName); e != nil {
		return nil, e
	}
	if current.SQLVersion != 101 || current.SQLDirty || current.SQLIdentity != m.SQLIdentity || current.MongoIdentity != m.MongoIdentity {
		return nil, errors.New("selected_identity_or_clean101_changed")
	}
	if e := validateLedger(current.Ledger.Namespace); e != nil {
		return nil, e
	}
	head, dirty, e := ledgerVersion(current.Ledger.Document)
	if e != nil || head != 40 || bytes.Equal(current.Ledger.Namespace.UUID, m.Ledger.Namespace.UUID) || !bytes.Equal(current.Ledger.Namespace.Options, m.Ledger.Namespace.Options) || !bytes.Equal(current.Ledger.Namespace.Indexes[0], m.Ledger.Namespace.Indexes[0]) {
		return nil, errors.New("provider40_ledger_state_rejected")
	}
	missing := [4]bool{}
	for i, original := range m.Collections {
		now := current.Collections[i]
		if e := validateNamespace(now, original.Name); e != nil {
			return nil, e
		}
		if !bytes.Equal(now.UUID, original.UUID) || !bytes.Equal(now.Options, original.Options) || *now.Count != *original.Count {
			return nil, errors.New("business_namespace_or_count_changed")
		}
		before, after := indexSpecs(original), indexSpecs(now)
		for name, spec := range before {
			position := targetPosition(original.Name, name)
			observed, present := after[name]
			if position < 0 && !present || present && !bytes.Equal(spec, observed) {
				return nil, errors.New("unrelated_or_present_index_spec_changed")
			}
			if position >= 0 {
				missing[position] = !present
			}
			delete(after, name)
		}
		if len(after) != 0 {
			return nil, errors.New("unexpected_current_index")
		}
	}
	var result []int
	presentSeen := false
	for i, absent := range missing {
		if absent && presentSeen {
			return nil, errors.New("non_prefix_or_interrupted_restore_shape")
		}
		if absent {
			result = append(result, i)
		} else {
			presentSeen = true
		}
	}
	if !dirty && len(result) != len(targets) {
		return nil, errors.New("clean40_requires_all_four_indexes_retired")
	}
	return result, nil
}
