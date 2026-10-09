package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
)

var sourceSHA string // Supplied only by the build, never by database rows.
var sourcePattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var runPattern = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)

const maximumJSONBytes = 256 << 10
const maximumEncodedSourceBytes = 2*retirement.MaxSourceBytes + (64 << 20)

var historyTargets = [][3]string{{"mysql", "domain_event_outbox", "base_table"}, {"mysql", "ai_bridge_commands", "base_table"}, {"mysql", "ai_messaging_legacy_commands", "base_table"}, {"mongodb", "domain_event_outbox", "collection"}}

// The host request binds raw, independently reviewed input bytes. It is not a
// capability proving production approval, writer fencing, AI closure or CAS.
type historyRequest struct {
	FormatVersion    int            `json:"format_version"`
	Kind             string         `json:"kind"`
	SourceSHA        string         `json:"source_sha"`
	OperationID      string         `json:"operation_id"`
	RunID            string         `json:"run_id"`
	InventoryRequest fileBinding    `json:"inventory_request"`
	InventoryReport  fileBinding    `json:"inventory_report"`
	Assets           []assetBinding `json:"assets"`
}
type fileBinding struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type assetBinding struct {
	Database       string `json:"database"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	FullFileSHA256 string `json:"full_file_sha256"`
	FullFileBytes  uint64 `json:"full_file_bytes"`
}
type inventoryLimits struct {
	QuerySeconds int `json:"query_seconds"`
	TotalSeconds int `json:"total_seconds"`
	MaxRecords   int `json:"max_records"`
	MaxBytes     int `json:"max_bytes"`
	PageSize     int `json:"page_size,omitempty"`
	MaxPages     int `json:"max_pages,omitempty"`
}
type inventoryRequest struct {
	FormatVersion        int                                `json:"format_version"`
	Kind                 string                             `json:"kind"`
	OperationID          string                             `json:"operation_id"`
	SourceSHA            string                             `json:"source_sha"`
	TargetHash           string                             `json:"target_hash"`
	DatabaseScope        string                             `json:"database_scope"`
	Identities           map[string]string                  `json:"identity_hashes"`
	Migrations           map[string]uint64                  `json:"expected_migrations"`
	Limits               inventoryLimits                    `json:"limits"`
	BoundaryRunID        string                             `json:"boundary_run_id,omitempty"`
	BoundaryReportHash   string                             `json:"boundary_report_hash,omitempty"`
	Boundaries           []retirement.SourceBoundary        `json:"approved_boundaries,omitempty"`
	MongoNamespaceAnchor *identitymeta.MongoNamespaceAnchor `json:"mongodb_namespace_anchor,omitempty"`
}
type inventorySnapshot struct {
	Database          string                     `json:"database"`
	Name              string                     `json:"name"`
	Kind              string                     `json:"kind"`
	Present           bool                       `json:"present"`
	Complete          bool                       `json:"complete"`
	Records           uint64                     `json:"records"`
	SchemaHash        string                     `json:"schema_hash"`
	DataHash          string                     `json:"data_hash"`
	IdentityHash      string                     `json:"identity_hash"`
	Bytes             uint64                     `json:"bytes"`
	Classification    map[string]uint64          `json:"classification"`
	SourceFile        string                     `json:"source_file,omitempty"`
	ErrorCategory     string                     `json:"error_category"`
	Boundary          *retirement.SourceBoundary `json:"boundary,omitempty"`
	Passes            int                        `json:"equal_full_passes"`
	Pages             uint64                     `json:"pages"`
	NextCycleRequired bool                       `json:"next_cycle_required"`
}
type databaseInventory struct {
	IdentityHash                 string                             `json:"identity_hash"`
	DatabaseAnchorHash           string                             `json:"database_anchor_hash"`
	NamespaceAnchor              *identitymeta.MongoNamespaceAnchor `json:"namespace_anchor,omitempty"`
	MigrationGenerationHash      string                             `json:"migration_generation_hash"`
	ExpectedIdentityMatch        bool                               `json:"expected_identity_match"`
	Version                      uint64                             `json:"migration_version"`
	Dirty                        bool                               `json:"migration_dirty"`
	ExpectedMigrationMatch       bool                               `json:"expected_migration_match"`
	CatalogHash                  string                             `json:"catalog_hash"`
	NonTargetSchemaHash          string                             `json:"non_target_schema_hash"`
	MetadataComplete             bool                               `json:"metadata_complete"`
	Permissions                  map[string]bool                    `json:"permissions"`
	OutsideDependencies          uint64                             `json:"outside_dependencies"`
	DependencyCoverageComplete   bool                               `json:"dependency_coverage_complete"`
	InboundFKCoverageComplete    bool                               `json:"inbound_foreign_key_coverage_complete"`
	DependencyScope              string                             `json:"dependency_scope"`
	DependencyTextReviewRequired bool                               `json:"dependency_text_review_required"`
	ErrorCategory                string                             `json:"error_category"`
}
type inventoryReport struct {
	FormatVersion        int                          `json:"format_version"`
	Kind                 string                       `json:"kind"`
	SourceSHA            string                       `json:"source_sha"`
	OperationID          string                       `json:"operation_id"`
	RunID                string                       `json:"run_id"`
	RequestHash          string                       `json:"request_hash"`
	TargetHash           string                       `json:"target_hash"`
	ObservedAt           string                       `json:"observed_at"`
	Complete             bool                         `json:"complete"`
	DropReady            bool                         `json:"drop_ready"`
	DatabaseBindings     map[string]databaseInventory `json:"database_bindings"`
	Targets              []inventorySnapshot          `json:"targets"`
	SourceBytesProtocol  string                       `json:"source_bytes_protocol"`
	ConsistencySemantics string                       `json:"consistency_semantics"`
	ErrorCategory        string                       `json:"error_category"`
	BoundaryReportHash   string                       `json:"boundary_report_hash,omitempty"`
	DiagnosticOnly       bool                         `json:"diagnostic_only"`
}
type approvedInputs struct {
	request     historyRequest
	inventory   inventoryRequest
	report      inventoryReport
	requestSHA  string
	requestPath string
	expected    [4]retirement.SourceCopyExpectation
	files       [4]*os.File
}

func fixedError(name string) error { return historyError(name) }
func rawHash(raw []byte) string    { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func jsonHash(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return rawHash(raw)
}

func strictDecode(raw []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var consume func() error
	consume = func() error {
		t, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fixedError("history_json_rejected")
				}
				seen[name] = true
				if err = consume(); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := consume(); err != nil {
					return err
				}
			}
		default:
			return fixedError("history_json_rejected")
		}
		_, err = d.Token()
		return err
	}
	if consume() != nil {
		return fixedError("history_json_rejected")
	}
	if _, err := d.Token(); err != io.EOF {
		return fixedError("history_json_rejected")
	}
	if validateJSONShape(raw, reflect.TypeOf(target)) != nil {
		return fixedError("history_json_rejected")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return fixedError("history_json_rejected")
	}
	return nil
}

func privateParent(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fixedError("history_private_path_rejected")
	}
	parent := filepath.Dir(path)
	for p := parent; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fixedError("history_private_path_rejected")
		}
		if info.Mode().Perm()&022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return fixedError("history_private_path_rejected")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	info, err := os.Lstat(parent)
	if err != nil || info.Mode().Perm() != 0700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return fixedError("history_private_path_rejected")
	}
	return nil
}
func openPrivate(path string, max uint64) (*os.File, error) {
	if privateParent(path) != nil {
		return nil, fixedError("history_private_file_rejected")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, fixedError("history_private_file_rejected")
	}
	st, ok := before.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Getuid()) || st.Nlink != 1 || before.Mode().Perm() != 0600 || before.Size() < 0 || uint64(before.Size()) > max {
		return nil, fixedError("history_private_file_rejected")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fixedError("history_private_file_rejected")
	}
	f := os.NewFile(uintptr(fd), path)
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || after.Mode() != before.Mode() || after.Size() != before.Size() {
		_ = f.Close()
		return nil, fixedError("history_private_file_changed")
	}
	return f, nil
}
func privateJSON(binding fileBinding, target any) (result error) {
	if !hashPattern.MatchString(binding.SHA256) {
		return fixedError("history_hash_rejected")
	}
	f, err := openPrivate(binding.Path, maximumJSONBytes)
	if err != nil {
		return err
	}
	defer func() {
		if e := f.Close(); result == nil && e != nil {
			result = fixedError("history_private_close_failed")
		}
	}()
	raw, err := io.ReadAll(io.LimitReader(f, maximumJSONBytes+1))
	if err != nil || len(raw) > maximumJSONBytes || rawHash(raw) != binding.SHA256 {
		return fixedError("history_hash_rejected")
	}
	return strictDecode(raw, target)
}
func (a *approvedInputs) close() error {
	var result error
	for i, f := range a.files {
		if f != nil {
			if f.Close() != nil {
				result = fixedError("history_private_close_failed")
			}
			a.files[i] = nil
		}
	}
	return result
}
func (a *approvedInputs) rewind() error {
	for _, f := range a.files {
		if f == nil {
			return fixedError("history_asset_missing")
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return fixedError("history_asset_read_failed")
		}
	}
	return nil
}
func (a *approvedInputs) readers() []io.Reader {
	out := make([]io.Reader, 4)
	for i, f := range a.files {
		out[i] = io.NewSectionReader(f, 0, 1<<63-1)
	}
	return out
}
func (a *approvedInputs) copies() []retirement.SourceCopyInput {
	out := make([]retirement.SourceCopyInput, 4)
	for i, f := range a.files {
		out[i] = retirement.SourceCopyInput{Input: f, Expected: a.expected[i]}
	}
	return out
}

// Independent cursors preserve the coordinator's own exactly-once stream.
// The borrowed files remain owned by the CLI; ReaderAt reaches actual EOF.
func (a *approvedInputs) originCopies() []retirement.SourceCopyInput {
	out := make([]retirement.SourceCopyInput, 4)
	for i, f := range a.files {
		out[i] = retirement.SourceCopyInput{Input: io.NewSectionReader(f, 0, 1<<63-1), Expected: a.expected[i]}
	}
	return out
}
func (a *approvedInputs) jointCopies() []retirement.WholeSourceJointCopy {
	out := make([]retirement.WholeSourceJointCopy, 4)
	for i, f := range a.files {
		out[i] = retirement.WholeSourceJointCopy{Input: f, Expected: a.expected[i]}
	}
	return out
}
func (a *approvedInputs) verifyFullFiles(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return fixedError("history_asset_read_failed")
	}
	var req historyRequest
	var inv inventoryRequest
	var report inventoryReport
	if privateJSON(fileBinding{a.requestPath, a.requestSHA}, &req) != nil || privateJSON(a.request.InventoryRequest, &inv) != nil || privateJSON(a.request.InventoryReport, &report) != nil || !reflect.DeepEqual(req, a.request) || !reflect.DeepEqual(inv, a.inventory) || !reflect.DeepEqual(report, a.report) {
		return fixedError("history_approved_metadata_changed")
	}
	for i, f := range a.files {
		if f == nil {
			return fixedError("history_asset_missing")
		}
		before, err := f.Stat()
		live, e := os.Lstat(a.request.Assets[i].Path)
		if err != nil || e != nil || !os.SameFile(before, live) || !live.Mode().IsRegular() || live.Mode().Perm() != 0600 {
			return fixedError("history_asset_read_failed")
		}
		st, ok := before.Sys().(*syscall.Stat_t)
		if !ok || st.Nlink != 1 || st.Uid != uint32(os.Getuid()) {
			return fixedError("history_asset_read_failed")
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return fixedError("history_asset_read_failed")
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(f, maximumEncodedSourceBytes+1))
		after, e := f.Stat()
		asset := a.request.Assets[i]
		if err != nil || e != nil || ctx.Err() != nil || uint64(n) != asset.FullFileBytes || uint64(n) > maximumEncodedSourceBytes || hex.EncodeToString(h.Sum(nil)) != asset.FullFileSHA256 || !os.SameFile(before, after) || before.Size() != after.Size() || before.ModTime() != after.ModTime() {
			return fixedError("history_asset_hash_changed")
		}
	}
	return a.rewind()
}
func loadInputs(ctx context.Context, path, expected, op, run string) (a *approvedInputs, result error) {
	if !sourcePattern.MatchString(sourceSHA) || !hashPattern.MatchString(expected) || !runPattern.MatchString(op) || !runPattern.MatchString(run) {
		return nil, fixedError("history_build_or_request_binding_rejected")
	}
	a = &approvedInputs{requestSHA: expected, requestPath: path}
	defer func() {
		if result != nil {
			_ = a.close()
			a = nil
		}
	}()
	if e := privateJSON(fileBinding{path, expected}, &a.request); e != nil {
		return a, e
	}
	r := a.request
	if r.FormatVersion != 1 || r.Kind != "readonly_compatibility_history_request" || r.SourceSHA != sourceSHA || r.OperationID != op || r.RunID != run || len(r.Assets) != 4 {
		return a, fixedError("history_request_binding_rejected")
	}
	if privateJSON(r.InventoryRequest, &a.inventory) != nil || privateJSON(r.InventoryReport, &a.report) != nil {
		return a, fixedError("history_inventory_input_rejected")
	}
	inv, report := a.inventory, a.report
	fixedLimits := inventoryLimits{30, 1500, 1_000_000, 2 << 30, 1000, 1001}
	if inv.FormatVersion != 2 || inv.Kind != "readonly_inventory_request" || inv.OperationID != op || inv.SourceSHA != sourceSHA || inv.TargetHash != jsonHash(historyTargets) || inv.DatabaseScope != "mysql-and-mongodb" || inv.Limits != fixedLimits || len(inv.Boundaries) != 4 || len(inv.Identities) != 2 || len(inv.Migrations) != 2 || !runPattern.MatchString(inv.BoundaryRunID) || !hashPattern.MatchString(inv.BoundaryReportHash) {
		return a, fixedError("history_inventory_request_rejected")
	}
	if report.FormatVersion != 2 || report.Kind != "readonly_compatibility_inventory" || report.SourceSHA != sourceSHA || report.OperationID != op || !runPattern.MatchString(report.RunID) || report.RequestHash != r.InventoryRequest.SHA256 || report.TargetHash != inv.TargetHash || !report.Complete || report.DropReady || !report.DiagnosticOnly || report.ErrorCategory != "none" || report.SourceBytesProtocol != "mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2" || report.ConsistencySemantics != "two_equal_complete_passes_within_independently_approved_upper;sql_same_readonly_snapshot;mongo_homogeneous_bson_id_simple_collation;after_upper_next_cycle_not_fenced" || report.BoundaryReportHash != inv.BoundaryReportHash || len(report.Targets) != 4 || len(report.DatabaseBindings) != 2 {
		return a, fixedError("history_inventory_report_rejected")
	}
	for _, db := range []string{"mysql", "mongodb"} {
		b, ok := report.DatabaseBindings[db]
		if !ok || !hashPattern.MatchString(inv.Identities[db]) || inv.Migrations[db] == 0 || b.IdentityHash != inv.Identities[db] || b.Version != inv.Migrations[db] || b.Dirty || !b.MetadataComplete || !b.ExpectedIdentityMatch || !b.ExpectedMigrationMatch || b.ErrorCategory != "none" || !hashPattern.MatchString(b.CatalogHash) || !hashPattern.MatchString(b.NonTargetSchemaHash) {
			return a, fixedError("history_database_binding_rejected")
		}
		if db == "mysql" && (b.DatabaseAnchorHash != b.IdentityHash || b.MigrationGenerationHash != "") {
			return a, fixedError("history_database_binding_rejected")
		}
		if db == "mongodb" && (!hashPattern.MatchString(b.DatabaseAnchorHash) || !hashPattern.MatchString(b.MigrationGenerationHash)) {
			return a, fixedError("history_database_binding_rejected")
		}
	}
	if !identitymeta.MatchMongoNamespaceAnchors(inv.MongoNamespaceAnchor, report.DatabaseBindings["mongodb"].NamespaceAnchor) || (inv.MongoNamespaceAnchor != nil && report.DatabaseBindings["mongodb"].DatabaseAnchorHash != inv.MongoNamespaceAnchor.Hash) {
		return a, fixedError("history_database_binding_rejected")
	}
	if report.DatabaseBindings["mysql"].NamespaceAnchor != nil {
		return a, fixedError("history_database_binding_rejected")
	}
	if inv.Identities["mysql"] == inv.Identities["mongodb"] {
		return a, fixedError("history_database_binding_rejected")
	}
	for i, t := range historyTargets {
		s, b, asset := report.Targets[i], inv.Boundaries[i], r.Assets[i]
		if s.Database != t[0] || s.Name != t[1] || s.Kind != t[2] || asset.Database != t[0] || asset.Name != t[1] || !b.Present || !s.Present || !s.Complete || s.ErrorCategory != "none" || s.Passes != 2 || s.NextCycleRequired || s.Records > retirement.MaxSourceRecords || s.Bytes > retirement.MaxSourceBytes || !hashPattern.MatchString(s.DataHash) || s.Boundary == nil || !reflect.DeepEqual(*s.Boundary, b) || s.SchemaHash != b.SchemaHash || s.IdentityHash != b.IdentityHash || !hashPattern.MatchString(b.SchemaHash) || !hashPattern.MatchString(b.IdentityHash) || b.Empty != (s.Records == 0) || (asset.FullFileBytes == 0 && (i != 3 || !b.Empty)) || asset.FullFileBytes > maximumEncodedSourceBytes || !hashPattern.MatchString(asset.FullFileSHA256) || filepath.Base(s.SourceFile) != s.SourceFile || s.SourceFile == "" || asset.Path != filepath.Join(filepath.Dir(r.InventoryReport.Path), s.SourceFile) {
			return a, fixedError("history_source_binding_rejected")
		}
		a.expected[i] = retirement.SourceCopyExpectation{Boundary: b, DataHash: s.DataHash, Records: s.Records, Bytes: s.Bytes}
		f, e := openPrivate(asset.Path, maximumEncodedSourceBytes)
		if e != nil {
			return a, e
		}
		a.files[i] = f
	}
	if e := a.verifyFullFiles(ctx); e != nil {
		return a, e
	}
	return a, nil
}

// encoding/json accepts case aliases. Private approved schemas require their
// exact declared names and all non-optional fields, including false/zero facts.
func validateJSONShape(raw []byte, t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		return validateJSONShape(raw, t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil || object == nil {
			return fixedError("history_json_rejected")
		}
		fields := map[string]reflect.StructField{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")
			name := tag[0]
			if name == "" {
				name = f.Name
			}
			fields[name] = f
			if !strings.Contains(f.Tag.Get("json"), "omitempty") && object[name] == nil {
				return fixedError("history_json_rejected")
			}
		}
		for name, value := range object {
			f, ok := fields[name]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return fixedError("history_json_rejected")
			}
			if validateJSONShape(value, f.Type) != nil {
				return fixedError("history_json_rejected")
			}
		}
	case reflect.Map:
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil || object == nil {
			return fixedError("history_json_rejected")
		}
		for _, v := range object {
			if validateJSONShape(v, t.Elem()) != nil {
				return fixedError("history_json_rejected")
			}
		}
	case reflect.Array, reflect.Slice:
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return fixedError("history_json_rejected")
		}
		for _, v := range values {
			if validateJSONShape(v, t.Elem()) != nil {
				return fixedError("history_json_rejected")
			}
		}
	}
	return nil
}
