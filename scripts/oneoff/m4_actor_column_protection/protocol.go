package main

import (
	"bytes"
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

const cSource = "040b122d7dfe77b1119d9ed6969a54abd6a9fbdf"
const cResources = "fadbb5a33baed5b2fb40a6f1efdf4a20dc5a1525ab2a0090486b7d5832437814"
const cResourceFiles = 284
const pageSize = 1000
const maxRows = 500000
const maxMaterialBytes = 128 << 20
const maxValueBytes = 1 << 20
const queryLimit = 5 * time.Second
const sessionMode = "ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION"

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var operationRE = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)
var sourceRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var scope = [...]struct {
	table, key, target string
	binaryKey          bool
}{
	{"testee", "id", "tags", false},
	{"operators", "id", "effective_roles", false},
	{"operator_retirement_tasks", "operator_id", "", false},
	{"operator_recovery_archives", "request_id", "", true},
}

type column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// JSON is the server's original representation, not an authoring input decoded
// and re-encoded through a map. SQL NULL, JSON null and [] remain distinct.
type originalValue struct {
	Kind string `json:"kind"`
	JSON []byte `json:"server_json_bytes"`
}
type rowProof struct {
	Key      string         `json:"primary_key"`
	Retained string         `json:"retained_sha256"`
	Value    *originalValue `json:"original_value,omitempty"`
}
type tableProof struct {
	Table       string     `json:"table"`
	DDL         string     `json:"original_ddl"`
	RetainedDDL string     `json:"retained_ddl"`
	Columns     []column   `json:"retained_columns"`
	Rows        []rowProof `json:"rows"`
	Pages       int        `json:"pages_including_empty_eof"`
	EOF         bool       `json:"mandatory_empty_eof"`
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
	CSource        string        `json:"c_execution_source"`
	CResources     string        `json:"c_execution_resources_sha256"`
	CResourceFiles int           `json:"c_execution_resource_files"`
	CapturedAt     string        `json:"captured_at"`
	CapturePath    string        `json:"first_capture_path"`
	Binding        binding       `json:"binding"`
	SessionZone    string        `json:"session_time_zone"`
	SessionCharset string        `json:"session_character_set_results"`
	SessionSQLMode string        `json:"session_sql_mode"`
	LedgerDDL      string        `json:"original_sql_ledger_ddl"`
	Ledger         []ledgerRow   `json:"original_complete_sql_ledger"`
	MongoHead      []byte        `json:"independently_observed_mongo40_bson"`
	Tables         [4]tableProof `json:"tables"`
}

