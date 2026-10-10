package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	aibridge "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	binding "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/aimessagingbinding"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	jose "github.com/go-jose/go-jose/v4"
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
type prepareFactsProtection struct {
	SHA256          string `json:"sha256"`
	DecryptKeys     int    `json:"decrypt_key_count"`
	TrustedSigners  int    `json:"trusted_signer_count"`
	BindingSHA256   string `json:"source_binding_sha256"`
	SourceSHA       string `json:"source_sha"`
	ImageSHA256     string `json:"image_id_sha256"`
	ContainerSHA256 string `json:"container_id_sha256"`
}
type prepareFactsReceipt struct {
	Protection                *prepareFactsProtection                  `json:"observed_ai_message_protection,omitempty"`
	FormatVersion             int                                      `json:"format_version"`
	Kind                      string                                   `json:"kind"`
	Operation                 string                                   `json:"operation"`
	PrepareMode               string                                   `json:"prepare_mode"`
	SourceSHA                 string                                   `json:"source_sha"`
	OperationID               string                                   `json:"operation_id"`
	RunID                     string                                   `json:"run_id"`
	RequestSHA256             string                                   `json:"request_sha256"`
	ObservationApprovalSHA256 string                                   `json:"observation_approval_sha256"`
	TargetHash                string                                   `json:"target_hash"`
	Complete                  bool                                     `json:"complete"`
	FactsObservationComplete  bool                                     `json:"prepare_facts_observation_complete"`
	DiagnosticOnly            bool                                     `json:"diagnostic_only"`
	ExecutionAllowed          bool                                     `json:"execution_allowed"`
	DropReady                 bool                                     `json:"drop_ready"`
	Producer                  prepareFactsProducer                     `json:"observed_inventory_producer"`
	Files                     []prepareFactsFile                       `json:"prepare_source_files"`
	OrderedMongoSchemaSHA256  string                                   `json:"observed_ordered_mongo_schema_sha256"`
	RestoreEngines            *lifecycleRestoreEngines                 `json:"observed_restore_engines,omitempty"`
	Capacity                  []prepareFactsFS                         `json:"observed_filesystems"`
	SocketKind                string                                   `json:"observed_socket_kind"`
	PrivateObservationSHA256  string                                   `json:"prepare_facts_private_observation_sha256,omitempty"`
	AIRuntime                 *retirement.AIExternalRuntimeObservation `json:"observed_ai_runtime,omitempty"`
	ElapsedMillis             int64                                    `json:"observation_elapsed_millis"`
	ErrorCategory             string                                   `json:"error_category"`
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
func observePrepareFactsIdentity(ctx context.Context, o *lifecyclePreparationOwner, req request, inventory report) error {
	return observeBorrowedDatabaseIdentity(ctx, o, req.Identities, req.Migrations, req.MongoNamespaceAnchor, inventory.DatabaseBindings["mongodb"].DatabaseAnchorHash)
}

// Expectations are already validated original producer facts. This function
// rereads both actual borrowed connections; it never opens or closes the pools.
func observeBorrowedDatabaseIdentity(ctx context.Context, o *lifecyclePreparationOwner, identities map[string]string, migrations map[string]uint64, anchor *identitymeta.MongoNamespaceAnchor, mongoAnchorHash string) (result error) {
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
	if e != nil || len(ids) != 1 || !mysqlUUIDRE.MatchString(val(ids[0], 0)) || val(ids[0], 1) != os.Getenv("MYSQL_DATABASE") || !strings.HasPrefix(val(ids[0], 2), "8.") || hashParts("mysql_database_identity_v1", val(ids[0], 0), val(ids[0], 1)) != identities["mysql"] {
		return lifecycleError("prepare_facts_identity_rejected")
	}
	q, c = context.WithTimeout(ctx, 30*time.Second)
	heads, e := scanSQL(q, tx, "SELECT version,dirty FROM schema_migrations LIMIT 2")
	c()
	if e != nil || len(heads) != 1 || val(heads[0], 0) != strconv.FormatUint(migrations["mysql"], 10) || val(heads[0], 1) != "0" {
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
	if e != nil || hashParts("mongodb_database_identity_v1", string(canonical), os.Getenv("MONGODB_DBNAME"), hex.EncodeToString(b)) != identities["mongodb"] {
		return lifecycleError("prepare_facts_identity_rejected")
	}
	if anchor != nil {
		actual, e := mongoNamespaceAnchor(ctx, db)
		if e != nil || !identitymeta.MatchMongoNamespaceAnchors(anchor, actual) || actual.Hash != mongoAnchorHash {
			return lifecycleError("prepare_facts_identity_rejected")
		}
	} else {
		actual, e := mongoDatabaseAnchor(ctx, db, hello)
		if e != nil || actual != mongoAnchorHash {
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
	if bson.Unmarshal(rawHeads[0], &migration) != nil || migration.Version < 1 || uint64(migration.Version) != migrations["mongodb"] || migration.Dirty {
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
	aiRuntime, e := retirement.ObserveAIExternalCurrentRuntime(ctx, false)
	if e != nil {
		return receipt, lifecycleError("prepare_facts_ai_runtime_unproven")
	}
	receipt.AIRuntime = &aiRuntime
	if ctx.Err() != nil {
		return receipt, lifecycleError("prepare_facts_read_budget_exceeded")
	}
	receipt.Protection, e = prepareFactsMessageProtection(ctx, docker, op, uint32(uid64))
	if e != nil {
		return receipt, e
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

// The existing prepare-facts root invocation is the only production caller.
// The actual running API, immutable release binding and read-only mounts supply
// the keys. No Secret name, caller key map or unreviewed path is accepted.
type prepareFactsMQContainer struct {
	Entrypoint []string `json:"entrypoint"`
	Command    []string `json:"command"`
	PID        int      `json:"pid"`
	User       string   `json:"user"`
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Image      string   `json:"image"`
	Running    bool     `json:"running"`
	Service    string   `json:"service"`
	Mounts     []struct {
		Type        string
		Source      string
		Destination string
		RW          bool
	} `json:"mounts"`
}

const prepareFactsMQInspect = `{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},"running":{{json .State.Running}},"pid":{{json .State.Pid}},"user":{{json .Config.User}},"entrypoint":{{json .Config.Entrypoint}},"command":{{json .Config.Cmd}},"service":{{json (index .Config.Labels "com.docker.compose.service")}},"mounts":{{json .Mounts}}}`

func prepareFactsKeyFile(path string, uid uint32, private bool, maximum int64) (f *os.File, raw []byte, result error) {
	// Existing release assets may be root or the source/deployment account owned.
	if filepath.Clean(path) != path || !filepath.IsAbs(path) {
		return nil, nil, lifecycleError("prepare_facts_message_protection_rejected")
	}
	for d := filepath.Dir(path); d != "/"; d = filepath.Dir(d) {
		st, e := os.Lstat(d)
		v, ok := infoStat(st)
		if e != nil || st == nil || !st.IsDir() || !ok || (v.Uid != 0 && v.Uid != uid) || (st.Mode().Perm()&0022 != 0 && st.Mode()&os.ModeSticky == 0) {
			return nil, nil, lifecycleError("prepare_facts_message_protection_rejected")
		}
	}
	f, result = os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if result != nil {
		return nil, nil, lifecycleError("prepare_facts_message_protection_rejected")
	}
	defer func() {
		if result != nil {
			_ = f.Close()
			f = nil
			raw = nil
		}
	}()
	st, e := f.Stat()
	v, ok := infoStat(st)
	if e != nil || st == nil || !st.Mode().IsRegular() || !ok || (v.Uid != 0 && v.Uid != uid) || v.Nlink != 1 || st.Mode().Perm()&0022 != 0 || (private && st.Mode().Perm()&0007 != 0) || st.Size() < 1 || st.Size() > maximum {
		return f, nil, lifecycleError("prepare_facts_message_protection_rejected")
	}
	raw, e = io.ReadAll(io.LimitReader(f, maximum+1))
	after, ae := f.Stat()
	visible, ve := os.Lstat(path)
	if e != nil || ae != nil || ve != nil || int64(len(raw)) != st.Size() || !sameLifecycleFile(st, after) || !sameLifecycleFile(after, visible) {
		return f, nil, lifecycleError("prepare_facts_message_protection_rejected")
	}
	return f, raw, nil
}

func prepareFactsProtectionBytes(keys aibridge.MessagingKeys) ([]byte, map[string]string, error) {
	if len(keys.Ring.Decrypt) < 1 || len(keys.Ring.Decrypt) > 8 || len(keys.Ring.Signers) < 1 || len(keys.Ring.Signers) > 8 {
		return nil, nil, lifecycleError("prepare_facts_message_protection_rejected")
	}
	publics := map[string]jose.JSONWebKey{keys.Signing.KeyID: keys.Signing.Public(), keys.Recipient.KeyID: keys.Recipient.Public()}
	for kid, k := range keys.Ring.Decrypt {
		if k.KeyID != kid || !strings.HasPrefix(kid, "qs.encrypt.") || k.IsPublic() {
			return nil, nil, lifecycleError("prepare_facts_message_protection_rejected")
		}
		publics[kid] = k.Public()
	}
	for kid, k := range keys.Ring.Signers {
		if k.Producer != "qs-ai" || k.Key.KeyID != kid || !strings.HasPrefix(kid, "ai.sign.") || !k.Key.IsPublic() {
			return nil, nil, lifecycleError("prepare_facts_message_protection_rejected")
		}
		publics[kid] = k.Key.Public()
	}
	fingerprints := map[string]string{}
	for kid, k := range publics {
		raw, e := json.Marshal(k)
		if e != nil {
			return nil, nil, lifecycleError("prepare_facts_message_protection_rejected")
		}
		fingerprints[kid] = digestRaw(raw)
	}
	type signer struct {
		Producer string          `json:"producer"`
		Key      jose.JSONWebKey `json:"key"`
	}
	signers := map[string]signer{}
	for kid, k := range keys.Ring.Signers {
		signers[kid] = signer{Producer: k.Producer, Key: k.Key}
	}
	raw, e := json.Marshal(struct {
		Decrypt map[string]jose.JSONWebKey `json:"decrypt_keys"`
		Signers map[string]signer          `json:"trusted_signers"`
	}{keys.Ring.Decrypt, signers})
	if e != nil || len(raw) > 256<<10 {
		return nil, nil, lifecycleError("prepare_facts_message_protection_rejected")
	}
	return append(raw, '\n'), fingerprints, nil
}

// The current container init process supplies the real host UID used by the
// existing CD-mounted readers. It is not a caller-supplied owner exception.
func prepareFactsContainerReaderUID(pid int, user string) (uint32, error) {
	userParts := strings.Split(user, ":")
	numericUID, numericErr := strconv.ParseUint(userParts[0], 10, 32)
	if pid < 1 || len(userParts) > 2 || (user != "www" && (numericErr != nil || numericUID == 0)) {
		return 0, lifecycleError("prepare_facts_message_protection_reader_rejected")
	}
	path := fmt.Sprintf("/proc/%d/status", pid)
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return 0, lifecycleError("prepare_facts_message_protection_reader_rejected")
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, 32769))
	if e != nil || len(raw) > 32768 {
		return 0, lifecycleError("prepare_facts_message_protection_reader_rejected")
	}
	var result uint32
	found := false
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "Uid:" {
			if found || len(fields) != 5 {
				return 0, lifecycleError("prepare_facts_message_protection_reader_rejected")
			}
			found = true
			for i, value := range fields[1:] {
				n, e := strconv.ParseUint(value, 10, 32)
				if e != nil || n == 0 || (i > 0 && uint32(n) != result) {
					return 0, lifecycleError("prepare_facts_message_protection_reader_rejected")
				}
				result = uint32(n)
			}
		}
	}
	if !found || (user != "www" && uint64(result) != numericUID) {
		return 0, lifecycleError("prepare_facts_message_protection_reader_rejected")
	}
	return result, nil
}

func prepareFactsMessageProtection(ctx context.Context, docker, op string, uid uint32) (_ *prepareFactsProtection, result error) {
	inspect, e := lifecycleDocker(ctx, docker, "inspect", "--format", prepareFactsMQInspect, "qs-apiserver")
	var container prepareFactsMQContainer
	if e != nil || rejectDuplicateJSON(inspect) != nil || json.Unmarshal(inspect, &container) != nil || !hashRE.MatchString(container.ID) || container.Name != "/qs-apiserver" || container.Service != "qs-apiserver" || !reflect.DeepEqual(container.Entrypoint, []string{"/app/qs-apiserver"}) || !reflect.DeepEqual(container.Command, []string{"--config=/app/configs/apiserver.prod.yaml"}) || !container.Running || !strings.HasPrefix(container.Image, "sha256:") || !hashRE.MatchString(strings.TrimPrefix(container.Image, "sha256:")) {
		return nil, lifecycleError("prepare_facts_message_protection_runtime_rejected")
	}
	source, e := lifecycleDocker(ctx, docker, "exec", container.ID, "/app/qs-ai-messaging-preflight", "--source-sha")
	sourceSHA := strings.TrimSpace(string(source))
	if e != nil || !shaRE.MatchString(sourceSHA) {
		return nil, lifecycleError("prepare_facts_message_protection_runtime_rejected")
	}
	readerUID, e := prepareFactsContainerReaderUID(container.PID, container.User)
	if e != nil {
		return nil, e
	}
	release := filepath.Join("/opt/qs-server/qs-apiserver/ai-mq-releases", sourceSHA)
	held := map[string]*os.File{}
	raws := map[string][]byte{}
	defer func() {
		for _, f := range held {
			result = errors.Join(result, f.Close())
		}
	}()
	read := func(path string, private bool, maximum int64) ([]byte, error) {
		f, raw, e := prepareFactsKeyFile(path, readerUID, private, maximum)
		if e == nil {
			held[path] = f
			raws[path] = raw
		}
		return raw, e
	}
	bindingRaw, e := read(filepath.Join(release, "binding.json"), true, 16384)
	if e != nil {
		return nil, e
	}
	b, e := binding.Decode(bindingRaw)
	if e != nil {
		return nil, lifecycleError("prepare_facts_message_protection_rejected")
	}
	metadataRaw, e := read(filepath.Join(release, "metadata.json"), true, 32768)
	if e != nil {
		return nil, e
	}
	var metadata struct {
		Source      string            `json:"source_sha"`
		Image       string            `json:"image_id"`
		BindingFile string            `json:"binding_file_sha256"`
		Binding     string            `json:"binding_sha256"`
		Config      string            `json:"rendered_config_sha256"`
		Publics     map[string]string `json:"public_key_fingerprints"`
	}
	if rejectDuplicateJSON(metadataRaw) != nil || json.Unmarshal(metadataRaw, &metadata) != nil || metadata.Source != sourceSHA || metadata.Image != container.Image || metadata.BindingFile != digestRaw(bindingRaw) || metadata.Binding != b.SHA256() {
		return nil, lifecycleError("prepare_facts_message_protection_binding_rejected")
	}
	configPath := filepath.Join(release, "apiserver.json")
	configRaw, e := read(configPath, true, 1<<20)
	if e != nil || metadata.Config != digestRaw(configRaw) {
		return nil, lifecycleError("prepare_facts_message_protection_binding_rejected")
	}
	mounts := map[string]string{}
	for _, m := range container.Mounts {
		if m.Destination == "/app/configs/apiserver.prod.yaml" || strings.HasPrefix(m.Destination, "/run/qs-server-jose/") {
			if m.Type != "bind" || m.RW || mounts[m.Destination] != "" {
				return nil, lifecycleError("prepare_facts_message_protection_mount_rejected")
			}
			mounts[m.Destination] = m.Source
		}
	}
	if mounts["/app/configs/apiserver.prod.yaml"] != configPath {
		return nil, lifecycleError("prepare_facts_message_protection_mount_rejected")
	}
	files, e := b.Files()
	if e != nil || len(mounts) != len(files)+1 {
		return nil, lifecycleError("prepare_facts_message_protection_mount_rejected")
	}
	paths := map[string]string{}
	for _, key := range files {
		expected := filepath.Join("/data/infra/qs-server-messaging/versions", b.Revision, filepath.Base(key.Path))
		if mounts[key.Path] != expected {
			return nil, lifecycleError("prepare_facts_message_protection_mount_rejected")
		}
		raw, e := read(expected, key.Private, 16384)
		if e != nil || rejectDuplicateJSON(raw) != nil {
			return nil, lifecycleError("prepare_facts_message_protection_rejected")
		}
		paths[key.Path] = fmt.Sprintf("/proc/self/fd/%d", held[expected].Fd())
	}
	opts := b.Options()
	opts.SigningKeyFile = paths[b.Signing]
	opts.AIRecipientKeyFile = paths[b.Recipient]
	opts.DecryptKeyFiles = map[string]string{}
	opts.AISignerFiles = map[string]string{}
	for kid, path := range b.Decrypt {
		opts.DecryptKeyFiles[kid] = paths[path]
	}
	for kid, path := range b.Signers {
		opts.AISignerFiles[kid] = paths[path]
	}
	keys, e := aibridge.LoadMessagingKeys(opts)
	if e != nil {
		return nil, lifecycleError("prepare_facts_message_protection_rejected")
	}
	packet, publics, e := prepareFactsProtectionBytes(keys)
	if e != nil || !reflect.DeepEqual(publics, metadata.Publics) {
		return nil, lifecycleError("prepare_facts_message_protection_binding_rejected")
	}
	// Recheck the original FDs and names after the host loader. No key is read
	// through an independently reopened, caller-selected filesystem path.
	for path, f := range held {
		before, e := f.Stat()
		if e != nil {
			return nil, lifecycleError("prepare_facts_message_protection_source_changed")
		}
		if _, e = f.Seek(0, io.SeekStart); e != nil {
			return nil, lifecycleError("prepare_facts_message_protection_source_changed")
		}
		current, e := io.ReadAll(io.LimitReader(f, int64(len(raws[path]))+1))
		after, ae := f.Stat()
		visible, ve := os.Lstat(path)
		if e != nil || ae != nil || ve != nil || !bytes.Equal(current, raws[path]) || !sameLifecycleFile(before, after) || !sameLifecycleFile(after, visible) {
			return nil, lifecycleError("prepare_facts_message_protection_source_changed")
		}
	}
	endingUID, e := prepareFactsContainerReaderUID(container.PID, container.User)
	if e != nil || endingUID != readerUID {
		return nil, lifecycleError("prepare_facts_message_protection_reader_changed")
	}
	again, e := lifecycleDocker(ctx, docker, "inspect", "--format", prepareFactsMQInspect, container.ID)
	if e != nil || !bytes.Equal(bytes.TrimSpace(again), bytes.TrimSpace(inspect)) || ctx.Err() != nil {
		return nil, lifecycleError("prepare_facts_message_protection_runtime_changed")
	}
	parent := filepath.Join("/opt/backups/qs-server/compatibility-retirement", op)
	if lifecycleSourcePrivateDirectory(parent, uid) != nil {
		return nil, lifecycleError("prepare_facts_message_protection_destination_rejected")
	}
	destination := filepath.Join(parent, "ai-message-protection.json")
	f, e := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, lifecycleError("prepare_facts_message_protection_creation_incomplete")
	}
	n, we := f.Write(packet)
	ce := f.Chown(int(uid), -1)
	se := f.Sync()
	closeErr := f.Close()
	if n != len(packet) || we != nil || ce != nil || se != nil || closeErr != nil {
		return nil, lifecycleError("prepare_facts_message_protection_creation_incomplete")
	}
	dir, e := os.OpenFile(parent, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, lifecycleError("prepare_facts_message_protection_creation_incomplete")
	}
	syncErr := dir.Sync()
	closeErr = dir.Close()
	if syncErr != nil || closeErr != nil {
		return nil, lifecycleError("prepare_facts_message_protection_creation_incomplete")
	}
	return &prepareFactsProtection{SHA256: digestRaw(packet), DecryptKeys: len(keys.Ring.Decrypt), TrustedSigners: len(keys.Ring.Signers), BindingSHA256: metadata.Binding, SourceSHA: sourceSHA, ImageSHA256: strings.TrimPrefix(container.Image, "sha256:"), ContainerSHA256: digestRaw([]byte(container.ID))}, nil
}
