package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const aSource = "51163edab7038adcdb6b8f1712d6dc434f361dc6"
const aResourceFiles = 280
const aResources = "77e11d801d270ef7b12f86261a05217a95bd8f8049f20d2e5f14ee0c0a20083b"
const originalDDLHash = "575fa26f588e4fb87ee7bbcdc61fc11eeb8fe8ed0234518209e7e3e13b7b0d7a"
const maxMaterialBytes = 128 << 20
const maxRows = 500000
const pageSize = 1000
const sessionMode = "ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION"

// This immutable copy is checked against the unchanged historical resource.
// It restores only the two named objects, never a whole table or arbitrary SQL.
//
//go:embed historical_columns.sql
var originalDDL string

var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sourceRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var operationRE = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)

var objects = [2]struct{ table, column, check string }{
	{"assessment", "historical_lifecycle_evidence", "chk_assessment_historical_lifecycle"},
	{"evaluation_outcome", "historical_committed_evidence", "chk_outcome_historical_committed"},
}

type column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type rowProof struct {
	ID     uint64 `json:"id"`
	SHA256 string `json:"retained_sha256"`
}
type tableProof struct {
	Table       string     `json:"table"`
	OriginalDDL string     `json:"original_ddl"`
	RetainedDDL string     `json:"retained_ddl"`
	Columns     []column   `json:"retained_columns"`
	Rows        []rowProof `json:"primary_key_retained_hashes"`
	Pages       int        `json:"pages_including_empty_eof"`
	EOF         bool       `json:"mandatory_eof"`
	SQLNullOnly bool       `json:"sql_null_only"`
}
type ledgerRow struct {
	Version uint64 `json:"version"`
	Dirty   bool   `json:"dirty"`
}
type material struct {
	Format         int           `json:"format_version"`
	Kind           string        `json:"kind"`
	ToolSource     string        `json:"compiled_tool_source"`
	ASource        string        `json:"a_execution_source"`
	AResources     string        `json:"a_execution_resources_sha256"`
	AResourceFiles int           `json:"a_execution_resource_files"`
	CapturedAt     string        `json:"captured_at"`
	CapturePath    string        `json:"first_capture_path"`
	SQLName        string        `json:"mysql_database"`
	MongoName      string        `json:"mongo_database"`
	SQLIdentity    string        `json:"mysql_identity_sha256"`
	MongoIdentity  string        `json:"mongo_identity_sha256"`
	MongoUUID      string        `json:"mongo_ledger_uuid_sha256"`
	MongoHead      []byte        `json:"original_mongo_ledger_bson"`
	SessionZone    string        `json:"session_time_zone"`
	SessionCharset string        `json:"session_character_set_results"`
	SessionSQLMode string        `json:"session_sql_mode"`
	LedgerDDL      string        `json:"original_sql_ledger_ddl"`
	Ledger         []ledgerRow   `json:"original_complete_sql_ledger"`
	Tables         [2]tableProof `json:"tables"`
}
type approval struct {
	Format             int    `json:"format_version"`
	Kind               string `json:"kind"`
	Operation          string `json:"operation_id"`
	ToolSource         string `json:"compiled_tool_source"`
	ASource            string `json:"a_execution_source"`
	AResources         string `json:"a_execution_resources_sha256"`
	AResourceFiles     int    `json:"a_execution_resource_files"`
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

func fingerprint(raw []byte) string { v := sha256.Sum256(raw); return hex.EncodeToString(v[:]) }
func strictDecode(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return errors.New("private_document_invalid")
	}
	return nil
}
func validateMaterial(m material, tool, sqlName, mongoName string) error {
	if !filepath.IsAbs(m.CapturePath) || filepath.Clean(m.CapturePath) != m.CapturePath {
		return errors.New("material_first_capture_path_required")
	}
	if m.Format != 1 || m.Kind != "m4_a_sql_null_object_protection" || m.ToolSource != tool || !sourceRE.MatchString(tool) || m.ASource != aSource || m.AResources != aResources || m.AResourceFiles != aResourceFiles || m.SQLName != sqlName || m.MongoName != mongoName || !hashRE.MatchString(m.SQLIdentity) || !hashRE.MatchString(m.MongoIdentity) || !hashRE.MatchString(m.MongoUUID) || len(m.MongoHead) == 0 || m.SessionZone != "+00:00" || m.SessionCharset != "utf8mb4" || m.SessionSQLMode != sessionMode || len(m.Ledger) != 1 || m.Ledger[0] != (ledgerRow{100, false}) || m.LedgerDDL == "" {
		return errors.New("material_binding_rejected")
	}
	if _, e := time.Parse(time.RFC3339Nano, m.CapturedAt); e != nil {
		return errors.New("material_time_rejected")
	}
	for i, t := range m.Tables {
		if t.Table != objects[i].table || !t.EOF || !t.SQLNullOnly || len(t.Rows) > maxRows || t.Pages < 1 || t.OriginalDDL == "" || t.RetainedDDL != normalizeDDL(t.OriginalDDL, i) || len(t.Columns) < 1 || len(t.Columns) > 128 {
			return errors.New("material_table_rejected")
		}
		if _, e := validateOriginalObjectDefinition(t.OriginalDDL, i); e != nil {
			return errors.New("material_original_099_definition_rejected")
		}
		seen := map[string]bool{}
		for _, c := range t.Columns {
			if !nameRE.MatchString(c.Name) || c.Name == objects[i].column || c.Type == "" || len(c.Type) > 255 || seen[c.Name] {
				return errors.New("material_columns_rejected")
			}
			seen[c.Name] = true
		}
		if !seen["id"] {
			return errors.New("material_primary_key_rejected")
		}
		for n, r := range t.Rows {
			if !hashRE.MatchString(r.SHA256) || (n > 0 && r.ID <= t.Rows[n-1].ID) {
				return errors.New("material_rows_rejected")
			}
		}
	}
	return nil
}

