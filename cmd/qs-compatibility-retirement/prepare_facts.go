package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// A facts request authorizes observations, not approval of those observations.
// The original inventory producer and this observer remain separate bindings.
type prepareFactsProducer struct {
	OperationID   string `json:"operation_id"`
	RunID         string `json:"run_id"`
	SourceSHA     string `json:"source_sha"`
	ReportSHA256  string `json:"sha256"`
	RequestSHA256 string `json:"request_sha256"`
}
type prepareFactsRequest struct {
	FormatVersion             int                      `json:"format_version"`
	Kind                      string                   `json:"kind"`
	SourceSHA                 string                   `json:"source_sha"`
	OperationID               string                   `json:"operation_id"`
	ActualRunID               string                   `json:"actual_run_id"`
	TargetHash                string                   `json:"target_hash"`
	DatabaseScope             string                   `json:"database_scope"`
	ObservationApprovalSHA256 string                   `json:"observation_approval_sha256"`
	Inventory                 prepareFactsProducer     `json:"inventory_report"`
	RestoreEngines            *lifecycleRestoreEngines `json:"restore_engines"`
	ArchiveDirectory          string                   `json:"archive_directory"`
}
type prepareFactsFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}
type prepareFactsFS struct {
	Scope          string `json:"scope"`
	PathSHA256     string `json:"path_sha256"`
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
}
type prepareFactsReceipt struct {
	FormatVersion             int                      `json:"format_version"`
	Kind                      string                   `json:"kind"`
	Operation                 string                   `json:"operation"`
	PrepareMode               string                   `json:"prepare_mode"`
	SourceSHA                 string                   `json:"source_sha"`
	OperationID               string                   `json:"operation_id"`
	RunID                     string                   `json:"run_id"`
	RequestSHA256             string                   `json:"request_sha256"`
	ObservationApprovalSHA256 string                   `json:"observation_approval_sha256"`
	TargetHash                string                   `json:"target_hash"`
	Complete                  bool                     `json:"complete"`
	FactsObservationComplete  bool                     `json:"prepare_facts_observation_complete"`
	DiagnosticOnly            bool                     `json:"diagnostic_only"`
	ExecutionAllowed          bool                     `json:"execution_allowed"`
	DropReady                 bool                     `json:"drop_ready"`
	Producer                  prepareFactsProducer     `json:"observed_inventory_producer"`
	Files                     []prepareFactsFile       `json:"prepare_source_files"`
	OrderedMongoSchemaSHA256  string                   `json:"observed_ordered_mongo_schema_sha256"`
	RestoreEngines            *lifecycleRestoreEngines `json:"observed_restore_engines,omitempty"`
	Capacity                  []prepareFactsFS         `json:"observed_filesystems"`
	SocketKind                string                   `json:"observed_socket_kind"`
	PrivateObservationSHA256  string                   `json:"prepare_facts_private_observation_sha256,omitempty"`
	ElapsedMillis             int64                    `json:"observation_elapsed_millis"`
	ErrorCategory             string                   `json:"error_category"`
}

func decodePrepareFacts(raw []byte, dst any) error {
	if rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(dst)) != nil {
		return lifecycleError("prepare_facts_schema_rejected")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil || d.Decode(new(any)) != io.EOF {
		return lifecycleError("prepare_facts_schema_rejected")
	}
	return nil
}
func validatePrepareFactsRequest(r prepareFactsRequest, op, run string) error {
	original := filepath.Join("/opt/backups/qs-server/compatibility-retirement", op)
	if r.FormatVersion != 1 || r.Kind != "readonly_prepare_facts_request" || r.SourceSHA != sourceSHA || !shaRE.MatchString(sourceSHA) || r.OperationID != op || r.ActualRunID != run || !runRE.MatchString(op) || !runRE.MatchString(run) || r.TargetHash != digest(targets) || r.DatabaseScope != "mysql-and-mongodb" || !hashRE.MatchString(r.ObservationApprovalSHA256) || r.Inventory.OperationID != op || !runRE.MatchString(r.Inventory.RunID) || r.Inventory.RunID == run || !shaRE.MatchString(r.Inventory.SourceSHA) || !hashRE.MatchString(r.Inventory.ReportSHA256) || !hashRE.MatchString(r.Inventory.RequestSHA256) || !r.RestoreEngines.valid() || r.RestoreEngines.MySQLImageID == r.RestoreEngines.MongoImageID || !lifecycleOwnedPath(original, r.ArchiveDirectory) || r.ArchiveDirectory == filepath.Join(original, "inventory-"+r.Inventory.RunID) || lifecycleOwnedPath(filepath.Join(original, "inventory-"+r.Inventory.RunID), r.ArchiveDirectory) {
		return lifecycleError("prepare_facts_binding_rejected")
	}
	return nil
}

