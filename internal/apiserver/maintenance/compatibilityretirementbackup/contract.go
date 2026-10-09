// Package compatibilityretirementbackup creates temporary four-object rollback
// assets and performs an actual isolated restore. It never retires business
// responsibility or authorizes DROP. Database handles are borrowed from hosts.
package compatibilityretirementbackup

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"hash"
	"io"
	"regexp"
	"time"
)

type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrApproval       Error = "backup_approval_binding_rejected"
	ErrStructure      Error = "backup_structure_incomplete_or_changed"
	ErrIdentity       Error = "backup_database_identity_or_head_changed"
	ErrSource         Error = "backup_source_copy_incomplete_or_changed"
	ErrPrivate        Error = "backup_private_asset_rejected"
	ErrRead           Error = "backup_borrowed_read_failed"
	ErrRestore        Error = "backup_restore_unknown_or_failed"
	ErrContent        Error = "backup_restored_content_not_equal"
	ErrIsolation      Error = "backup_restore_isolation_unproven"
	ErrBudget         Error = "backup_restore_budget_exceeded"
	ErrSerialization  Error = "backup_private_serialization_forbidden"
	MaxRestoreSeconds       = 600
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sourcePattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var runPattern = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)
var targetNames = [4]string{"domain_event_outbox", "ai_bridge_commands", "ai_messaging_legacy_commands", "domain_event_outbox"}
var sourceNames = [4]string{"mysql-domain_event_outbox.source.ndjson", "mysql-ai_bridge_commands.source.ndjson", "mysql-ai_messaging_legacy_commands.source.ndjson", "mongodb-domain_event_outbox.source.bsonframes"}

// Approval is independently approved input, never a source/header or boolean
// assertion of completion. OrderedMongoSchemaHash requires a distinct approval
// after ReadOrderedMongoSchema: v2 ExtJSON maps lose compound-index key order.
type Approval struct {
	InventorySHA256, SQLMetadataSHA256, MongoMetadataSHA256 string
	OrderedMongoSchemaSHA256                                string
	SourceSHA, OperationID, RunID, RequestHash              string
}
type Inputs struct {
	Inventory, SQLMetadata, MongoMetadata io.Reader
	Sources                               [4]io.Reader
}
type SourceSnapshot struct {
	Database       string                    `json:"database"`
	Name           string                    `json:"name"`
	Kind           string                    `json:"kind"`
	Present        bool                      `json:"present"`
	Complete       bool                      `json:"complete"`
	Records        uint64                    `json:"records"`
	SchemaHash     string                    `json:"schema_hash"`
	DataHash       string                    `json:"data_hash"`
	IdentityHash   string                    `json:"identity_hash"`
	Bytes          uint64                    `json:"bytes"`
	Classification map[string]uint64         `json:"classification"`
	SourceFile     string                    `json:"source_file"`
	ErrorCategory  string                    `json:"error_category"`
	Boundary       retirement.SourceBoundary `json:"boundary"`
	Passes         int                       `json:"equal_full_passes"`
	Pages          uint64                    `json:"pages"`
	NextCycle      bool                      `json:"next_cycle_required"`
}
type Binding struct {
	IdentityHash        string          `json:"identity_hash"`
	AnchorHash          string          `json:"database_anchor_hash"`
	GenerationHash      string          `json:"migration_generation_hash"`
	IdentityMatch       bool            `json:"expected_identity_match"`
	Version             uint64          `json:"migration_version"`
	Dirty               bool            `json:"migration_dirty"`
	HeadMatch           bool            `json:"expected_migration_match"`
	CatalogHash         string          `json:"catalog_hash"`
	NonTargetHash       string          `json:"non_target_schema_hash"`
	MetadataComplete    bool            `json:"metadata_complete"`
	Permissions         map[string]bool `json:"permissions"`
	OutsideDependencies uint64          `json:"outside_dependencies"`
	DependencyCoverage  bool            `json:"dependency_coverage_complete"`
	InboundCoverage     bool            `json:"inbound_foreign_key_coverage_complete"`
	DependencyScope     string          `json:"dependency_scope"`
	TextReview          bool            `json:"dependency_text_review_required"`
	ErrorCategory       string          `json:"error_category"`
}
type inventory struct {
	Format        int                `json:"format_version"`
	Kind          string             `json:"kind"`
	SourceSHA     string             `json:"source_sha"`
	OperationID   string             `json:"operation_id"`
	RunID         string             `json:"run_id"`
	RequestHash   string             `json:"request_hash"`
	TargetHash    string             `json:"target_hash"`
	ObservedAt    string             `json:"observed_at"`
	Complete      bool               `json:"complete"`
	DropReady     bool               `json:"drop_ready"`
	Bindings      map[string]Binding `json:"database_bindings"`
	Targets       []SourceSnapshot   `json:"targets"`
	Protocol      string             `json:"source_bytes_protocol"`
	Semantics     string             `json:"consistency_semantics"`
	ErrorCategory string             `json:"error_category"`
	BoundaryHash  string             `json:"boundary_report_hash"`
	Diagnostic    bool               `json:"diagnostic_only"`
}
type SQLStructure struct {
	DDL                   string                `json:"ddl"`
	Columns               retirement.SQLColumns `json:"columns"`
	CharacterSets         [][]*string           `json:"column_character_sets"`
	ShowCreateEnvironment [][]*string           `json:"show_create_environment"`
}
type MongoStructure struct {
	Collection []byte   `json:"collection_bson"`
	Indexes    [][]byte `json:"indexes_bson"`
}
type manifest struct {
	InventoryRaw         []byte `json:"inventory_raw"`
	SourceMongoNamespace string `json:"source_mongo_namespace"`
	SourceMongoProcessID string `json:"source_mongo_process_id"`

	Version                int             `json:"version"`
	Approval               Approval        `json:"approval"`
	Inventory              inventory       `json:"inventory"`
	SQL                    [3]SQLStructure `json:"sql"`
	Mongo                  MongoStructure  `json:"mongo"`
	Assets                 [4]Asset        `json:"assets"`
	SQLMetadataHash        string          `json:"sql_metadata_hash"`
	MongoMetadataHash      string          `json:"mongo_metadata_hash"`
	OrderedMongoSchemaHash string          `json:"ordered_mongo_schema_hash"`
}
type Asset struct {
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
	Bytes    int64  `json:"file_bytes"`
}

