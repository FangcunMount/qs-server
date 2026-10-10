package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"go.mongodb.org/mongo-driver/mongo"
	"sync"
)

const (
	ErrRecoveryAuthority   Error = "target_recovery_native_drop_authority_missing"
	ErrRecoveryBinding     Error = "target_recovery_binding_rejected"
	ErrRecoveryState       Error = "target_recovery_existing_or_partial_state_rejected"
	ErrRecoveryUnknown     Error = "target_recovery_statement_or_durability_unknown"
	ErrRecoveryHead        Error = "target_recovery_head_dirty_unknown_or_changed"
	ErrRecoveryTransaction Error = "target_recovery_sql_transaction_visibility_unproven"
	ErrRecoveryJournal     Error = "target_recovery_journal_changed_or_incomplete"
)

// TargetRecoveryRequest is expected binding, not independent approval, a writer
// fence, or permission. No serialized receipt can construct a DROP capability.
type TargetRecoveryRequest struct {
	SourceSHA            string `json:"source_sha"`
	OperationID          string `json:"operation_id"`
	OriginalRunID        string `json:"original_run_id"`
	ActualRunID          string `json:"actual_run_id"`
	ManifestSHA256       string `json:"manifest_sha256"`
	ArchiveSHA256        string `json:"archive_sha256"`
	SQLNonTargetSHA256   string `json:"mysql_non_target_sha256"`
	MongoNonTargetSHA256 string `json:"mongodb_non_target_sha256"`
	SQLHead              uint64 `json:"mysql_head"`
	MongoHead            uint64 `json:"mongodb_head"`
}

// Handles and their lifecycle remain host owned. SQL is dedicated, autocommit,
// outside a transaction. Mongo DDL/loads execute outside a session transaction.
type TargetRecoveryBorrowed struct {
	SQL   *sql.Conn
	Mongo *mongo.Database
}
type TargetRecoveryObservation struct {
	Database string `json:"database"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Records  uint64 `json:"records"`
}
type TargetRecoverySummary struct {
	ArchiveSHA256                 string                      `json:"archive_sha256"`
	ManifestSHA256                string                      `json:"manifest_sha256"`
	OperationID                   string                      `json:"operation_id"`
	Targets                       []TargetRecoveryObservation `json:"targets"`
	NativeDropProofs              int                         `json:"native_drop_proofs"`
	ProductionAuthorityIntegrated bool                        `json:"production_authority_integrated"`
	WholeWriterFenceProven        bool                        `json:"whole_writer_fence_proven"`
	DropReady                     bool                        `json:"drop_ready"`
}
type targetDropProof struct {
	self                   *targetDropProof
	plan                   *TargetRecoveryPlan
	index                  int
	intentHash, resultHash string
	nativeAbsent           bool
}

// Public preparation performs actual inspection but cannot mint DROP proof.
// Future fixed root-host execution must call package-private executeTargetDrop
// only after independent approval and a whole-writer fence. No JSON loader can
// reconstitute that private, process-bound native statement/readback capability.
type TargetRecoveryPlan struct {
	self                                                                                   *TargetRecoveryPlan
	mu                                                                                     sync.Mutex
	archive                                                                                *Archive
	borrowed                                                                               TargetRecoveryBorrowed
	request                                                                                TargetRecoveryRequest
	journal                                                                                *targetRecoveryJournal
	window                                                                                 *fence.MaintenanceWindow
	budget                                                                                 func(context.Context) (context.Context, context.CancelFunc, error)
	drop                                                                                   [4]*targetDropProof
	observations                                                                           [4]TargetRecoveryObservation
	sqlNamespace, sqlConnection, sqlBaseline, mongoBaseline, mongoProcess, mongoTargetUUID string
	blocked                                                                                bool
	bMigration                                                                             *targetBMigrationTransition
}

func (*TargetRecoveryPlan) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (*TargetRecoveryPlan) String() string {
	return "opaque target-only recovery plan; production authority not integrated"
}
func (p *TargetRecoveryPlan) summaryLocked() TargetRecoverySummary {
	s := TargetRecoverySummary{ArchiveSHA256: p.request.ArchiveSHA256, ManifestSHA256: p.request.ManifestSHA256, OperationID: p.request.OperationID, Targets: append([]TargetRecoveryObservation(nil), p.observations[:]...)}
	for i := range p.drop {
		if p.validDrop(i) {
			s.NativeDropProofs++
		}
	}
	return s
}
func (p *TargetRecoveryPlan) Summary() TargetRecoverySummary {
	if p == nil || p.self != p {
		return TargetRecoverySummary{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.summaryLocked()
}

type TargetRecoveryVerification struct {
	self    *TargetRecoveryVerification
	plan    *TargetRecoveryPlan
	summary TargetRecoverySummary
}

func (*TargetRecoveryVerification) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (v *TargetRecoveryVerification) Summary() TargetRecoverySummary {
	if v == nil || v.self != v {
		return TargetRecoverySummary{}
	}
	s := v.summary
	s.Targets = append([]TargetRecoveryObservation(nil), s.Targets...)
	return s
}