// Hash bytes in place; no source-body copy or owner change. The caller gets
// small metadata only when requested; source streams never enter a receipt.
func hashPrepareFactsFile(ctx context.Context, path string, uid uint32, maximum int64, keep bool) (fact prepareFactsFile, raw []byte, result error) {
	if ctx == nil || ctx.Err() != nil || maximum < 0 || (keep && maximum > 16<<20) || lifecycleSourcePrivateDirectory(filepath.Dir(path), uid) != nil {
		return fact, nil, lifecycleError("prepare_facts_source_rejected")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return fact, nil, lifecycleError("prepare_facts_source_rejected")
	}
	defer func() {
		if e := f.Close(); result == nil && e != nil {
			result = lifecycleError("prepare_facts_source_close_failed")
		}
	}()
	before, e := f.Stat()
	st, ok := infoStat(before)
	if e != nil || before == nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || !ok || st.Uid != uid || st.Nlink != 1 || before.Size() < 0 || before.Size() > maximum {
		return fact, nil, lifecycleError("prepare_facts_source_rejected")
	}
	h := sha256.New()
	buffer := make([]byte, 64<<10)
	var n int64
	for {
		if ctx.Err() != nil {
			return fact, nil, lifecycleError("prepare_facts_read_budget_exceeded")
		}
		count, readErr := f.Read(buffer)
		if count > 0 {
			n += int64(count)
			if n > maximum {
				return fact, nil, lifecycleError("prepare_facts_source_rejected")
			}
			_, _ = h.Write(buffer[:count])
			if keep {
				raw = append(raw, buffer[:count]...)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fact, nil, lifecycleError("prepare_facts_source_read_failed")
		}
	}
	after, e := f.Stat()
	if e != nil || n != before.Size() || !sameLifecycleFile(before, after) || ctx.Err() != nil {
		return fact, nil, lifecycleError("prepare_facts_source_changed")
	}
	return prepareFactsFile{Name: filepath.Base(path), SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: uint64(n)}, raw, nil
}

func validatePrepareFactsInventory(r prepareFactsRequest, req request, report report) error {
	if req.FormatVersion != 2 || req.Kind != "readonly_inventory_request" || req.OperationID != r.OperationID || req.SourceSHA != r.Inventory.SourceSHA || req.TargetHash != digest(targets) || req.DatabaseScope != "mysql-and-mongodb" || req.Limits != productionLimits() || !runRE.MatchString(req.BoundaryRunID) || !hashRE.MatchString(req.BoundaryReportHash) || len(req.Boundaries) != 4 || report.BoundaryReportHash != req.BoundaryReportHash || len(req.Identities) != 2 || len(req.Migrations) != 2 || req.Migrations["mysql"] != 99 || req.Migrations["mongodb"] != 38 || !hashRE.MatchString(req.Identities["mysql"]) || !hashRE.MatchString(req.Identities["mongodb"]) || req.Identities["mysql"] == req.Identities["mongodb"] || (req.MongoNamespaceAnchor != nil && req.MongoNamespaceAnchor.Validate() != nil) || report.FormatVersion != 2 || report.Kind != "readonly_compatibility_inventory" || report.SourceSHA != req.SourceSHA || report.OperationID != req.OperationID || report.RunID != r.Inventory.RunID || report.RequestHash != r.Inventory.RequestSHA256 || report.TargetHash != digest(targets) || !report.Complete || report.DropReady || report.ErrorCategory != "none" || len(report.DatabaseBindings) != 2 || len(report.Targets) != 4 {
		return lifecycleError("prepare_facts_inventory_binding_rejected")
	}
	for _, db := range []string{"mysql", "mongodb"} {
		v := report.DatabaseBindings[db]
		if v.IdentityHash != req.Identities[db] || v.Version != req.Migrations[db] || v.Dirty || !v.MetadataComplete || !v.ExpectedIdentityMatch || !v.ExpectedMigrationMatch || !hashRE.MatchString(v.CatalogHash) || !hashRE.MatchString(v.DatabaseAnchorHash) {
			return lifecycleError("prepare_facts_inventory_binding_rejected")
		}
	}
	for i, t := range targets {
		v := report.Targets[i]
		if v.Database != t[0] || v.Name != t[1] || v.Kind != t[2] || !v.Present || !v.Complete || v.ErrorCategory != "none" || v.Passes != 2 || v.Boundary == nil || validateBoundary(req.Boundaries[i]) != nil || digest(*v.Boundary) != digest(req.Boundaries[i]) || v.SourceFile != lifecycleSourceNames[i+3] {
			return lifecycleError("prepare_facts_inventory_binding_rejected")
		}
	}
	return nil
}

// Read actual identity and clean heads through the owned, already-open handles.
// This is a source-binding check, not a new inventory/history verdict.
func observePrepareFactsIdentity(ctx context.Context, o *lifecyclePreparationOwner, req request, inventory report) (result error) {
	tx, e := o.originalSQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return lifecycleError("prepare_facts_sql_read_failed")
	}
	defer func() {
		if e := tx.Rollback(); result == nil && e != nil && e != sql.ErrTxDone {
			result = lifecycleError("prepare_facts_sql_close_failed")
		}
	}()
	q, c := context.WithTimeout(ctx, 30*time.Second)
	ids, e := scanSQL(q, tx, "SELECT @@server_uuid,DATABASE(),VERSION()")
	c()
	if e != nil || len(ids) != 1 || !mysqlUUIDRE.MatchString(val(ids[0], 0)) || val(ids[0], 1) != os.Getenv("MYSQL_DATABASE") || !strings.HasPrefix(val(ids[0], 2), "8.") || hashParts("mysql_database_identity_v1", val(ids[0], 0), val(ids[0], 1)) != req.Identities["mysql"] {
		return lifecycleError("prepare_facts_identity_rejected")
	}
	q, c = context.WithTimeout(ctx, 30*time.Second)
	heads, e := scanSQL(q, tx, "SELECT version,dirty FROM schema_migrations LIMIT 2")
	c()
	if e != nil || len(heads) != 1 || val(heads[0], 0) != strconv.FormatUint(req.Migrations["mysql"], 10) || val(heads[0], 1) != "0" {
		return lifecycleError("prepare_facts_migration_rejected")
	}
	db := o.originalDB
	var hello bson.Raw
	q, c = context.WithTimeout(ctx, 30*time.Second)
	e = o.originalMongo.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
	c()
	if e != nil {
		return lifecycleError("prepare_facts_mongo_read_failed")
	}
	q, c = context.WithTimeout(ctx, 30*time.Second)
	cursor, e := db.ListCollections(q, bson.D{{Key: "name", Value: "schema_migrations"}}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		c()
		return lifecycleError("prepare_facts_mongo_read_failed")
	}
	var collections []bson.Raw
	e = cursor.All(q, &collections)
	closeErr := cursor.Close(q)
	c()
	if e != nil || closeErr != nil || len(collections) != 1 {
		return lifecycleError("prepare_facts_mongo_read_failed")
	}
	uuid := collections[0].Lookup("info", "uuid")
	if uuid.Type != bson.TypeBinary {
		return lifecycleError("prepare_facts_identity_rejected")
	}
	subtype, b := uuid.Binary()
	if subtype != 4 || len(b) != 16 {
		return lifecycleError("prepare_facts_identity_rejected")
	}
	stable := bson.D{}
	for _, name := range []string{"setName", "hosts", "me"} {
		v := hello.Lookup(name)
		if v.Type != 0 {
			var decoded any
			if v.Unmarshal(&decoded) != nil {
				return lifecycleError("prepare_facts_identity_rejected")
			}
			stable = append(stable, bson.E{Key: name, Value: decoded})
		}
	}
	canonical, e := json.Marshal(stable)
	if e != nil || hashParts("mongodb_database_identity_v1", string(canonical), os.Getenv("MONGODB_DBNAME"), hex.EncodeToString(b)) != req.Identities["mongodb"] {
		return lifecycleError("prepare_facts_identity_rejected")
	}
	if req.MongoNamespaceAnchor != nil {
		actual, e := mongoNamespaceAnchor(ctx, db)
		if e != nil || !identitymeta.MatchMongoNamespaceAnchors(req.MongoNamespaceAnchor, actual) || actual.Hash != inventory.DatabaseBindings["mongodb"].DatabaseAnchorHash {
			return lifecycleError("prepare_facts_identity_rejected")
		}
	} else {
		actual, e := mongoDatabaseAnchor(ctx, db, hello)
		if e != nil || actual != inventory.DatabaseBindings["mongodb"].DatabaseAnchorHash {
			return lifecycleError("prepare_facts_identity_rejected")
		}
	}
	var build struct {
		Version string `bson:"version"`
	}
	q, c = context.WithTimeout(ctx, 30*time.Second)
	e = o.originalMongo.Database("admin").RunCommand(q, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build)
	c()
	if e != nil || !strings.HasPrefix(build.Version, "7.") {
		return lifecycleError("prepare_facts_identity_rejected")
	}
	q, c = context.WithTimeout(ctx, 30*time.Second)
	cursor, e = db.Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2))
	if e != nil {
		c()
		return lifecycleError("prepare_facts_mongo_read_failed")
	}
	var migration struct {
		Version int64 `bson:"version"`
		Dirty   bool  `bson:"dirty"`
	}
	var rawHeads []bson.Raw
	e = cursor.All(q, &rawHeads)
	closeErr = cursor.Close(q)
	c()
	if e != nil || closeErr != nil || len(rawHeads) != 1 || rawHeads[0].Lookup("dirty").Type != bson.TypeBoolean || (rawHeads[0].Lookup("version").Type != bson.TypeInt32 && rawHeads[0].Lookup("version").Type != bson.TypeInt64) {
		return lifecycleError("prepare_facts_migration_rejected")
	}
	if bson.Unmarshal(rawHeads[0], &migration) != nil || migration.Version < 1 || uint64(migration.Version) != req.Migrations["mongodb"] || migration.Dirty {
		return lifecycleError("prepare_facts_migration_rejected")
	}
	return nil
}

