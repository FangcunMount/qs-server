package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type scanLimits struct {
	QuerySeconds int `json:"query_seconds"`
	TotalSeconds int `json:"total_seconds"`
	MaxRecords   int `json:"max_records"`
	MaxBytes     int `json:"max_bytes"`
	PageSize     int `json:"page_size,omitempty"`
	MaxPages     int `json:"max_pages,omitempty"`
}

var errSourceFileBound = errors.New("target_output_byte_bound_exceeded")

type boundedSourceWriter struct {
	dst            io.Writer
	written, limit uint64
}

func (w *boundedSourceWriter) Write(raw []byte) (int, error) {
	if w.written > w.limit || uint64(len(raw)) > w.limit-w.written {
		return 0, errSourceFileBound
	}
	n, e := w.dst.Write(raw)
	w.written += uint64(n)
	return n, e
}
func sourceWriter(f *os.File, limit int) (io.Writer, error) {
	offset, e := f.Seek(0, io.SeekCurrent)
	if e != nil || offset < 0 || uint64(offset) > uint64(limit) {
		return nil, category("target_output_byte_bound_exceeded")
	}
	return &boundedSourceWriter{dst: f, written: uint64(offset), limit: uint64(limit)}, nil
}
func outputError(e error) error {
	if errors.Is(e, errSourceFileBound) {
		return category("target_output_byte_bound_exceeded")
	}
	return category("private_output_failed")
}

func productionLimits() scanLimits {
	return scanLimits{QuerySeconds: 30, TotalSeconds: 1500, MaxRecords: 1000000, MaxBytes: 2 << 30, PageSize: 1000, MaxPages: 1001}
}
func legacyLimits() scanLimits {
	return scanLimits{QuerySeconds: querySeconds, TotalSeconds: totalSeconds, MaxRecords: maxRecords, MaxBytes: maxBytes}
}
func scopedQuery(ctx context.Context, limits scanLimits) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(limits.QuerySeconds)*time.Second)
}