// MySQL8.0.36 SHOW CREATE renders the original OCTET_LENGTH alias as length.
// Accept only the two fixed ASCII literal encodings of the immutable 099;
// no generic SQL normalization or material-supplied DDL is executed.
type originalEncoding uint8

const (
	originalUTF8MB3 originalEncoding = 3
	originalUTF8MB4 originalEncoding = 4
)

func originalCheckDefinition(i int, encoding originalEncoding) (string, error) {
	if i < 0 || i >= len(objects) || fingerprint([]byte(originalDDL)) != originalDDLHash {
		return "", errors.New("compiled_original_ddl_changed")
	}
	parts := strings.SplitN(originalDDL, "ALTER TABLE `"+objects[i].table+"`", 2)
	if len(parts) != 2 {
		return "", errors.New("original_check_resource_invalid")
	}
	parts = strings.SplitN(parts[1], "JSON_SCHEMA_VALID('", 2)
	if len(parts) != 2 {
		return "", errors.New("original_check_resource_invalid")
	}
	parts = strings.SplitN(parts[1], "', `"+objects[i].column+"`)", 2)
	if len(parts) != 2 || !json.Valid([]byte(parts[0])) || strings.ContainsAny(parts[0], "'\\") {
		return "", errors.New("original_check_resource_invalid")
	}
	for _, b := range []byte(parts[0]) {
		if b >= 128 {
			return "", errors.New("original_check_resource_invalid")
		}
	}
	var intro string
	switch encoding {
	case originalUTF8MB3:
		intro = "_utf8mb3"
	case originalUTF8MB4:
		intro = "_utf8mb4"
	default:
		return "", errors.New("original_check_resource_invalid")
	}
	c := objects[i].column
	return fmt.Sprintf("CONSTRAINT `%s` CHECK (((`%s` is null) or (json_schema_valid(%s'%s',`%s`) and (length(cast(`%s` as char charset utf8mb4)) <= 524288))))", objects[i].check, c, intro, parts[0], c, c), nil
}

