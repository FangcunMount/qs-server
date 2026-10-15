// This temporary M4-B tool is retired with the other one-off tools in M7.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

const bSource = "c6ad16329863a4aed7b8003d0c351db280be22a0"
const bResources = "60a0242e235bf696cb1e10d3c7e994f52d3ab81c17df2bc5eab195f60c90c95b"
const bResourceFiles = 282
const maxMaterialBytes = 4 << 20

var sourceRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var operationRE = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)

var targets = [4]struct {
	collection, name string
	key              bson.D
	uniquePath       string
}{
	{"answersheets", "uk_answersheet_legacy_submission_event_id", bson.D{{Key: "legacy_submission_evidence.entries.event_id", Value: int32(1)}}, "legacy_submission_evidence.entries.event_id"},
	{"answersheets", "idx_answersheet_legacy_submission_audit", bson.D{{Key: "legacy_submission_evidence.version", Value: int32(1)}, {Key: "deleted_at", Value: int32(1)}, {Key: "domain_id", Value: int32(1)}}, ""},
	{"report_generations", "uk_generation_historical_generated_event_id", bson.D{{Key: "historical_generated_evidence.entries.event_id", Value: int32(1)}}, "historical_generated_evidence.entries.event_id"},
	{"report_generations", "idx_generation_historical_generated_audit", bson.D{{Key: "historical_generated_evidence.version", Value: int32(1)}, {Key: "deleted_at", Value: int32(1)}, {Key: "status", Value: int32(1)}, {Key: "domain_id", Value: int32(1)}}, ""},
}

// BSON is stored as original bytes, not JSON-reencoded documents. Business
// document bodies are never part of this format. Counts do not certify bodies;
// continued writer freeze and the fixed index-only code scope remain required.
type namespaceProof struct {
	Name    string   `json:"collection"`
	UUID    []byte   `json:"namespace_uuid"`
	Options []byte   `json:"original_options_bson"`
	Indexes [][]byte `json:"original_ordered_index_specs_bson"`
	Count   *int64   `json:"exact_document_count"`
}
type ledgerProof struct {
	Namespace namespaceProof `json:"namespace"`
	Document  []byte         `json:"original_complete_document_bson"`
}
type material struct {
	Format         int               `json:"format_version"`
	Kind           string            `json:"kind"`
	ToolSource     string            `json:"compiled_tool_source"`
	BSource        string            `json:"b_execution_source"`
	BResources     string            `json:"b_execution_resources_sha256"`
	BResourceFiles int               `json:"b_execution_resource_files"`
	CapturedAt     string            `json:"captured_at"`
	SQLName        string            `json:"mysql_database"`
	MongoName      string            `json:"mongo_database"`
	SQLIdentity    string            `json:"mysql_identity_sha256"`
	MongoIdentity  string            `json:"mongo_identity_sha256"`
	SQLVersion     uint64            `json:"mysql_clean_version"`
	Collections    [2]namespaceProof `json:"collections"`
	Ledger         ledgerProof       `json:"original_mongo_ledger"`
}
type approval struct {
	Format             int    `json:"format_version"`
	Kind               string `json:"kind"`
	Operation          string `json:"operation_id"`
	ToolSource         string `json:"compiled_tool_source"`
	BSource            string `json:"b_execution_source"`
	BResources         string `json:"b_execution_resources_sha256"`
	BResourceFiles     int    `json:"b_execution_resource_files"`
	MaterialHash       string `json:"independently_frozen_material_sha256"`
	FreezeHash         string `json:"complete_writer_freeze_receipt_sha256"`
	ResponsibilityHash string `json:"responsibility_receipt_sha256"`
	SQLIdentity        string `json:"mysql_identity_sha256"`
	MongoIdentity      string `json:"mongo_identity_sha256"`
	RuntimeFrozen      bool   `json:"all_six_instances_and_other_writers_frozen"`
	RuntimeNotStarted  bool   `json:"first_new_runtime_command_not_issued"`
	Marker             string `json:"first_new_runtime_marker"`
	WindowStarted      string `json:"window_started_at"`
	RecoveryDecision   string `json:"recovery_decision_at"`
	Deadline           string `json:"restore_deadline"`
	Expires            string `json:"expires_at"`
	RetentionDays      int    `json:"protection_retention_days"`
	DataOwner          string `json:"data_owner"`
	RecoveryOwner      string `json:"recovery_owner"`
}

