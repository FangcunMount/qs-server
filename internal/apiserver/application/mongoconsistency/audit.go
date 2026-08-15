// Package mongoconsistency owns the bounded, read-only audit workflow for
// cross-collection Mongo invariants. It never exposes repair capability.
package mongoconsistency

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Phase string

const (
	PhaseAnswerSheetOutbox     Phase = "answersheet_outbox"
	PhaseOutboxAnswerSheet     Phase = "outbox_answersheet"
	PhaseGenerationRun         Phase = "generation_run"
	PhaseGeneratedTerminal     Phase = "generated_terminal"
	PhaseRetryOutbox           Phase = "retry_outbox"
	PhaseModelRelease          Phase = "model_release"
	PhasePublishedModelRuntime Phase = "published_model_runtime"
	PhaseCompleted             Phase = "completed"
	CheckpointKey                    = "mongo_consistency"
	CheckpointSchemaVersion          = 1
)

var AuditPhases = []Phase{
	PhaseAnswerSheetOutbox,
	PhaseOutboxAnswerSheet,
	PhaseGenerationRun,
	PhaseGeneratedTerminal,
	PhaseRetryOutbox,
	PhaseModelRelease,
	PhasePublishedModelRuntime,
}

type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

const (
	DriftAnswerSheetMissingOutbox       = "answersheet_missing_outbox"
	DriftOutboxMissingAnswerSheet       = "outbox_missing_answersheet"
	DriftGenerationMissingRun           = "generation_missing_run"
	DriftGenerationRunStateMismatch     = "generation_run_state_mismatch"
	DriftGeneratedMissingArtifact       = "generated_missing_artifact"
	DriftGeneratedMissingTerminalOutbox = "generated_missing_terminal_outbox"
	DriftRetryMissingScheduledOutbox    = "retry_missing_scheduled_outbox"
	DriftModelActiveSnapshotMismatch    = "model_active_snapshot_mismatch"
	DriftModelQuestionnaireMissing      = "model_questionnaire_snapshot_missing"
	DriftModelBindingMismatch           = "model_binding_mismatch"
	DriftModelNormMissing               = "model_norm_missing"
	DriftModelDefinitionHashMismatch    = "model_definition_hash_mismatch"
	DriftModelRuntimeInvalid            = "model_runtime_materialization_invalid"
)

var DriftSeverities = map[string]Severity{
	DriftAnswerSheetMissingOutbox:       SeverityHigh,
	DriftOutboxMissingAnswerSheet:       SeverityHigh,
	DriftGenerationMissingRun:           SeverityHigh,
	DriftGenerationRunStateMismatch:     SeverityHigh,
	DriftGeneratedMissingArtifact:       SeverityHigh,
	DriftGeneratedMissingTerminalOutbox: SeverityHigh,
	DriftRetryMissingScheduledOutbox:    SeverityHigh,
	DriftModelActiveSnapshotMismatch:    SeverityHigh,
	DriftModelQuestionnaireMissing:      SeverityHigh,
	DriftModelBindingMismatch:           SeverityHigh,
	DriftModelNormMissing:               SeverityHigh,
	DriftModelDefinitionHashMismatch:    SeverityMedium,
	DriftModelRuntimeInvalid:            SeverityHigh,
}

var (
	ErrCheckpointMissing = errors.New("mongo consistency audit checkpoint is missing")
	ErrCheckpointCAS     = errors.New("mongo consistency audit checkpoint CAS conflict")
)

type Finding struct {
	Kind     string   `json:"kind"`
	Severity Severity `json:"severity"`
	SampleID string   `json:"sample_id,omitempty"`
}

type Statistics struct {
	Scanned  int64               `json:"scanned"`
	Findings map[string]int64    `json:"findings"`
	Samples  map[string][]string `json:"samples,omitempty"`
}

func NewStatistics() Statistics {
	return Statistics{Findings: make(map[string]int64), Samples: make(map[string][]string)}
}

func (s *Statistics) Add(batch BatchResult, maxSamples int) {
	if s.Findings == nil {
		s.Findings = make(map[string]int64)
	}
	if s.Samples == nil {
		s.Samples = make(map[string][]string)
	}
	s.Scanned += int64(batch.Scanned)
	for _, finding := range batch.Findings {
		s.Findings[finding.Kind]++
		if finding.SampleID == "" || maxSamples <= 0 || len(s.Samples[finding.Kind]) >= maxSamples {
			continue
		}
		s.Samples[finding.Kind] = append(s.Samples[finding.Kind], finding.SampleID)
	}
}

func (s Statistics) Total() int64 {
	var total int64
	for _, count := range s.Findings {
		total += count
	}
	return total
}

type CompletedCycle struct {
	CycleID     string     `json:"cycle_id"`
	CompletedAt time.Time  `json:"completed_at"`
	Statistics  Statistics `json:"statistics"`
}