func validateOriginalObjectDefinition(text string, i int) (originalEncoding, error) {
	want3, e := originalCheckDefinition(i, originalUTF8MB3)
	if e != nil {
		return 0, e
	}
	want4, e := originalCheckDefinition(i, originalUTF8MB4)
	if e != nil {
		return 0, e
	}
	columnCount, checkCount := 0, 0
	lastColumn := ""
	var encoding originalEncoding
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSuffix(strings.TrimSpace(raw), ",")
		if strings.HasPrefix(line, "`") {
			parts := strings.SplitN(line, "`", 3)
			if len(parts) != 3 {
				return 0, errors.New("original_099_object_definition_rejected")
			}
			lastColumn = parts[1]
			if lastColumn == objects[i].column {
				columnCount++
				if line != "`"+objects[i].column+"` json DEFAULT NULL" {
					return 0, errors.New("original_099_object_definition_rejected")
				}
			}
		}
		if strings.HasPrefix(line, "CONSTRAINT `"+objects[i].check+"`") {
			checkCount++
			switch line {
			case want3:
				encoding = originalUTF8MB3
			case want4:
				encoding = originalUTF8MB4
			default:
				return 0, errors.New("original_099_object_definition_rejected")
			}
		}
	}
	if columnCount != 1 || checkCount != 1 || lastColumn != objects[i].column {
		return 0, errors.New("original_099_object_definition_rejected")
	}
	return encoding, nil
}
func validateApproval(a approval, m material, expected, tool string, now time.Time) error {
	if a.Format != 1 || a.Kind != "m4_a_pre_start_sql_object_restore" || !operationRE.MatchString(a.Operation) || a.ToolSource != tool || a.ASource != aSource || a.AResources != aResources || a.AResourceFiles != aResourceFiles || a.MaterialHash != expected || !hashRE.MatchString(expected) || !nonzeroHash(a.FreezeHash) || !nonzeroHash(a.ResponsibilityHash) || a.SQLIdentity != m.SQLIdentity || a.MongoIdentity != m.MongoIdentity || !a.RuntimeFrozen || !a.RuntimeNotStarted || a.RetentionDays < 30 || a.DataOwner != "system (10001)" || a.RecoveryOwner != "system (10001)" {
		return errors.New("restore_approval_binding_rejected")
	}
	start, e1 := time.Parse(time.RFC3339Nano, a.WindowStarted)
	decision, e2 := time.Parse(time.RFC3339Nano, a.RecoveryDecision)
	deadline, e3 := time.Parse(time.RFC3339Nano, a.Deadline)
	expiry, e4 := time.Parse(time.RFC3339Nano, a.Expires)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || start.After(decision) || decision.After(now) || !deadline.After(now) || deadline.After(decision.Add(time.Hour)) || !expiry.After(now) || expiry.After(now.Add(24*time.Hour)) {
		return errors.New("restore_approval_budget_rejected")
	}
	captured, e := time.Parse(time.RFC3339Nano, m.CapturedAt)
	if e != nil || captured.Before(start) || captured.After(decision) {
		return errors.New("capture_not_bound_to_actual_window")
	}
	return checkMarker(a.Marker)
}
func privatePath(path string, mustExist bool) (os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("private_path_rejected")
	}
	var leaf os.FileInfo
	for p := path; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if p == path && !mustExist && os.IsNotExist(e) {
		} else if e != nil || s.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("private_path_rejected")
		} else if p == path {
			leaf = s
		} else if !s.IsDir() || (s.Mode().Perm()&022 != 0 && s.Mode()&os.ModeSticky == 0) {
			return nil, errors.New("private_ancestor_rejected")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return leaf, nil
}
func readPrivate(path, expected string, limit int64) ([]byte, error) {
	if !hashRE.MatchString(expected) {
		return nil, errors.New("external_fingerprint_required")
	}
	raw, e := readOwnedPrivate(path, limit)
	if e != nil {
		return nil, e
	}
	if fingerprint(raw) != expected {
		return nil, errors.New("private_file_hash_or_release_failed")
	}
	return raw, nil
}

// Only tool-owned recovery state in the original capture's fixed private parent
// has no caller-supplied SHA. Material and approval inputs still use readPrivate.
func readOwnedPrivate(path string, limit int64) ([]byte, error) {
	s, e := privatePath(path, true)
	if e != nil || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || s.Sys().(*syscall.Stat_t).Nlink != 1 || (s.Sys().(*syscall.Stat_t).Uid != 0 && s.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid())) || s.Size() < 1 || s.Size() > limit {
		return nil, errors.New("private_file_rejected")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, errors.New("private_file_open_failed")
	}
	opened, e := f.Stat()
	if e != nil || !os.SameFile(s, opened) {
		_ = f.Close()
		return nil, errors.New("private_file_changed")
	}
	raw, e := io.ReadAll(io.LimitReader(f, limit+1))
	after, se := f.Stat()
	ce := f.Close()
	current, pe := os.Lstat(path)
	if e != nil || se != nil || ce != nil || pe != nil || !os.SameFile(opened, after) || !os.SameFile(after, current) || after.Mode().Perm() != 0600 || after.Sys().(*syscall.Stat_t).Nlink != 1 || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || len(raw) > int(limit) {
		return nil, errors.New("private_file_hash_or_release_failed")
	}
	return raw, nil
}
func writeExclusive(path string, raw []byte) error {
	if len(raw) < 1 || len(raw) > maxMaterialBytes {
		return errors.New("material_size_rejected")
	}
	if s, e := privatePath(path, false); e != nil || s != nil {
		return errors.New("first_capture_exists_or_path_rejected")
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return errors.New("first_capture_create_failed")
	}
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return errors.New("first_capture_write_or_release_failed")
	}
	return syncDirectory(filepath.Dir(path))
}
func syncDirectory(path string) error {
	d, e := os.Open(path)
	if e != nil {
		return errors.New("private_directory_open_failed")
	}
	se := d.Sync()
	ce := d.Close()
	if se != nil || ce != nil {
		return errors.New("private_directory_release_failed")
	}
	return nil
}
func checkMarker(path string) error {
	s, e := privatePath(path, false)
	if e != nil || s != nil {
		return errors.New("first_runtime_already_started_or_marker_unknown")
	}
	return nil
}
func nonzeroHash(s string) bool {
	return hashRE.MatchString(s) && s != "0000000000000000000000000000000000000000000000000000000000000000"
}