// Archive is opaque. Opening it checks every registered source to actual EOF;
// a caller cannot fabricate a successful backup using imported booleans.
type Archive struct {
	dir, digest string
	data        manifest
}

func (*Archive) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (*Archive) String() string               { return "opaque temporary four-object backup" }
func (a *Archive) GoString() string           { return a.String() }

type Summary struct {
	ArchiveSHA256                string `json:"archive_sha256"`
	OrderedMongoSchemaSHA256     string `json:"ordered_mongo_schema_sha256"`
	TargetCount                  int    `json:"target_count"`
	TemporaryOriginalBodies      bool   `json:"temporary_original_bodies"`
	PurgeAfterAcceptanceRequired bool   `json:"purge_after_acceptance_required"`
	SourceOriginAuthentication   string `json:"source_origin_authentication"`
	ProductionFence              string `json:"production_fence"`
	ProductionContainerAdapter   string `json:"production_container_adapter"`
	DropReady                    bool   `json:"drop_ready"`
}

func (a *Archive) Summary() Summary {
	s := Summary{TemporaryOriginalBodies: true, PurgeAfterAcceptanceRequired: true, SourceOriginAuthentication: "host_binding_required", ProductionFence: "unproven", ProductionContainerAdapter: "not_integrated"}
	if a != nil {
		s.ArchiveSHA256 = a.digest
		s.OrderedMongoSchemaSHA256 = a.data.OrderedMongoSchemaHash
		s.TargetCount = 4
	}
	return s
}

type TargetLoadMetrics struct {
	Database                string `json:"database"`
	Name                    string `json:"name"`
	SourceRecords           uint64 `json:"source_records"`
	RestoredRecords         uint64 `json:"restored_records"`
	SourceRawBytes          uint64 `json:"source_raw_bytes"`
	RestoredRawBytes        uint64 `json:"restored_raw_bytes"`
	SourceFileBytes         int64  `json:"source_file_bytes"`
	InsertStatements        uint64 `json:"insert_statements"`
	AutocommitInsertBatches uint64 `json:"autocommit_insert_batches"`
	ExplicitTransactions    uint64 `json:"explicit_transactions"`
	ExplicitCommits         uint64 `json:"explicit_commits"`
	MaxBatchRecords         uint64 `json:"max_batch_records"`
	MaxBatchRawBytes        uint64 `json:"max_batch_raw_bytes"`
}

type Verification struct {
	Targets                            []TargetLoadMetrics `json:"targets"`
	ProductionBoundRestoreBudgetProven bool                `json:"production_bound_restore_budget_proven"`
	Database                           string              `json:"database"`
	ArchiveSHA256                      string              `json:"archive_sha256"`
	RestoreIdentityHash                string              `json:"restore_identity_hash"`
	TargetCount                        int                 `json:"target_count"`
	ContentEqual                       bool                `json:"complete_content_equal"`
	SchemaEqual                        bool                `json:"complete_schema_equal"`
	SQLStructureSemantics              string              `json:"sql_structure_semantics"`
	OriginalSQLDDLHashes               []string            `json:"original_sql_ddl_hashes"`
	RestoredSQLDDLHashes               []string            `json:"restored_sql_ddl_hashes"`
	SQLSemanticHashes                  []string            `json:"sql_semantic_hashes"`
	SQLShowCreateEnvironmentSHA256     string              `json:"sql_show_create_environment_sha256"`
	StartedAt                          time.Time           `json:"started_at"`
	FinishedAt                         time.Time           `json:"finished_at"`
	ElapsedMillis                      int64               `json:"elapsed_millis"`
	SourceOriginAuthentication         string              `json:"source_origin_authentication"`
	ForeignKeyBusinessClosure          string              `json:"foreign_key_business_closure"`
	Isolation                          string              `json:"isolation"`
	DropReady                          bool                `json:"drop_ready"`
}

func boundedRestore(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, nil, ErrBudget
	}
	q, c := context.WithTimeout(ctx, MaxRestoreSeconds*time.Second)
	return q, c, nil
}
func sha(raw []byte) string { s := sha256.Sum256(raw); return hex.EncodeToString(s[:]) }
func jsonSHA(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	return sha(b)
}
func frame(h hash.Hash, v []byte, null bool) {
	var p [9]byte
	if !null {
		p[0] = 1
		binary.BigEndian.PutUint64(p[1:], uint64(len(v)))
	}
	_, _ = h.Write(p[:])
	if !null {
		_, _ = h.Write(v)
	}
}
func parts(values ...string) string {
	h := sha256.New()
	for _, v := range values {
		frame(h, []byte(v), false)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func quote(v string) string { return "`" + regexp.MustCompile("`").ReplaceAllString(v, "``") + "`" }