type Checkpoint struct {
	SchemaVersion int             `json:"schema_version"`
	Revision      int64           `json:"revision"`
	CycleID       string          `json:"cycle_id"`
	Phase         Phase           `json:"phase"`
	Cursor        uint64          `json:"cursor"`
	UpperBound    uint64          `json:"upper_bound"`
	Working       Statistics      `json:"working"`
	LastCompleted *CompletedCycle `json:"last_completed,omitempty"`
	NextCycleAt   time.Time       `json:"next_cycle_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type BatchRequest struct {
	Phase      Phase
	AfterID    uint64
	UpperBound uint64
	Limit      int
	MaxTime    time.Duration
	MaxSamples int
}

type BatchResult struct {
	NextID    uint64
	Scanned   int
	Exhausted bool
	Findings  []Finding
}

type BatchOutcome struct {
	CycleID     string     `json:"cycle_id"`
	Phase       Phase      `json:"phase"`
	Cursor      uint64     `json:"cursor"`
	UpperBound  uint64     `json:"upper_bound"`
	Scanned     int        `json:"scanned"`
	Findings    int        `json:"findings"`
	Completed   bool       `json:"completed"`
	Idle        bool       `json:"idle"`
	NextCycleAt time.Time  `json:"next_cycle_at,omitempty"`
	Statistics  Statistics `json:"statistics,omitempty"`
}

type RunOptions struct {
	BatchSize     int
	BatchTimeout  time.Duration
	CycleInterval time.Duration
	MaxSamples    int
}

type Scanner interface {
	UpperBound(context.Context, Phase, time.Duration) (uint64, error)
	ScanBatch(context.Context, BatchRequest) (BatchResult, error)
}

// CheckpointStore is the only write-capable audit port. Scanner deliberately
// has no mutation method, which keeps Mongo business documents read-only.
type CheckpointStore interface {
	Load(context.Context) (Checkpoint, error)
	Save(context.Context, int64, Checkpoint) error
}

type RunnerService interface {
	RunAuditBatch(context.Context, RunOptions) (BatchOutcome, error)
}

type Service struct {
	scanner    Scanner
	checkpoint CheckpointStore
	now        func() time.Time
}

func NewService(scanner Scanner, checkpoint CheckpointStore) *Service {
	return &Service{scanner: scanner, checkpoint: checkpoint, now: time.Now}
}

func (s *Service) RunAuditBatch(ctx context.Context, opts RunOptions) (BatchOutcome, error) {
	if s == nil || s.scanner == nil || s.checkpoint == nil {
		observeReady(false)
		return BatchOutcome{}, fmt.Errorf("mongo consistency audit is not configured")
	}
	if opts.BatchSize <= 0 || opts.BatchTimeout <= 0 || opts.CycleInterval <= 0 || opts.MaxSamples < 0 {
		return BatchOutcome{}, fmt.Errorf("mongo consistency audit options are invalid")
	}
	checkpoint, err := s.checkpoint.Load(ctx)
	missing := errors.Is(err, ErrCheckpointMissing)
	if err != nil && !missing {
		observeError("checkpoint_load")
		return BatchOutcome{}, fmt.Errorf("load mongo consistency checkpoint: %w", err)
	}
	observeReady(true)
	now := s.now().UTC()
	if missing || (checkpoint.Phase == PhaseCompleted && !now.Before(checkpoint.NextCycleAt)) {
		return s.startCycle(ctx, checkpoint, opts, now)
	}
	if checkpoint.Phase == PhaseCompleted {
		if checkpoint.LastCompleted == nil {
			observeReady(false)
			observeError("checkpoint_invalid")
			return BatchOutcome{}, fmt.Errorf("completed mongo consistency checkpoint has no completed cycle")
		}
		// Restore process-local gauges after a restart instead of temporarily
		// reporting a zero timestamp and zero drift until the next daily cycle.
		observeCompleted(checkpoint.LastCompleted)
		return BatchOutcome{CycleID: checkpoint.CycleID, Phase: PhaseCompleted, Completed: true, Idle: true, NextCycleAt: checkpoint.NextCycleAt, Statistics: cloneStatistics(checkpoint.LastCompleted.Statistics)}, nil
	}

	started := time.Now()
	result, err := s.scanner.ScanBatch(ctx, BatchRequest{
		Phase: checkpoint.Phase, AfterID: checkpoint.Cursor, UpperBound: checkpoint.UpperBound,
		Limit: opts.BatchSize, MaxTime: opts.BatchTimeout, MaxSamples: opts.MaxSamples,
	})
	if err != nil {
		observeError("scan")
		observeBatch(checkpoint.Phase, "error", time.Since(started), 0)
		return BatchOutcome{CycleID: checkpoint.CycleID, Phase: checkpoint.Phase, Cursor: checkpoint.Cursor, UpperBound: checkpoint.UpperBound}, fmt.Errorf("scan mongo consistency phase %s: %w", checkpoint.Phase, err)
	}
	if err := ctx.Err(); err != nil {
		observeError("batch_context")
		observeBatch(checkpoint.Phase, "error", time.Since(started), 0)
		return BatchOutcome{}, err
	}
	next := checkpoint
	next.Revision++
	next.Cursor = result.NextID
	next.UpdatedAt = now
	next.Working.Add(result, opts.MaxSamples)
	completed := false
	if result.Exhausted {
		if nextPhase, ok := phaseAfter(checkpoint.Phase); ok {
			upper, upperErr := s.scanner.UpperBound(ctx, nextPhase, opts.BatchTimeout)
			if upperErr != nil {
				observeError("upper_bound")
				return BatchOutcome{}, fmt.Errorf("load mongo consistency upper bound for %s: %w", nextPhase, upperErr)
			}
			next.Phase, next.Cursor, next.UpperBound = nextPhase, 0, upper
		} else {
			completed = true
			next.Phase, next.Cursor, next.UpperBound = PhaseCompleted, 0, 0
			next.LastCompleted = &CompletedCycle{CycleID: checkpoint.CycleID, CompletedAt: now, Statistics: cloneStatistics(next.Working)}
			next.NextCycleAt = now.Add(opts.CycleInterval)
		}
	}
	if err := s.checkpoint.Save(ctx, checkpoint.Revision, next); err != nil {
		if errors.Is(err, ErrCheckpointCAS) {
			observeCheckpointCAS()
		}
		observeError("checkpoint_save")
		return BatchOutcome{}, err
	}
	observeBatch(checkpoint.Phase, "success", time.Since(started), result.Scanned)
	if completed {
		observeCompleted(next.LastCompleted)
	}
	return BatchOutcome{
		CycleID: checkpoint.CycleID, Phase: checkpoint.Phase, Cursor: result.NextID,
		UpperBound: checkpoint.UpperBound, Scanned: result.Scanned, Findings: len(result.Findings),
		Completed: completed, NextCycleAt: next.NextCycleAt, Statistics: cloneStatistics(next.Working),
	}, nil
}

func (s *Service) startCycle(ctx context.Context, previous Checkpoint, opts RunOptions, now time.Time) (BatchOutcome, error) {
	first := AuditPhases[0]
	upper, err := s.scanner.UpperBound(ctx, first, opts.BatchTimeout)
	if err != nil {
		observeError("upper_bound")
		return BatchOutcome{}, fmt.Errorf("load mongo consistency upper bound: %w", err)
	}
	next := Checkpoint{
		SchemaVersion: CheckpointSchemaVersion, Revision: previous.Revision + 1,
		CycleID: meta.New().String(), Phase: first, UpperBound: upper,
		Working: NewStatistics(), LastCompleted: previous.LastCompleted, UpdatedAt: now,
	}
	if err := s.checkpoint.Save(ctx, previous.Revision, next); err != nil {
		if errors.Is(err, ErrCheckpointCAS) {
			observeCheckpointCAS()
		}
		observeError("checkpoint_save")
		return BatchOutcome{}, err
	}
	return BatchOutcome{CycleID: next.CycleID, Phase: first, UpperBound: upper, Statistics: next.Working}, nil
}

func phaseAfter(current Phase) (Phase, bool) {
	for index, phase := range AuditPhases {
		if phase == current && index+1 < len(AuditPhases) {
			return AuditPhases[index+1], true
		}
	}
	return "", false
}

func ParseScopes(values []string) ([]Phase, error) {
	if len(values) == 0 {
		return append([]Phase(nil), AuditPhases...), nil
	}
	allowed := make(map[Phase]struct{}, len(AuditPhases))
	for _, phase := range AuditPhases {
		allowed[phase] = struct{}{}
	}
	seen := make(map[Phase]struct{})
	result := make([]Phase, 0, len(values))
	for _, value := range values {
		phase := Phase(value)
		if _, ok := allowed[phase]; !ok {
			return nil, fmt.Errorf("unknown mongo consistency audit scope %q", value)
		}
		if _, duplicate := seen[phase]; duplicate {
			continue
		}
		seen[phase] = struct{}{}
		result = append(result, phase)
	}
	return result, nil
}

func SortedFindingKinds(stats Statistics) []string {
	result := make([]string, 0, len(stats.Findings))
	for kind := range stats.Findings {
		result = append(result, kind)
	}
	sort.Strings(result)
	return result
}

func cloneStatistics(source Statistics) Statistics {
	result := NewStatistics()
	result.Scanned = source.Scanned
	for kind, count := range source.Findings {
		result.Findings[kind] = count
	}
	for kind, samples := range source.Samples {
		result.Samples[kind] = append([]string(nil), samples...)
	}
	return result
}
