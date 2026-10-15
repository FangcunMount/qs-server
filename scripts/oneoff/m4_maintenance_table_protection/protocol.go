package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/sys/unix"
)

// These identify the D release, not the tool's independently compiled source.
const dSource = "c335b2a1a34dd69830261e2d8e11b16da3f8befb"
const dResources = "20e0096ffa50a50a7818bfd09741fa48ce8d6e0dc909bf0cf49e0292ba254c4d"
const dResourceFiles = 286
const pageSize = 500
const maxValueBytes = 8 << 20
const maxRowBytes = 12 << 20
const maxTableBytes = 128 << 20
const maxMaterialBytes = 256 << 20
const maxRows = 250000
const maxTotalRows = 500000
const maxDDLBytes = 65536
const queryLimit = 5 * time.Second
const maxAction = 20 * time.Minute
const sessionMode = "ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION"

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sourceRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}
type tableSpec struct {
	name        string
	columns     []column
	keys        []string
	binaryFirst bool
}

var scope = [...]tableSpec{
	{"testee_store_migration_reversions", []column{{"migration_id", "varchar(64)", false}, {"testee_id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"from_store_id", "bigint unsigned", false}, {"original_history_id", "bigint unsigned", false}, {"restored_version", "int unsigned", false}, {"actor_id", "bigint", false}, {"created_at", "datetime(6)", false}}, []string{"migration_id", "testee_id"}, true},
	{"testee_store_migration_items", []column{{"migration_id", "varchar(64)", false}, {"testee_id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"before_store_id", "bigint unsigned", true}, {"target_store_id", "bigint unsigned", true}, {"before_version", "int unsigned", false}, {"applied_version", "int unsigned", false}, {"history_id", "bigint unsigned", true}, {"disposition", "varchar(24)", false}, {"reason", "varchar(100)", false}}, []string{"migration_id", "testee_id"}, true},
	{"testee_store_migration_manifests", []column{{"migration_id", "varchar(64)", false}, {"state", "varchar(24)", false}, {"before_hash", "char(64)", false}, {"after_hash", "char(64)", false}, {"rollback_hash", "char(64)", false}, {"items_hash", "char(64)", false}, {"item_count", "bigint unsigned", false}, {"actor_id", "bigint", false}, {"created_at", "datetime(6)", false}, {"completed_at", "datetime(6)", true}, {"rolled_back_at", "datetime(6)", true}}, []string{"migration_id"}, true},
	{"clinician_operator_binding_archive", []column{{"clinician_id", "bigint unsigned", false}, {"org_id", "bigint", false}, {"operator_id", "bigint unsigned", false}, {"migration_id", "varchar(64)", false}, {"before_record", "longtext", false}, {"record_sha256", "char(64)", false}, {"archived_at", "datetime(6)", false}}, []string{"clinician_id"}, false},
	{"operator_retirement_manifests", []column{{"migration_id", "varchar(64)", false}, {"stage", "varchar(24)", false}, {"payload", "longtext", false}, {"payload_sha256", "char(64)", false}, {"created_at", "datetime(6)", false}, {"updated_at", "datetime(6)", false}}, []string{"migration_id"}, true},
}

// Bytes are CAST(column AS BINARY) from the bound SQL session, not JSON/text
// authoring values. SQL NULL and an empty non-NULL value have distinct frames.
type cell struct {
	SQLNull bool   `json:"sql_null"`
	Bytes   []byte `json:"server_bytes"`
}
type rowProof struct {
	Key    []string `json:"primary_key"`
	Values []cell   `json:"original_values"`
	Hash   string   `json:"server_row_sha256"`
}
type tableProof struct {
	Table            string     `json:"table"`
	DDL              string     `json:"original_ddl"`
	Columns          []column   `json:"columns"`
	Keys             []string   `json:"primary_key_columns"`
	Rows             []rowProof `json:"rows"`
	Pages            int        `json:"pages_including_empty_eof"`
	EOF              bool       `json:"mandatory_empty_eof"`
	EncodedRowsBytes int        `json:"encoded_rows_bytes"`
}
type ledgerRow struct {
	Version uint64 `json:"version"`
	Dirty   bool   `json:"dirty"`
}
type binding struct {
	SQLName       string `json:"mysql_database"`
	MongoName     string `json:"mongo_database"`
	SQLIdentity   string `json:"mysql_identity_sha256"`
	MongoIdentity string `json:"mongo_identity_sha256"`
	MongoUUID     string `json:"mongo_ledger_uuid_sha256"`
}
type material struct {
	Format         int           `json:"format_version"`
	Kind           string        `json:"kind"`
	ToolSource     string        `json:"compiled_tool_source"`
	DSource        string        `json:"d_execution_source"`
	DResources     string        `json:"d_execution_resources_sha256"`
	DResourceFiles int           `json:"d_execution_resource_files"`
	CapturedAt     string        `json:"captured_at"`
	CapturePath    string        `json:"first_capture_path"`
	Binding        binding       `json:"binding"`
	SessionZone    string        `json:"session_time_zone"`
	SessionCharset string        `json:"session_character_set_results"`
	SessionSQLMode string        `json:"session_sql_mode"`
	LedgerDDL      string        `json:"original_sql_ledger_ddl"`
	Ledger         []ledgerRow   `json:"original_complete_sql_ledger"`
	MongoHead      []byte        `json:"independently_observed_mongo40_bson"`
	Tables         [5]tableProof `json:"tables"`
}

func fingerprint(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func validBinding(b binding) bool {
	return nameRE.MatchString(b.SQLName) && nameRE.MatchString(b.MongoName) && nonzeroHash(b.SQLIdentity) && nonzeroHash(b.MongoIdentity) && nonzeroHash(b.MongoUUID)
}
func validKey(s tableSpec, k []string) bool {
	if len(k) != len(s.keys) {
		return false
	}
	for i, v := range k {
		if i == 0 && s.binaryFirst {
			b, e := hex.DecodeString(v)
			if e != nil || len(b) > 256 || strings.ToUpper(v) != v {
				return false
			}
		} else {
			n, e := strconv.ParseUint(v, 10, 64)
			if e != nil || strconv.FormatUint(n, 10) != v {
				return false
			}
		}
	}
	return true
}
func keyLess(s tableSpec, a, b []string) bool {
	for i := range a {
		if a[i] == b[i] {
			continue
		}
		if i == 0 && s.binaryFirst {
			return a[i] < b[i]
		}
		x, e1 := strconv.ParseUint(a[i], 10, 64)
		y, e2 := strconv.ParseUint(b[i], 10, 64)
		return e1 == nil && e2 == nil && x < y
	}
	return false
}
func rowHash(cols []column, values []cell) string {
	var b bytes.Buffer
	for i, c := range cols {
		b.WriteString(c.Name + ":" + c.Type + ":")
		if values[i].SQLNull {
			b.WriteString("N;")
		} else {
			b.WriteString("V" + strconv.Itoa(len(values[i].Bytes)) + ":" + strings.ToUpper(hex.EncodeToString(values[i].Bytes)) + ";")
		}
	}
	return fingerprint(b.Bytes())
}
func validateRow(s tableSpec, row rowProof) error {
	if !validKey(s, row.Key) || len(row.Values) != len(s.columns) || !hashRE.MatchString(row.Hash) {
		return errors.New("material_row_shape_rejected")
	}
	size := 0
	for i, v := range row.Values {
		if len(v.Bytes) > maxValueBytes || !v.SQLNull && v.Bytes == nil || v.SQLNull && (v.Bytes != nil || !s.columns[i].Nullable) {
			return errors.New("material_cell_rejected")
		}
		size += len(v.Bytes)
	}
	if size > maxRowBytes || row.Hash != rowHash(s.columns, row.Values) {
		return errors.New("material_row_hash_or_limit_rejected")
	}
	for n, key := range s.keys {
		for i, c := range s.columns {
			if c.Name != key {
				continue
			}
			v := row.Values[i]
			actual := string(v.Bytes)
			if n == 0 && s.binaryFirst {
				actual = strings.ToUpper(hex.EncodeToString(v.Bytes))
			}
			if v.SQLNull || actual != row.Key[n] {
				return errors.New("material_primary_key_value_rejected")
			}
		}
	}
	return nil
}
func cleanMongo40(raw bson.Raw) bool {
	el, e := raw.Elements()
	if e != nil || len(el) != 3 {
		return false
	}
	seen := map[string]bool{}
	for _, v := range el {
		if seen[v.Key()] || (v.Key() != "_id" && v.Key() != "version" && v.Key() != "dirty") {
			return false
		}
		seen[v.Key()] = true
	}
	id := raw.Lookup("_id")
	if id.Type == 0 || id.Type == bson.TypeNull || id.Type == bson.TypeUndefined {
		return false
	}
	v, ok := raw.Lookup("version").Int64OK()
	if !ok {
		x, valid := raw.Lookup("version").Int32OK()
		if !valid {
			return false
		}
		v = int64(x)
	}
	dirty, ok := raw.Lookup("dirty").BooleanOK()
	return v == 40 && ok && !dirty
}
func validateMaterial(m material, tool string, b binding) error {
	if m.Format != 1 || m.Kind != "m4_d_maintenance_table_protection" || !sourceRE.MatchString(tool) || m.ToolSource != tool || m.DSource != dSource || m.DResources != dResources || m.DResourceFiles != dResourceFiles || m.Binding != b || !validBinding(b) || !absolutePath(m.CapturePath) || m.SessionZone != "+00:00" || m.SessionCharset != "utf8mb4" || m.SessionSQLMode != sessionMode || m.LedgerDDL == "" || len(m.LedgerDDL) > maxDDLBytes || len(m.Ledger) != 1 || m.Ledger[0] != (ledgerRow{102, false}) || !cleanMongo40(m.MongoHead) {
		return errors.New("material_binding_rejected")
	}
	if _, e := time.Parse(time.RFC3339Nano, m.CapturedAt); e != nil {
		return errors.New("material_time_rejected")
	}
	totalRows, totalBytes := 0, 0
	for i, t := range m.Tables {
		s := scope[i]
		if t.Table != s.name || t.DDL == "" || len(t.DDL) > maxDDLBytes || !sameColumns(t.Columns, s.columns) || !sameStrings(t.Keys, s.keys) || !t.EOF || len(t.Rows) > maxRows || t.Pages != (len(t.Rows)+pageSize-1)/pageSize+1 {
			return errors.New("material_table_shape_or_eof_rejected")
		}
		used := 0
		for n, row := range t.Rows {
			if validateRow(s, row) != nil || n > 0 && !keyLess(s, t.Rows[n-1].Key, row.Key) {
				return errors.New("material_rows_rejected")
			}
			raw, e := json.Marshal(row)
			if e != nil {
				return errors.New("material_encoding_rejected")
			}
			used += len(raw) + 1
			if used > maxTableBytes {
				return errors.New("material_table_limit_rejected")
			}
		}
		if used != t.EncodedRowsBytes {
			return errors.New("material_table_length_rejected")
		}
		totalRows += len(t.Rows)
		totalBytes += used
	}
	if totalRows > maxTotalRows || totalBytes > maxMaterialBytes {
		return errors.New("material_total_limit_rejected")
	}
	return nil
}
func sameColumns(a, b []column) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Reject duplicate JSON members in addition to unknown schema fields. Raw JSON
// column values are base64 bytes, so their original content is not traversed.
func strictDecode(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		if delim, ok := t.(json.Delim); ok {
			seen := map[string]bool{}
			for d.More() {
				if delim == '{' {
					t, e := d.Token()
					key, ok := t.(string)
					if e != nil || !ok || seen[key] {
						return errors.New("private_document_duplicate_member")
					}
					seen[key] = true
				}
				if e := walk(); e != nil {
					return e
				}
			}
			_, e = d.Token()
		}
		return e
	}
	if walk() != nil {
		return errors.New("private_document_invalid")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(new(any)) != io.EOF {
		return errors.New("private_document_invalid")
	}
	return nil
}

func privateParent(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("private_path_rejected")
	}
	fd, e := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if e != nil {
		return nil, errors.New("private_directory_unavailable")
	}
	for _, name := range strings.Split(strings.TrimPrefix(filepath.Dir(path), "/"), "/") {
		if name == "" {
			continue
		}
		next, e := unix.Openat(fd, name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
		_ = syscall.Close(fd)
		if e != nil {
			return nil, errors.New("private_directory_rejected")
		}
		fd = next
		var s syscall.Stat_t
		if syscall.Fstat(fd, &s) != nil || (s.Uid != 0 && s.Uid != uint32(os.Getuid())) || s.Mode&0022 != 0 {
			_ = syscall.Close(fd)
			return nil, errors.New("private_directory_rejected")
		}
	}
	var last syscall.Stat_t
	if syscall.Fstat(fd, &last) != nil || last.Uid != uint32(os.Getuid()) || last.Mode&0777 != 0700 {
		_ = syscall.Close(fd)
		return nil, errors.New("private_leaf_directory_required")
	}
	return os.NewFile(uintptr(fd), filepath.Dir(path)), nil
}
func absolutePath(path string) bool { return filepath.IsAbs(path) && filepath.Clean(path) == path }
func nonzeroHash(s string) bool     { return hashRE.MatchString(s) && s != strings.Repeat("0", 64) }

func readPrivate(path, expected string) ([]byte, error) {
	return readPrivateLimit(path, expected, maxMaterialBytes)
}
func readPrivateLimit(path, expected string, limit int64) ([]byte, error) {
	if !nonzeroHash(expected) {
		return nil, errors.New("external_fingerprint_required")
	}
	raw, e := readOwnedPrivate(path, limit)
	if e != nil || fingerprint(raw) != expected {
		return nil, errors.New("private_material_fingerprint_or_release_rejected")
	}
	return raw, nil
}

// This bounded file read is internal to readPrivate; callers must supply the
// independently frozen SHA before any material can be accepted.
func readOwnedPrivate(path string, limit int64) (raw []byte, resultErr error) {
	if !absolutePath(path) || limit <= 0 || limit > maxMaterialBytes {
		return nil, errors.New("private_path_or_limit_rejected")
	}
	parent, e := privateParent(path)
	if e != nil {
		return nil, e
	}
	defer func() {
		if parent.Close() != nil {
			raw = nil
			resultErr = errors.Join(resultErr, errors.New("private_directory_release_unproven"))
		}
	}()
	fd, e := unix.Openat(int(parent.Fd()), filepath.Base(path), syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, errors.New("private_material_unavailable")
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() {
		if f.Close() != nil {
			raw = nil
			resultErr = errors.Join(resultErr, errors.New("private_file_release_unproven"))
		}
	}()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() <= 0 || st.Size() > limit {
		return nil, errors.New("private_material_shape_rejected")
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Getuid()) || s.Nlink != 1 {
		return nil, errors.New("private_material_owner_rejected")
	}
	raw, e = io.ReadAll(io.LimitReader(f, limit+1))
	after, statErr := f.Stat()
	if e != nil || statErr != nil || st.Size() != after.Size() || st.ModTime() != after.ModTime() || len(raw) != int(st.Size()) {
		return nil, errors.New("private_material_fingerprint_rejected")
	}
	return raw, nil
}
func writeExclusive(path string, raw []byte) (resultErr error) {
	parent, e := privateParent(path)
	if e != nil {
		return e
	}
	defer func() {
		if parent.Close() != nil {
			resultErr = errors.Join(resultErr, errors.New("private_directory_release_unproven"))
		}
	}()
	fd, e := unix.Openat(int(parent.Fd()), filepath.Base(path), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return errors.New("new_private_capture_required")
	}
	f := os.NewFile(uintptr(fd), path)
	n, we := f.Write(raw)
	se, ce := f.Sync(), f.Close()
	pe := parent.Sync()
	if n != len(raw) || we != nil || se != nil || ce != nil || pe != nil {
		return errors.New("capture_persistence_unproven")
	}
	_, e = readPrivate(path, fingerprint(raw))
	return e
}

// Capture binds the real parent identity before ENV/DB and checks it again on
// both sides of the O_EXCL, fsync and original-byte readback. A changed parent
// never yields a successful material receipt.
func sameCaptureParent(path string, initial os.FileInfo, uid uint32) error {
	p, e := privateParent(path)
	if e != nil {
		return errors.New("capture_parent_changed")
	}
	current, e := p.Stat()
	ce := p.Close()
	if e != nil || ce != nil || initial == nil || !os.SameFile(initial, current) {
		return errors.New("capture_parent_changed")
	}
	st, ok := current.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uid {
		return errors.New("capture_parent_changed")
	}
	return nil
}
func finishCapture(ctx context.Context, path string, raw []byte, initial os.FileInfo, uid uint32) error {
	if len(raw) == 0 || len(raw) > maxMaterialBytes {
		return errors.New("material_encoding_limit")
	}
	if e := ctx.Err(); e != nil {
		return errors.New("total_budget_exceeded")
	}
	if e := sameCaptureParent(path, initial, uid); e != nil {
		return e
	}
	if e := writeExclusive(path, raw); e != nil {
		return e
	}
	if e := sameCaptureParent(path, initial, uid); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	return nil
}