func observePrepareFactsFS(scope, path string) (prepareFactsFS, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return prepareFactsFS{}, lifecycleError("prepare_facts_filesystem_rejected")
	}
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil || resolved != path {
		return prepareFactsFS{}, lifecycleError("prepare_facts_filesystem_rejected")
	}
	for p := path; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || (info.Mode().Perm()&022 != 0 && info.Mode()&os.ModeSticky == 0) {
			return prepareFactsFS{}, lifecycleError("prepare_facts_filesystem_rejected")
		}
		if p == "/" {
			break
		}
	}
	var fs syscall.Statfs_t
	if syscall.Statfs(path, &fs) != nil || fs.Bsize <= 0 {
		return prepareFactsFS{}, lifecycleError("prepare_facts_filesystem_read_failed")
	}
	size := uint64(fs.Bsize)
	if fs.Bavail > fs.Blocks || fs.Bfree > fs.Blocks {
		return prepareFactsFS{}, lifecycleError("prepare_facts_filesystem_rejected")
	}
	if fs.Blocks > ^uint64(0)/size || fs.Bavail > ^uint64(0)/size || fs.Bfree > ^uint64(0)/size {
		return prepareFactsFS{}, lifecycleError("prepare_facts_filesystem_rejected")
	}
	return prepareFactsFS{Scope: scope, PathSHA256: digestRaw([]byte(path)), TotalBytes: fs.Blocks * size, AvailableBytes: fs.Bavail * size, FreeBytes: fs.Bfree * size}, nil
}
func prepareFactsArchiveParent(path string) (string, error) {
	for {
		info, e := os.Lstat(path)
		if e == nil {
			if !info.IsDir() {
				break
			}
			return path, nil
		}
		if !os.IsNotExist(e) || path == "/" {
			break
		}
		path = filepath.Dir(path)
	}
	return "", lifecycleError("prepare_facts_filesystem_rejected")
}