func nonzeroHash(s string) bool {
	return hashRE.MatchString(s) && s != string(bytes.Repeat([]byte{'0'}, 64))
}
func strictDecode(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > maxMaterialBytes || d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return errors.New("private_document_invalid")
	}
	return nil
}
func document(raw []byte) (map[string]bson.RawValue, error) {
	r := bson.Raw(raw)
	if len(raw) > 65536 || r.Validate() != nil {
		return nil, errors.New("metadata_bson_invalid")
	}
	elements, e := r.Elements()
	if e != nil {
		return nil, errors.New("metadata_bson_invalid")
	}
	m := make(map[string]bson.RawValue, len(elements))
	for _, field := range elements {
		if _, ok := m[field.Key()]; ok {
			return nil, errors.New("metadata_duplicate_bson_key")
		}
		m[field.Key()] = field.Value()
	}
	return m, nil
}
func orderedDocumentEquals(raw bson.RawValue, want bson.D) bool {
	b, e := bson.Marshal(want)
	d, ok := raw.DocumentOK()
	return e == nil && ok && bytes.Equal(d, b)
}
func indexName(raw []byte) (string, error) {
	d, e := document(raw)
	if e != nil {
		return "", e
	}
	n, ok := d["name"].StringValueOK()
	key, keyOK := d["key"].DocumentOK()
	if !ok || n == "" || len(n) > 4096 || !keyOK || key.Validate() != nil {
		return "", errors.New("index_metadata_invalid")
	}
	return n, nil
}
func validateTargetSpec(raw []byte, i int) error {
	if i < 0 || i >= len(targets) {
		return errors.New("target_index_invalid")
	}
	d, e := document(raw)
	if e != nil {
		return e
	}
	t := targets[i]
	v, vok := d["v"].Int32OK()
	n, nok := d["name"].StringValueOK()
	wantFields := 3
	if t.uniquePath != "" {
		wantFields = 6
	}
	if len(d) != wantFields || !vok || v != 2 || !nok || n != t.name || !orderedDocumentEquals(d["key"], t.key) {
		return errors.New("original_038_index_spec_rejected")
	}
	if t.uniquePath != "" {
		unique, ok := d["unique"].BooleanOK()
		partial := bson.D{{Key: t.uniquePath, Value: bson.D{{Key: "$type", Value: "string"}}}}
		if !ok || !unique || !orderedDocumentEquals(d["collation"], bson.D{{Key: "locale", Value: "simple"}}) || !orderedDocumentEquals(d["partialFilterExpression"], partial) {
			return errors.New("original_038_index_spec_rejected")
		}
	}
	return nil
}
func validateNamespace(n namespaceProof, name string) error {
	if n.Name != name || len(n.UUID) != 16 || bytes.Equal(n.UUID, make([]byte, 16)) || n.Count == nil || *n.Count < 0 || len(n.Indexes) < 1 || len(n.Indexes) > 256 {
		return errors.New("namespace_proof_invalid")
	}
	if _, e := document(n.Options); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, spec := range n.Indexes {
		name, e := indexName(spec)
		if e != nil || seen[name] {
			return errors.New("namespace_indexes_invalid")
		}
		seen[name] = true
	}
	if !seen["_id_"] {
		return errors.New("namespace_primary_index_missing")
	}
	return nil
}
func ledgerVersion(raw []byte) (int64, bool, error) {
	d, e := document(raw)
	if e != nil {
		return 0, false, e
	}
	v, ok := d["version"].AsInt64OK()
	dirty, dok := d["dirty"].BooleanOK()
	if len(d) != 3 || d["_id"].Type == 0 || d["_id"].Type == bson.TypeNull || (d["version"].Type != bson.TypeInt32 && d["version"].Type != bson.TypeInt64) || !ok || !dok {
		return 0, false, errors.New("complete_typed_ledger_bson_required")
	}
	return v, dirty, nil
}
func validateLedger(n namespaceProof) error {
	if e := validateNamespace(n, "schema_migrations"); e != nil {
		return e
	}
	options, _ := document(n.Options)
	if len(options) != 0 || *n.Count != 1 || len(n.Indexes) != 1 {
		return errors.New("provider_ledger_catalog_unsupported")
	}
	d, _ := document(n.Indexes[0])
	v, ok := d["v"].Int32OK()
	if len(d) != 3 || !ok || v != 2 || !orderedDocumentEquals(d["key"], bson.D{{Key: "_id", Value: int32(1)}}) {
		return errors.New("provider_ledger_catalog_unsupported")
	}
	return nil
}