// Tokens are private source identity, never public receipts. Mongo retains the
// exact one-field BSON document including its original scalar BSON type.
type targetBoundary struct {
	Database     string `json:"database"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Present      bool   `json:"present"`
	Empty        bool   `json:"empty"`
	PKType       string `json:"pk_type"`
	UpperToken   string `json:"upper_token"`
	SchemaHash   string `json:"schema_hash"`
	IdentityHash string `json:"identity_hash"`
}

func boundaryMode(r request) bool { return r.Kind == "readonly_inventory_boundary_request" }
func productionRequestClass(r request, mode string) error {
	if r.FormatVersion != 2 || ((mode == "bounds") != boundaryMode(r)) {
		return category("request_class_or_version_rejected")
	}
	return nil
}
func boundaryFor(r request, database, name string) (targetBoundary, error) {
	for _, b := range r.Boundaries {
		if b.Database == database && b.Name == name {
			return b, nil
		}
	}
	return targetBoundary{}, category("approved_boundary_missing")
}
func privateJSON(path string, expected string, dst any) error {
	if privateDir(filepath.Dir(path)) != nil {
		return category("boundary_private_invalid")
	}
	st, e := os.Lstat(path)
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() < 1 || st.Size() > 256*1024 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || st.Sys().(*syscall.Stat_t).Nlink != 1 {
		return category("boundary_private_invalid")
	}
	raw, e := os.ReadFile(path)
	if e != nil || digestRaw(raw) != expected {
		return category("boundary_report_hash_mismatch")
	}
	if rejectDuplicateJSON(raw) != nil {
		return category("boundary_schema_invalid")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil || d.Decode(new(any)) != io.EOF {
		return category("boundary_schema_invalid")
	}
	return nil
}
func validateV2Request(r request, path string) error {
	if r.FormatVersion != 2 || r.Limits != productionLimits() {
		return category("request_limits_invalid")
	}
	if boundaryMode(r) {
		if filepath.Base(path) != "boundary-request.json" || r.BoundaryRunID != "" || r.BoundaryReportHash != "" || len(r.Boundaries) != 0 {
			return category("request_class_invalid")
		}
		return nil
	}
	if r.Kind != "readonly_inventory_request" || filepath.Base(path) != "inventory-request.json" || !runRE.MatchString(r.BoundaryRunID) || !hashRE.MatchString(r.BoundaryReportHash) || len(r.Boundaries) != 4 {
		return category("request_class_invalid")
	}
	var observed report
	if e := privateJSON(filepath.Join(filepath.Dir(path), "bounds-"+r.BoundaryRunID, "boundary.private.json"), r.BoundaryReportHash, &observed); e != nil {
		return e
	}
	if observed.FormatVersion != 2 || observed.Kind != "readonly_inventory_boundaries" || !observed.Complete || observed.DropReady || !observed.DiagnosticOnly || observed.SourceSHA != r.SourceSHA || observed.OperationID != r.OperationID || observed.RunID != r.BoundaryRunID || observed.TargetHash != r.TargetHash || len(observed.Targets) != 4 {
		return category("boundary_report_binding_invalid")
	}
	for _, db := range []string{"mysql", "mongodb"} {
		d, ok := observed.DatabaseBindings[db]
		if !ok || d.IdentityHash != r.Identities[db] || d.Version != r.Migrations[db] || d.Dirty || !d.MetadataComplete || !d.ExpectedIdentityMatch || !d.ExpectedMigrationMatch {
			return category("boundary_report_binding_invalid")
		}
	}
	for i, t := range targets {
		s := observed.Targets[i]
		b := r.Boundaries[i]
		if s.Boundary == nil || !s.Complete || s.ErrorCategory != "none" || digest(*s.Boundary) != digest(b) || b.Database != t[0] || b.Name != t[1] || b.Kind != t[2] || !hashRE.MatchString(b.SchemaHash) || !hashRE.MatchString(b.IdentityHash) {
			return category("approved_boundary_mismatch")
		}
		if e := validateBoundary(b); e != nil {
			return e
		}
	}
	return nil
}
func validateBoundary(b targetBoundary) error {
	if !b.Present || b.Empty {
		if b.UpperToken != "" || (!b.Present && b.Empty) {
			return category("boundary_token_invalid")
		}
		return nil
	}
	raw, e := base64.StdEncoding.Strict().DecodeString(b.UpperToken)
	if e != nil || len(raw) == 0 || len(raw) > 1024 || base64.StdEncoding.EncodeToString(raw) != b.UpperToken {
		return category("boundary_token_invalid")
	}
	if b.Database == "mongodb" {
		_, kind, e := decodeMongoToken(raw)
		if e != nil || kind != b.PKType {
			return category("boundary_token_invalid")
		}
		return nil
	}
	switch b.PKType {
	case "uint64":
		_, e = strconv.ParseUint(string(raw), 10, 64)
	case "int64":
		_, e = strconv.ParseInt(string(raw), 10, 64)
	case "ascii_string":
		if !validSQLString(raw) {
			e = errors.New("invalid")
		}
	default:
		e = errors.New("invalid")
	}
	if e != nil {
		return category("boundary_token_invalid")
	}
	return nil
}
func validSQLString(raw []byte) bool {
	if len(raw) == 0 || len(raw) > 128 || !utf8.Valid(raw) {
		return false
	}
	for _, b := range raw {
		if b < 33 || b > 126 {
			return false
		}
	}
	return true
}
func sqlPK(ctx context.Context, tx *sql.Tx, name string) (string, string, error) {
	cols, e := scanSQL(ctx, tx, "SELECT c.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE,c.CHARACTER_SET_NAME,c.COLLATION_NAME FROM information_schema.statistics s JOIN information_schema.columns c ON c.TABLE_SCHEMA=s.TABLE_SCHEMA AND c.TABLE_NAME=s.TABLE_NAME AND c.COLUMN_NAME=s.COLUMN_NAME WHERE s.TABLE_SCHEMA=DATABASE() AND s.TABLE_NAME=? AND s.INDEX_NAME='PRIMARY' ORDER BY s.SEQ_IN_INDEX", name)
	expected := "command_id"
	if name == "domain_event_outbox" {
		expected = "id"
	}
	if e != nil || len(cols) != 1 || val(cols[0], 0) != expected {
		return "", "", category("target_primary_key_rejected")
	}
	kind := ""
	if expected == "id" && val(cols[0], 1) == "bigint" {
		kind = "int64"
		if strings.Contains(val(cols[0], 2), "unsigned") {
			kind = "uint64"
		}
	} else if expected == "command_id" && (val(cols[0], 1) == "char" || val(cols[0], 1) == "varchar") {
		// Byte-ordered UUID identity; alternate collations must not silently alter
		// continuation ordering or compare numeric keys as strings.
		charset, collation := val(cols[0], 3), val(cols[0], 4)
		if charset == "ascii" && collation == "ascii_bin" {
			kind = "ascii_string"
		}
	}
	if kind == "" {
		return "", "", category("target_primary_key_type_unsupported")
	}
	return expected, kind, nil
}
func sqlUpper(ctx context.Context, tx *sql.Tx, name, pk, kind string, limits scanLimits) (string, bool, error) {
	q, cancel := scopedQuery(ctx, limits)
	defer cancel()
	var raw []byte
	e := tx.QueryRowContext(q, "SELECT CAST("+quote(pk)+" AS BINARY) FROM "+quote(name)+" ORDER BY "+quote(pk)+" DESC LIMIT 1").Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return "", true, nil
	}
	if e != nil {
		return "", false, category("target_boundary_read_failed")
	}
	b := targetBoundary{Database: "mysql", Present: true, PKType: kind, UpperToken: base64.StdEncoding.EncodeToString(raw)}
	if e = validateBoundary(b); e != nil {
		return "", false, e
	}
	return b.UpperToken, false, nil
}
func sqlToken(token, kind string) (any, error) {
	raw, e := base64.StdEncoding.Strict().DecodeString(token)
	if e != nil {
		return nil, category("boundary_token_invalid")
	}
	switch kind {
	case "uint64":
		v, e := strconv.ParseUint(string(raw), 10, 64)
		return v, e
	case "int64":
		v, e := strconv.ParseInt(string(raw), 10, 64)
		return v, e
	case "ascii_string":
		if validSQLString(raw) {
			return string(raw), nil
		}
	}
	return nil, category("boundary_token_invalid")
}
func mysqlTargetV2(ctx context.Context, tx *sql.Tx, name, kind string, defs map[string]any, dir string, r request) (snapshot, error) {
	s := snapshot{Database: "mysql", Name: name, Kind: "base_table", Present: kind != "", Classification: map[string]uint64{}, ErrorCategory: "none"}
	b := targetBoundary{Database: s.Database, Name: name, Kind: s.Kind, Present: s.Present}
	if !s.Present {
		b.SchemaHash = digest(nil)
		b.IdentityHash = hashParts("mysql-absent", name)
		s.SchemaHash = b.SchemaHash
		s.IdentityHash = b.IdentityHash
		s.DataHash = digest(nil)
		s.Boundary = &b
		if !boundaryMode(r) {
			approved, e := boundaryFor(r, s.Database, name)
			if e != nil || digest(approved) != digest(b) {
				return s, category("approved_boundary_mismatch")
			}
		}
		s.Complete = true
		if !boundaryMode(r) {
			s.Passes = 2
		}
		return s, nil
	}
	if kind != "BASE TABLE" {
		return s, category("target_type_rejected")
	}
	s.SchemaHash = digest(defs["table:"+name])
	s.IdentityHash = hashParts("mysql-object-v1", name, s.SchemaHash)
	b.SchemaHash = s.SchemaHash
	b.IdentityHash = s.IdentityHash
	pk, pkKind, e := sqlPK(ctx, tx, name)
	if e != nil {
		return s, e
	}
	b.PKType = pkKind
	b.UpperToken, b.Empty, e = sqlUpper(ctx, tx, name, pk, pkKind, r.Limits)
	if e != nil {
		return s, e
	}
	if boundaryMode(r) {
		s.Boundary = &b
		s.Complete = true
		return s, nil
	}
	approved, e := boundaryFor(r, s.Database, name)
	if e != nil || !approved.Present || approved.SchemaHash != b.SchemaHash || approved.IdentityHash != b.IdentityHash || approved.PKType != pkKind {
		return s, category("approved_boundary_mismatch")
	}
	s.Boundary = &approved
	if approved.Empty {
		s.NextCycleRequired = !b.Empty
	} else {
		upper, e := sqlToken(approved.UpperToken, pkKind)
		if e != nil {
			return s, category("boundary_token_invalid")
		}
		q, cancel := scopedQuery(ctx, r.Limits)
		var above bool
		e = tx.QueryRowContext(q, "SELECT EXISTS(SELECT 1 FROM "+quote(name)+" FORCE INDEX (PRIMARY) WHERE "+quote(pk)+" > ? LIMIT 1)", upper).Scan(&above)
		cancel()
		if e != nil {
			return s, category("target_boundary_read_failed")
		}
		s.NextCycleRequired = above
	}
	columns, e := scanSQL(ctx, tx, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION", name)
	if e != nil || len(columns) == 0 {
		return s, category("target_columns_unavailable")
	}
	filename := "mysql-" + name + ".source.ndjson"
	if e = registerSource(ctx, dir, filename, "mysql_cast_binary_columns_pk_order_v2", approved); e != nil {
		return s, e
	}
	f, e := os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return s, category("private_output_exists_or_unavailable")
	}
	defer func() { _ = f.Close() }()
	if json.NewEncoder(f).Encode(map[string]any{"protocol": "mysql_cast_binary_columns_pk_order_v2", "columns": columns, "boundary": approved}) != nil {
		return s, category("private_output_failed")
	}
	first, e := sqlPagedPass(ctx, tx, name, pk, columns, approved, r.Limits, dir, 1, f)
	if e != nil {
		return s, e
	}
	if f.Sync() != nil || f.Close() != nil {
		return s, category("private_output_failed")
	}
	second, e := sqlPagedPass(ctx, tx, name, pk, columns, approved, r.Limits, dir, 2, nil)
	if e != nil {
		return s, e
	}
	if first.Records != second.Records || first.Bytes != second.Bytes || first.DataHash != second.DataHash {
		return s, category("mysql_source_changed_during_scan")
	}
	s.Records = first.Records
	s.Bytes = first.Bytes
	s.DataHash = first.DataHash
	s.Classification = first.Classification
	s.SourceFile = filename
	s.Pages = first.Pages + second.Pages
	s.Passes = 2
	s.Complete = true
	return s, nil
}
func mysqlObserveAfterUpper(ctx context.Context, db *sql.DB, r request, catalogHash string, targets []snapshot) error {
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelReadCommitted})
	if e != nil {
		return category("mysql_readonly_transaction_failed")
	}
	defer func() { _ = tx.Rollback() }()
	ids, e := scanSQL(ctx, tx, "SELECT @@server_uuid,DATABASE(),VERSION()")
	if e != nil || len(ids) != 1 || !strings.HasPrefix(val(ids[0], 2), "8.") || hashParts("mysql_database_identity_v1", val(ids[0], 0), val(ids[0], 1)) != r.Identities["mysql"] {
		return category("database_identity_mismatch")
	}
	heads, e := scanSQL(ctx, tx, "SELECT version,dirty FROM schema_migrations")
	if e != nil || len(heads) != 1 || val(heads[0], 0) != strconv.FormatUint(r.Migrations["mysql"], 10) || val(heads[0], 1) != "0" {
		return category("mysql_migration_head_changed_during_scan")
	}
	_, defs, e := mysqlCatalog(ctx, tx)
	if e != nil || digest(defs) != catalogHash {
		return category("mysql_schema_changed_during_scan")
	}
	for i := range targets {
		s := &targets[i]
		if !s.Present {
			continue
		}
		b := s.Boundary
		if b == nil {
			return category("approved_boundary_missing")
		}
		pk, kind, e := sqlPK(ctx, tx, s.Name)
		if e != nil || kind != b.PKType {
			return category("approved_boundary_mismatch")
		}
		q, cancel := scopedQuery(ctx, r.Limits)
		query := "SELECT EXISTS(SELECT 1 FROM " + quote(s.Name) + " FORCE INDEX (PRIMARY)"
		var args []any
		if !b.Empty {
			upper, e := sqlToken(b.UpperToken, b.PKType)
			if e != nil {
				cancel()
				return category("boundary_token_invalid")
			}
			query += " WHERE " + quote(pk) + " > ?"
			args = append(args, upper)
		}
		query += " LIMIT 1)"
		e = tx.QueryRowContext(q, query, args...).Scan(&s.NextCycleRequired)
		cancel()
		if e != nil {
			return category("target_boundary_read_failed")
		}
	}
	return nil
}

type scanRunKey struct{}
type scanRunIdentity struct{ OperationID, RunID, RequestHash string }

func registerSource(ctx context.Context, dir, filename, protocol string, b targetBoundary) error {
	run, ok := ctx.Value(scanRunKey{}).(scanRunIdentity)
	if !ok || !runRE.MatchString(run.OperationID) || !runRE.MatchString(run.RunID) || !hashRE.MatchString(run.RequestHash) {
		return category("source_asset_binding_missing")
	}
	return writeJSON(filepath.Join(dir, filename+".asset.json"), map[string]any{"format_version": 1, "kind": "temporary_inventory_source_copy", "filename": filename, "source_sha": sourceSHA, "operation_id": run.OperationID, "run_id": run.RunID, "request_hash": run.RequestHash, "protocol": protocol, "boundary": b, "contains_original_body": true, "retirement_proof": false, "purge_required_after_acceptance": true, "resume_existing_file_allowed": false})
}
func checkpoint(dir, name string, pass, page int, token string, count, size uint64, h hash.Hash) error {
	return writeJSON(filepath.Join(dir, fmt.Sprintf("%s-pass-%d-page-%06d.checkpoint.json", name, pass, page)), map[string]any{"format_version": 1, "kind": "readonly_inventory_page_checkpoint", "source_sha": sourceSHA, "pass": pass, "page": page, "cursor_token": token, "records": count, "source_bytes": size, "prefix_hash": hex.EncodeToString(h.Sum(nil)), "diagnostic_only": true, "resume_existing_file_allowed": false})
}
func sqlPagedPass(ctx context.Context, tx *sql.Tx, name, pk string, columns [][]*string, b targetBoundary, limits scanLimits, dir string, pass int, f *os.File) (snapshot, error) {
	s := snapshot{Classification: map[string]uint64{}}
	h := sha256.New()
	var sink io.Writer
	if f != nil {
		var e error
		sink, e = sourceWriter(f, limits.MaxBytes)
		if e != nil {
			return s, e
		}
	}
	frame(h, []byte(digest(columns)), false)
	names := make([]string, len(columns))
	expr := make([]string, len(columns))
	pkIndex := -1
	for i, c := range columns {
		names[i] = val(c, 0)
		expr[i] = "CAST(" + quote(names[i]) + " AS BINARY)"
		if names[i] == pk {
			pkIndex = i
		}
	}
	if pkIndex < 0 {
		return s, category("target_primary_key_rejected")
	}
	if b.Empty {
		s.DataHash = hex.EncodeToString(h.Sum(nil))
		return s, nil
	}
	upper, e := sqlToken(b.UpperToken, b.PKType)
	if e != nil {
		return s, category("boundary_token_invalid")
	}
	cursor := ""
	for page := 1; ; page++ {
		if page > limits.MaxPages {
			return s, category("target_page_bound_exceeded")
		}
		where := quote(pk) + " <= ?"
		args := []any{upper}
		if cursor != "" {
			last, e := sqlToken(cursor, b.PKType)
			if e != nil {
				return s, category("boundary_token_invalid")
			}
			where += " AND " + quote(pk) + " > ?"
			args = append(args, last)
		}
		args = append(args, limits.PageSize)
		q, cancel := scopedQuery(ctx, limits)
		rows, e := tx.QueryContext(q, "SELECT "+strings.Join(expr, ",")+" FROM "+quote(name)+" FORCE INDEX (PRIMARY) WHERE "+where+" ORDER BY "+quote(pk)+" LIMIT ?", args...)
		if e != nil {
			cancel()
			return s, category("target_read_failed_or_timed_out")
		}
		n := 0
		for rows.Next() {
			raw := make([]sql.RawBytes, len(columns))
			dest := make([]any, len(raw))
			for i := range raw {
				dest[i] = &raw[i]
			}
			if rows.Scan(dest...) != nil {
				_ = rows.Close()
				cancel()
				return s, category("target_read_failed_or_timed_out")
			}
			if s.Records >= uint64(limits.MaxRecords) {
				_ = rows.Close()
				cancel()
				return s, category("target_record_bound_exceeded")
			}
			encoded := make([]*string, len(raw))
			for i, v := range raw {
				s.Bytes += uint64(len(v))
				if s.Bytes > uint64(limits.MaxBytes) {
					_ = rows.Close()
					cancel()
					return s, category("target_byte_bound_exceeded")
				}
				frame(h, v, v == nil)
				if v != nil {
					v64 := base64.StdEncoding.EncodeToString(v)
					encoded[i] = &v64
				}
			}
			next := base64.StdEncoding.EncodeToString(raw[pkIndex])
			if next == cursor {
				_ = rows.Close()
				cancel()
				return s, category("cursor_did_not_advance")
			}
			cursor = next
			if f != nil {
				if e := json.NewEncoder(sink).Encode(encoded); e != nil {
					_ = rows.Close()
					cancel()
					return s, outputError(e)
				}
			}
			s.Records++
			n++
			classifySQL(s.Classification, name, names, raw)
		}
		readErr, closeErr := rows.Err(), rows.Close()
		cancel()
		if readErr != nil || closeErr != nil {
			return s, category("target_read_failed_or_timed_out")
		}
		if f != nil && f.Sync() != nil {
			return s, category("private_output_failed")
		}
		s.Pages++
		if e = checkpoint(dir, "mysql-"+name, pass, page, cursor, s.Records, s.Bytes, h); e != nil {
			return s, e
		}
		if n < limits.PageSize {
			break
		}
	}
	s.DataHash = hex.EncodeToString(h.Sum(nil))
	return s, nil
}

func decodeMongoToken(raw []byte) (any, string, error) {
	doc := bson.Raw(raw)
	if doc.Validate() != nil {
		return nil, "", category("mongo_cursor_invalid")
	}
	es, e := doc.Elements()
	if e != nil || len(es) != 1 || es[0].Key() != "_id" {
		return nil, "", category("mongo_cursor_invalid")
	}
	v := es[0].Value()
	kind := ""
	switch v.Type {
	case bson.TypeString:
		if v.StringValue() == "" {
			return nil, "", category("mongo_cursor_invalid")
		}
		kind = "string"
	case bson.TypeObjectID:
		kind = "objectId"
	case bson.TypeInt32:
		kind = "int"
	case bson.TypeInt64:
		kind = "long"
	default:
		return nil, "", category("mongo_id_type_unsupported")
	}
	var value any
	if v.Unmarshal(&value) != nil {
		return nil, "", category("mongo_cursor_invalid")
	}
	return value, kind, nil
}
func mongoToken(raw bson.Raw) (string, string, error) {
	v := raw.Lookup("_id")
	if v.Type == 0 {
		return "", "", category("mongo_cursor_invalid")
	}
	doc, e := bson.Marshal(bson.D{{Key: "_id", Value: v}})
	if e != nil {
		return "", "", category("mongo_cursor_invalid")
	}
	_, kind, e := decodeMongoToken(doc)
	if e != nil {
		return "", "", e
	}
	return base64.StdEncoding.EncodeToString(doc), kind, nil
}
func mongoIDType(ctx context.Context, col *mongo.Collection, limits scanLimits) (string, error) {
	q, cancel := scopedQuery(ctx, limits)
	defer cancel()
	// Query operators type-bracket comparisons. Before using an indexed cursor,
	// prove the entire collection has exactly one supported scalar _id type.
	cur, e := col.Aggregate(q, bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: "$_id"}}}}}}, bson.D{{Key: "$limit", Value: 2}}}, options.Aggregate().SetCollation(&options.Collation{Locale: "simple"}).SetAllowDiskUse(false).SetMaxTime(time.Duration(limits.QuerySeconds)*time.Second))
	if e != nil {
		return "", category("mongo_id_type_proof_failed")
	}
	var types []struct {
		Type string `bson:"_id"`
	}
	e = cur.All(q, &types)
	closeErr := cur.Close(q)
	if e != nil || closeErr != nil {
		return "", category("mongo_id_type_proof_failed")
	}
	if len(types) == 0 {
		return "", nil
	}
	if len(types) != 1 {
		return "", category("mongo_mixed_id_types_unsupported")
	}
	switch types[0].Type {
	case "string", "objectId", "int", "long":
		return types[0].Type, nil
	}
	return "", category("mongo_id_type_unsupported")
}
func mongoUpper(ctx context.Context, col *mongo.Collection, limits scanLimits) (string, string, bool, error) {
	kind, e := mongoIDType(ctx, col, limits)
	if e != nil {
		return "", "", false, e
	}
	q, cancel := scopedQuery(ctx, limits)
	defer cancel()
	var row bson.Raw
	e = col.FindOne(q, bson.D{}, options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}}).SetHint("_id_").SetCollation(&options.Collation{Locale: "simple"}).SetProjection(bson.D{{Key: "_id", Value: 1}}).SetMaxTime(time.Duration(limits.QuerySeconds)*time.Second)).Decode(&row)
	if errors.Is(e, mongo.ErrNoDocuments) {
		if kind != "" {
			return "", "", false, category("mongo_source_changed_during_bounds")
		}
		return "", "", true, nil
	}
	if e != nil {
		return "", "", false, category("target_boundary_read_failed")
	}
	token, actual, e := mongoToken(row)
	if e != nil || actual != kind {
		return "", "", false, category("mongo_source_changed_during_bounds")
	}
	return token, kind, false, nil
}
func mongoPagedPass(ctx context.Context, col *mongo.Collection, b targetBoundary, limits scanLimits, dir string, pass int, f *os.File) (snapshot, error) {
	s := snapshot{Classification: map[string]uint64{}}
	h := sha256.New()
	var sink io.Writer
	if f != nil {
		var e error
		sink, e = sourceWriter(f, limits.MaxBytes)
		if e != nil {
			return s, e
		}
	}
	if b.Empty {
		s.DataHash = hex.EncodeToString(h.Sum(nil))
		return s, nil
	}
	upperRaw, e := base64.StdEncoding.Strict().DecodeString(b.UpperToken)
	if e != nil {
		return s, category("mongo_cursor_invalid")
	}
	upper, kind, e := decodeMongoToken(upperRaw)
	if e != nil || kind != b.PKType {
		return s, category("mongo_cursor_invalid")
	}
	cursor := ""
	for page := 1; ; page++ {
		if page > limits.MaxPages {
			return s, category("target_page_bound_exceeded")
		}
		rangeOps := bson.D{{Key: "$lte", Value: upper}}
		if cursor != "" {
			raw, e := base64.StdEncoding.Strict().DecodeString(cursor)
			if e != nil {
				return s, category("mongo_cursor_invalid")
			}
			last, k, e := decodeMongoToken(raw)
			if e != nil || k != kind {
				return s, category("mongo_cursor_invalid")
			}
			rangeOps = append(rangeOps, bson.E{Key: "$gt", Value: last})
		}
		q, cancel := scopedQuery(ctx, limits)
		cur, e := col.Find(q, bson.D{{Key: "_id", Value: rangeOps}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetHint("_id_").SetCollation(&options.Collation{Locale: "simple"}).SetLimit(int64(limits.PageSize)).SetBatchSize(int32(limits.PageSize)).SetMaxTime(time.Duration(limits.QuerySeconds)*time.Second))
		if e != nil {
			cancel()
			return s, category("mongo_target_read_failed_or_timed_out")
		}
		n := 0
		for cur.Next(q) {
			raw := cur.Current
			if raw.Validate() != nil {
				_ = cur.Close(q)
				cancel()
				return s, category("mongo_target_decode_failed")
			}
			next, k, e := mongoToken(raw)
			if e != nil || k != kind || next == cursor {
				_ = cur.Close(q)
				cancel()
				return s, category("mongo_cursor_invalid")
			}
			if s.Records >= uint64(limits.MaxRecords) {
				_ = cur.Close(q)
				cancel()
				return s, category("target_record_bound_exceeded")
			}
			s.Bytes += uint64(len(raw))
			if s.Bytes > uint64(limits.MaxBytes) {
				_ = cur.Close(q)
				cancel()
				return s, category("target_byte_bound_exceeded")
			}
			frame(h, raw, false)
			if f != nil {
				var length [8]byte
				binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
				if _, e = sink.Write(length[:]); e != nil {
					_ = cur.Close(q)
					cancel()
					return s, outputError(e)
				}
				if _, e = sink.Write(raw); e != nil {
					_ = cur.Close(q)
					cancel()
					return s, outputError(e)
				}
			}
			classifyMongo(s.Classification, raw)
			s.Records++
			n++
			cursor = next
		}
		readErr, closeErr := cur.Err(), cur.Close(q)
		cancel()
		if readErr != nil || closeErr != nil {
			return s, category("mongo_target_read_failed_or_timed_out")
		}
		if f != nil && f.Sync() != nil {
			return s, category("private_output_failed")
		}
		s.Pages++
		if e = checkpoint(dir, "mongodb-domain_event_outbox", pass, page, cursor, s.Records, s.Bytes, h); e != nil {
			return s, e
		}
		if n < limits.PageSize {
			break
		}
	}
	s.DataHash = hex.EncodeToString(h.Sum(nil))
	return s, nil
}
func mongoTargetV2(ctx context.Context, db *mongo.Database, collections map[string]bson.Raw, defs map[string]any, dir string, r request) (snapshot, error) {
	name := "domain_event_outbox"
	s := snapshot{Database: "mongodb", Name: name, Kind: "collection", Classification: map[string]uint64{}, ErrorCategory: "none"}
	raw, present := collections[name]
	s.Present = present
	b := targetBoundary{Database: s.Database, Name: name, Kind: s.Kind, Present: present}
	if !present {
		b.SchemaHash = digest(nil)
		b.IdentityHash = hashParts("mongodb-absent", name)
		s.SchemaHash = b.SchemaHash
		s.IdentityHash = b.IdentityHash
		s.DataHash = digest(nil)
		s.Boundary = &b
		if !boundaryMode(r) {
			approved, e := boundaryFor(r, s.Database, name)
			if e != nil || digest(approved) != digest(b) {
				return s, category("approved_boundary_mismatch")
			}
		}
		s.Complete = true
		if !boundaryMode(r) {
			s.Passes = 2
		}
		return s, nil
	}
	if raw.Lookup("type").StringValue() != "collection" {
		return s, category("target_type_rejected")
	}
	collation := raw.Lookup("options", "collation", "locale")
	if collation.Type != 0 && (collation.Type != bson.TypeString || collation.StringValue() != "simple") {
		return s, category("mongo_target_collation_unsupported")
	}
	uuid := raw.Lookup("info", "uuid")
	if uuid.Type != bson.TypeBinary {
		return s, category("mongo_target_uuid_unavailable")
	}
	subtype, uuidBytes := uuid.Binary()
	if subtype != 4 || len(uuidBytes) != 16 {
		return s, category("mongo_target_uuid_unavailable")
	}
	b.SchemaHash = digest(map[string]any{"collection": defs["collection:"+name], "indexes": defs["indexes:"+name]})
	b.IdentityHash = hashParts("mongodb-object-v1", hex.EncodeToString(uuidBytes))
	s.SchemaHash = b.SchemaHash
	s.IdentityHash = b.IdentityHash
	var e error
	b.UpperToken, b.PKType, b.Empty, e = mongoUpper(ctx, db.Collection(name), r.Limits)
	if e != nil {
		return s, e
	}
	if boundaryMode(r) {
		s.Boundary = &b
		s.Complete = true
		return s, nil
	}
	approved, e := boundaryFor(r, s.Database, name)
	if e != nil || !approved.Present || approved.SchemaHash != b.SchemaHash || approved.IdentityHash != b.IdentityHash || (!approved.Empty && !b.Empty && approved.PKType != b.PKType) {
		return s, category("approved_boundary_mismatch")
	}
	s.Boundary = &approved
	filename := "mongodb-domain_event_outbox.source.bsonframes"
	if e = registerSource(ctx, dir, filename, "mongodb_server_bson_pk_order_v2", approved); e != nil {
		return s, e
	}
	f, e := os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return s, category("private_output_exists_or_unavailable")
	}
	defer func() { _ = f.Close() }()
	first, e := mongoPagedPass(ctx, db.Collection(name), approved, r.Limits, dir, 1, f)
	if e != nil {
		return s, e
	}
	if f.Sync() != nil || f.Close() != nil {
		return s, category("private_output_failed")
	}
	currentType, e := mongoIDType(ctx, db.Collection(name), r.Limits)
	if e != nil {
		return s, e
	}
	if !approved.Empty && currentType != "" && currentType != approved.PKType {
		return s, category("mongo_source_changed_during_scan")
	}
	second, e := mongoPagedPass(ctx, db.Collection(name), approved, r.Limits, dir, 2, nil)
	if e != nil {
		return s, e
	}
	if first.Records != second.Records || first.Bytes != second.Bytes || first.DataHash != second.DataHash {
		return s, category("mongo_source_changed_during_scan")
	}
	_, endType, endEmpty, e := mongoUpper(ctx, db.Collection(name), r.Limits)
	if e != nil {
		return s, e
	}
	if !approved.Empty && !endEmpty && endType != approved.PKType {
		return s, category("mongo_source_changed_during_scan")
	}
	if approved.Empty {
		s.NextCycleRequired = !endEmpty
	} else {
		upperRaw, e := base64.StdEncoding.Strict().DecodeString(approved.UpperToken)
		if e != nil {
			return s, category("mongo_cursor_invalid")
		}
		upper, _, e := decodeMongoToken(upperRaw)
		if e != nil {
			return s, e
		}
		q, cancel := scopedQuery(ctx, r.Limits)
		e = db.Collection(name).FindOne(q, bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: upper}}}}, options.FindOne().SetHint("_id_").SetCollation(&options.Collation{Locale: "simple"}).SetProjection(bson.D{{Key: "_id", Value: 1}})).Err()
		cancel()
		if e != nil && !errors.Is(e, mongo.ErrNoDocuments) {
			return s, category("target_boundary_read_failed")
		}
		s.NextCycleRequired = e == nil
	}
	s.Records = first.Records
	s.Bytes = first.Bytes
	s.DataHash = first.DataHash
	s.Classification = first.Classification
	s.SourceFile = filename
	s.Pages = first.Pages + second.Pages
	s.Passes = 2
	s.Complete = true
	return s, nil
}
func classifyMongo(classes map[string]uint64, raw bson.Raw) {
	value := raw.Lookup("status")
	if value.Type == bson.TypeString {
		switch state := value.StringValue(); state {
		case "pending", "publishing", "failed", "published", "quarantined":
			classes[state]++
			return
		}
	}
	classes["unknown_transport_state"]++
}