func prepareFactsBoundRequestPath(path, operation, run string) bool {
	return runRE.MatchString(operation) && runRE.MatchString(run) && path == filepath.Join("/opt/backups/qs-server/compatibility-retirement", operation, "prepare-facts-request-"+run+".json")
}

func runPrepareFacts(ctx context.Context, path, expected, op, run string) (receipt prepareFactsReceipt, result error) {
	started := time.Now()
	receipt = prepareFactsReceipt{FormatVersion: 1, Kind: "readonly_prepare_facts_observation", Operation: "prepare", PrepareMode: "prepare-facts", SourceSHA: sourceSHA, OperationID: op, RunID: run, RequestSHA256: expected, TargetHash: digest(targets), DiagnosticOnly: true, Files: []prepareFactsFile{}, Capacity: []prepareFactsFS{}, ErrorCategory: "prepare_facts_incomplete"}
	defer func() {
		if result != nil {
			receipt.ElapsedMillis = time.Since(started).Milliseconds()
			receipt.ErrorCategory = lifecycleCategory(result)
			receipt.FactsObservationComplete = false
		}
	}()
	uid64, e := strconv.ParseUint(os.Getenv("QS_RETIREMENT_SOURCE_UID"), 10, 32)
	if e != nil || os.Getuid() != 0 || os.Geteuid() != 0 || !prepareFactsBoundRequestPath(path, op, run) || privateDir(lifecycleRootBatch(op, run)) != nil {
		return receipt, lifecycleError("prepare_facts_root_once_required")
	}
	raw, e := readLifecycleOwnedBytes(path, expected, uint32(uid64), 256<<10)
	var r prepareFactsRequest
	if e != nil {
		return receipt, e
	}
	if e = decodePrepareFacts(raw, &r); e != nil {
		return receipt, e
	}
	if e = validatePrepareFactsRequest(r, op, run); e != nil {
		return receipt, e
	}
	receipt.ObservationApprovalSHA256 = r.ObservationApprovalSHA256
	receipt.Producer = r.Inventory
	original := filepath.Join("/opt/backups/qs-server/compatibility-retirement", op)
	source := filepath.Join(original, "inventory-"+r.Inventory.RunID)
	raw, e = readLifecycleOwnedBytes(filepath.Join(original, "inventory-request.json"), r.Inventory.RequestSHA256, uint32(uid64), 256<<10)
	var req request
	if e != nil {
		return receipt, e
	}
	if e = decodePrepareFacts(raw, &req); e != nil {
		return receipt, e
	}
	var inventory report
	raw, e = readLifecycleOwnedBytes(filepath.Join(source, lifecycleSourceNames[0]), r.Inventory.ReportSHA256, uint32(uid64), 256<<10)
	if e != nil {
		return receipt, e
	}
	if e = decodePrepareFacts(raw, &inventory); e != nil {
		return receipt, e
	}
	if e = validatePrepareFactsInventory(r, req, inventory); e != nil {
		return receipt, e
	}
	if e = verifyLifecycleRestoreImages(ctx, r.RestoreEngines); e != nil {
		return receipt, e
	}
	docker, e := lifecycleDockerExecutable()
	if e != nil {
		return receipt, e
	}
	raw, e = lifecycleDocker(ctx, docker, "info", "--format", "{{json .DockerRootDir}}")
	var dockerRoot string
	if e != nil || json.Unmarshal(raw, &dockerRoot) != nil || dockerRoot == "" {
		return receipt, lifecycleError("prepare_facts_docker_root_unproven")
	}
	archiveParent, e := prepareFactsArchiveParent(r.ArchiveDirectory)
	if e != nil {
		return receipt, e
	}
	for _, p := range [][2]string{{"source", source}, {"staging", lifecycleRootBatch(op, run)}, {"archive", archiveParent}, {"docker", dockerRoot}} {
		fact, e := observePrepareFactsFS(p[0], p[1])
		if e != nil {
			return receipt, e
		}
		receipt.Capacity = append(receipt.Capacity, fact)
	}
	owner, e := openLifecyclePreparationOwner(ctx, lifecycleRequest{})
	if e != nil {
		return receipt, e
	}
	defer func() {
		if owner != nil {
			_ = owner.Close()
		}
	}()
	if e = observePrepareFactsIdentity(ctx, owner, req, inventory); e != nil {
		return receipt, e
	}
	ordered, e := backup.ReadOrderedMongoSchema(ctx, owner.originalDB)
	if e != nil {
		return receipt, lifecycleError("prepare_facts_ordered_schema_read_failed")
	}
	for i, name := range lifecycleSourceNames {
		maximum := int64(16 << 30)
		keep := false
		if i == 0 {
			maximum = 256 << 10
			keep = true
		}
		if i == 1 || i == 2 {
			maximum = 16 << 20
			keep = true
		}
		fact, body, e := hashPrepareFactsFile(ctx, filepath.Join(source, name), uint32(uid64), maximum, keep)
		if e != nil {
			return receipt, e
		}
		if i == 0 && fact.SHA256 != r.Inventory.ReportSHA256 {
			return receipt, lifecycleError("prepare_facts_source_changed")
		}
		if i == 1 || i == 2 {
			var metadata map[string]any
			if rejectDuplicateJSON(body) != nil || json.Unmarshal(body, &metadata) != nil || metadata["schema"] == nil || digest(metadata["schema"]) != inventory.DatabaseBindings[[]string{"mysql", "mongodb"}[i-1]].CatalogHash {
				return receipt, lifecycleError("prepare_facts_metadata_binding_rejected")
			}
		}
		receipt.Files = append(receipt.Files, fact)
	}
	end, e := backup.ReadOrderedMongoSchema(ctx, owner.originalDB)
	if e != nil || end.SHA256() != ordered.SHA256() {
		return receipt, lifecycleError("prepare_facts_ordered_schema_changed")
	}
	if e = observePrepareFactsIdentity(ctx, owner, req, inventory); e != nil {
		return receipt, e
	}
	if e = owner.closeHandles(ctx); e != nil {
		return receipt, e
	}
	owner = nil
	if ctx.Err() != nil {
		return receipt, lifecycleError("prepare_facts_read_budget_exceeded")
	}
	receipt.OrderedMongoSchemaSHA256 = ordered.SHA256()
	receipt.RestoreEngines = r.RestoreEngines
	receipt.SocketKind = "fixed_root_owned_unix_docker"
	receipt.FactsObservationComplete = true
	receipt.ErrorCategory = "none"
	receipt.ElapsedMillis = time.Since(started).Milliseconds()
	// Complete/drop remain false: no history verdict, backup, restore budget,
	// approval, persistent business evidence or execution capability is minted.
	encoded, e := json.Marshal(receipt)
	if e != nil || writeJSON(filepath.Join(lifecycleRootBatch(op, run), "prepare-facts.private.json"), receipt) != nil {
		return receipt, lifecycleError("prepare_facts_private_observation_write_failed")
	}
	receipt.PrivateObservationSHA256 = digestRaw(append(encoded, '\n'))
	return receipt, nil
}