func fingerprint(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func checkValue(v *originalValue) bool {
	if v == nil {
		return false
	}
	if v.Kind == "sql_null" {
		return len(v.JSON) == 0
	}
	return v.Kind == "json" && len(v.JSON) > 0 && len(v.JSON) <= maxValueBytes && json.Valid(v.JSON)
}
func validKey(key string, binary bool) bool {
	if binary {
		raw, e := hex.DecodeString(key)
		return e == nil && len(raw) > 0 && len(raw) <= 256 && strings.ToUpper(key) == key
	}
	n, e := strconv.ParseUint(key, 10, 64)
	return e == nil && strconv.FormatUint(n, 10) == key
}
func keyLess(a, b string, binary bool) bool {
	if binary {
		return a < b
	}
	x, e1 := strconv.ParseUint(a, 10, 64)
	y, e2 := strconv.ParseUint(b, 10, 64)
	return e1 == nil && e2 == nil && x < y
}
func cleanMongo40(raw bson.Raw) bool {
	elements, e := raw.Elements()
	if e != nil || len(elements) != 3 {
		return false
	}
	seen := map[string]bool{}
	for _, element := range elements {
		if seen[element.Key()] || (element.Key() != "_id" && element.Key() != "version" && element.Key() != "dirty") {
			return false
		}
		seen[element.Key()] = true
	}
	id := raw.Lookup("_id")
	if id.Type == 0 || id.Type == bson.TypeNull || id.Type == bson.TypeUndefined {
		return false
	}
	v, ok := raw.Lookup("version").Int64OK()
	if !ok {
		n, valid := raw.Lookup("version").Int32OK()
		if !valid {
			return false
		}
		v = int64(n)
	}
	dirty, ok := raw.Lookup("dirty").BooleanOK()
	return v == 40 && ok && !dirty
}

// Only the named middle column is projected out. No general DDL normalization
// or AUTO_INCREMENT rewrite is permitted by the read-only protocol.
func retainedDDL(ddl string, index int, present bool) (string, error) {
	if index >= 2 || !present {
		return ddl, nil
	}
	lines := strings.Split(ddl, "\n")
	matched := -1
	want := "  `tags` json DEFAULT NULL COMMENT '标签（JSON数组）',"
	previous := "  `birthday` "
	if index == 1 {
		want, previous = "  `effective_roles` json NOT NULL,", "  `roles` "
	}
	for n, line := range lines {
		if strings.HasPrefix(line, "  `"+scope[index].target+"` ") {
			if matched != -1 || line != want || n == 0 || n == len(lines)-1 || !strings.HasPrefix(lines[n-1], previous) {
				return "", errors.New("original_C_column_definition_rejected")
			}
			matched = n
		}
	}
	if matched < 0 {
		return "", errors.New("original_C_column_definition_rejected")
	}
	return strings.Join(append(lines[:matched], lines[matched+1:]...), "\n"), nil
}

func validateMaterial(m material, tool string, b binding) error {
	if !absolutePath(m.CapturePath) {
		return errors.New("material_first_capture_path_required")
	}
	if m.Format != 1 || m.Kind != "m4_c_actor_column_protection" || !sourceRE.MatchString(tool) || m.ToolSource != tool || m.CSource != cSource || m.CResources != cResources || m.CResourceFiles != cResourceFiles || m.Binding != b || !validBinding(b) || m.SessionZone != "+00:00" || m.SessionCharset != "utf8mb4" || m.SessionSQLMode != sessionMode || m.LedgerDDL == "" || len(m.LedgerDDL) > 65536 || len(m.Ledger) != 1 || m.Ledger[0] != (ledgerRow{101, false}) || !cleanMongo40(m.MongoHead) {
		return errors.New("material_binding_rejected")
	}
	if _, e := time.Parse(time.RFC3339Nano, m.CapturedAt); e != nil {
		return errors.New("material_time_rejected")
	}
	for i, t := range m.Tables {
		d, e := retainedDDL(t.DDL, i, i < 2)
		if e != nil || t.Table != scope[i].table || len(t.DDL) > 65536 || t.DDL == "" || d != t.RetainedDDL || !t.EOF || len(t.Rows) > maxRows || t.Pages != (len(t.Rows)+pageSize-1)/pageSize+1 || len(t.Columns) == 0 || len(t.Columns) > 128 {
			return errors.New("material_fixed_table_rejected")
		}
		seen := map[string]bool{}
		for _, c := range t.Columns {
			if !nameRE.MatchString(c.Name) || c.Name == scope[i].target || c.Type == "" || len(c.Type) > 255 || seen[c.Name] {
				return errors.New("material_columns_rejected")
			}
			seen[c.Name] = true
		}
		if !seen[scope[i].key] {
			return errors.New("material_primary_key_rejected")
		}
		for n, row := range t.Rows {
			if !validKey(row.Key, scope[i].binaryKey) || !hashRE.MatchString(row.Retained) || (n > 0 && !keyLess(t.Rows[n-1].Key, row.Key, scope[i].binaryKey)) || (i < 2 && !checkValue(row.Value)) || (i == 1 && row.Value.Kind == "sql_null") || (i >= 2 && row.Value != nil) {
				return errors.New("material_rows_rejected")
			}
		}
	}
	return nil
}
func validBinding(b binding) bool {
	return nameRE.MatchString(b.SQLName) && nameRE.MatchString(b.MongoName) && hashRE.MatchString(b.SQLIdentity) && hashRE.MatchString(b.MongoIdentity) && hashRE.MatchString(b.MongoUUID)
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

// Only local boundary/claim state uses a bounded read without an external SHA.
// External original materials and approvals always use readPrivate.
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