func validateApproval(a approval, m material, materialHash, tool string, now time.Time) error {
	if a.Format != 1 || a.Kind != "m4_b_pre_start_mongo_object_restore" || !operationRE.MatchString(a.Operation) || a.ToolSource != tool || a.BSource != bSource || a.BResources != bResources || a.BResourceFiles != bResourceFiles || a.MaterialHash != materialHash || !nonzeroHash(materialHash) || !nonzeroHash(a.FreezeHash) || !nonzeroHash(a.ResponsibilityHash) || a.SQLIdentity != m.SQLIdentity || a.MongoIdentity != m.MongoIdentity || !a.RuntimeFrozen || !a.RuntimeNotStarted || a.RetentionDays < 30 || a.DataOwner != "system (10001)" || a.RecoveryOwner != "system (10001)" {
		return errors.New("restore_approval_binding_rejected")
	}
	start, e1 := time.Parse(time.RFC3339Nano, a.WindowStarted)
	decision, e2 := time.Parse(time.RFC3339Nano, a.RecoveryDecision)
	deadline, e3 := time.Parse(time.RFC3339Nano, a.Deadline)
	expiry, e4 := time.Parse(time.RFC3339Nano, a.Expires)
	captured, e5 := time.Parse(time.RFC3339Nano, m.CapturedAt)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || start.After(captured) || captured.After(decision) || decision.After(now) || !deadline.After(now) || deadline.After(decision.Add(time.Hour)) || !expiry.After(now) || expiry.After(now.Add(24*time.Hour)) {
		return errors.New("restore_approval_budget_rejected")
	}
	return markerAbsent(a.Marker)
}

// Same first-runtime boundary as the approved A tool. Absence is only a local
// guard; it never independently certifies that the six instances are frozen.
func markerAbsent(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("first_runtime_already_started_or_marker_unknown")
	}
	for p := path; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if p == path && os.IsNotExist(e) {
		} else if e != nil || s.Mode()&os.ModeSymlink != 0 || p == path || !s.IsDir() || (s.Mode().Perm()&022 != 0 && s.Mode()&os.ModeSticky == 0) {
			return errors.New("first_runtime_already_started_or_marker_unknown")
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}
func validateMaterial(m material, tool, sqlName, mongoName string) error {
	if m.Format != 1 || m.Kind != "m4_b_mongo_index_object_protection" || !sourceRE.MatchString(tool) || m.ToolSource != tool || m.BSource != bSource || m.BResources != bResources || m.BResourceFiles != bResourceFiles || !nameRE.MatchString(sqlName) || !nameRE.MatchString(mongoName) || m.SQLName != sqlName || m.MongoName != mongoName || !nonzeroHash(m.SQLIdentity) || !nonzeroHash(m.MongoIdentity) || m.SQLVersion != 101 {
		return errors.New("material_binding_rejected")
	}
	if _, e := time.Parse(time.RFC3339Nano, m.CapturedAt); e != nil {
		return errors.New("material_time_rejected")
	}
	for i, name := range []string{"answersheets", "report_generations"} {
		if e := validateNamespace(m.Collections[i], name); e != nil {
			return e
		}
		for target, fixed := range targets {
			if fixed.collection != name {
				continue
			}
			found := false
			for _, spec := range m.Collections[i].Indexes {
				n, _ := indexName(spec)
				if n == fixed.name {
					if e := validateTargetSpec(spec, target); e != nil {
						return e
					}
					found = true
				}
			}
			if !found {
				return errors.New("original_historical_index_missing")
			}
		}
	}
	if bytes.Equal(m.Collections[0].UUID, m.Collections[1].UUID) || bytes.Equal(m.Collections[0].UUID, m.Ledger.Namespace.UUID) || bytes.Equal(m.Collections[1].UUID, m.Ledger.Namespace.UUID) {
		return errors.New("namespace_physical_identity_repeated")
	}
	if e := validateLedger(m.Ledger.Namespace); e != nil {
		return e
	}
	v, dirty, e := ledgerVersion(m.Ledger.Document)
	if e != nil || v != 39 || dirty {
		return errors.New("original_clean39_complete_ledger_required")
	}
	return nil
}
