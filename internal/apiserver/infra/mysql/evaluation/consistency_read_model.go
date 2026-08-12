package evaluation

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	evalevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"gorm.io/gorm"
)

type consistencyReadModel struct {
	db *gorm.DB
}

type outcomeEvidenceRow struct {
	AssessmentID    uint64 `gorm:"column:assessment_id"`
	ID              uint64 `gorm:"column:id"`
	EvaluationRunID string `gorm:"column:evaluation_run_id"`
	ModelKind       string `gorm:"column:model_kind"`
}

func NewConsistencyReadModel(db *gorm.DB) evaluationconsistency.Reader {
	return &consistencyReadModel{db: db}
}

func (r *consistencyReadModel) ReadBatch(ctx context.Context, afterID uint64, limit int) (evaluationconsistency.Batch, error) {
	if limit <= 0 {
		return evaluationconsistency.Batch{Items: []evaluationconsistency.AssessmentEvidence{}, CycleComplete: true}, nil
	}
	var candidates []struct {
		ID     uint64 `gorm:"column:id"`
		Status string `gorm:"column:status"`
	}
	if err := r.db.WithContext(ctx).
		Table("assessment").
		Select("id", "status").
		Where("status IN ? AND id > ? AND deleted_at IS NULL", []string{"submitted", "evaluated", "failed"}, afterID).
		Order("id ASC").
		Limit(limit).
		Find(&candidates).Error; err != nil {
		return evaluationconsistency.Batch{}, err
	}
	if len(candidates) == 0 {
		return evaluationconsistency.Batch{Items: []evaluationconsistency.AssessmentEvidence{}, CycleComplete: true}, nil
	}

	ids := make([]uint64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	outcomes, err := r.listOutcomeEvidence(ctx, ids)
	if err != nil {
		return evaluationconsistency.Batch{}, err
	}
	runs, err := r.listRunEvidence(ctx, ids)
	if err != nil {
		return evaluationconsistency.Batch{}, err
	}
	projections, err := r.listProjectionEvidence(ctx, ids)
	if err != nil {
		return evaluationconsistency.Batch{}, err
	}
	outboxes, err := r.listCommittedOutboxEvidence(ctx, ids)
	if err != nil {
		return evaluationconsistency.Batch{}, err
	}

	items := make([]evaluationconsistency.AssessmentEvidence, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, evaluationconsistency.AssessmentEvidence{
			AssessmentID: candidate.ID,
			Status:       candidate.Status,
			Outcome:      outcomes[candidate.ID],
			Run:          runs[candidate.ID],
			Projection:   projections[candidate.ID],
			Outbox:       outboxes[candidate.ID],
		})
	}
	return evaluationconsistency.Batch{
		Items:         items,
		NextCursor:    candidates[len(candidates)-1].ID,
		CycleComplete: len(candidates) < limit,
	}, nil
}

func (r *consistencyReadModel) listOutcomeEvidence(ctx context.Context, assessmentIDs []uint64) (map[uint64]*evaluationconsistency.OutcomeEvidence, error) {
	rows := make([]outcomeEvidenceRow, 0, len(assessmentIDs))
	if err := r.db.WithContext(ctx).
		Table("evaluation_outcome").
		Select("assessment_id", "id", "evaluation_run_id", "model_kind").
		Where("assessment_id IN ?", assessmentIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[uint64]*evaluationconsistency.OutcomeEvidence, len(rows))
	for _, row := range rows {
		result[row.AssessmentID] = &evaluationconsistency.OutcomeEvidence{
			ID: strconv.FormatUint(row.ID, 10), RunID: row.EvaluationRunID, ModelKind: row.ModelKind,
		}
	}
	return result, nil
}

func (r *consistencyReadModel) listRunEvidence(ctx context.Context, assessmentIDs []uint64) (map[uint64]*evaluationconsistency.RunEvidence, error) {
	var rows []struct {
		ID             uint64     `gorm:"column:id"`
		AssessmentID   uint64     `gorm:"column:assessment_id"`
		ResourceID     string     `gorm:"column:resource_id"`
		Status         string     `gorm:"column:status"`
		LeaseExpiresAt *time.Time `gorm:"column:lease_expires_at"`
		AttemptNo      uint       `gorm:"column:attempt_no"`
	}
	if err := r.db.WithContext(ctx).
		Table("runtime_checkpoint").
		Select("id", "assessment_id", "resource_id", "status", "lease_expires_at", "attempt_no").
		Where("scope = ? AND assessment_id IN ? AND deleted_at IS NULL", "evaluation_run", assessmentIDs).
		Order("assessment_id ASC, attempt_no DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[uint64]*evaluationconsistency.RunEvidence, len(assessmentIDs))
	for _, row := range rows {
		if _, exists := result[row.AssessmentID]; exists {
			continue
		}
		result[row.AssessmentID] = &evaluationconsistency.RunEvidence{
			ID: row.ResourceID, Status: row.Status, LeaseExpiresAt: row.LeaseExpiresAt,
		}
	}
	return result, nil
}

func (r *consistencyReadModel) listProjectionEvidence(ctx context.Context, assessmentIDs []uint64) (map[uint64]*evaluationconsistency.ProjectionEvidence, error) {
	var rows []struct {
		AssessmentID         uint64  `gorm:"column:assessment_id"`
		RowCount             int64   `gorm:"column:row_count"`
		UnlinkedRowCount     int64   `gorm:"column:unlinked_row_count"`
		DistinctOutcomeCount int64   `gorm:"column:distinct_outcome_count"`
		OutcomeID            *uint64 `gorm:"column:outcome_id"`
	}
	if err := r.db.WithContext(ctx).
		Table("assessment_score").
		Select(`assessment_id,
			COUNT(*) AS row_count,
			COALESCE(SUM(CASE WHEN evaluation_outcome_id IS NULL THEN 1 ELSE 0 END), 0) AS unlinked_row_count,
			COUNT(DISTINCT evaluation_outcome_id) AS distinct_outcome_count,
			MIN(evaluation_outcome_id) AS outcome_id`).
		Where("assessment_id IN ? AND deleted_at IS NULL", assessmentIDs).
		Group("assessment_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[uint64]*evaluationconsistency.ProjectionEvidence, len(rows))
	for _, row := range rows {
		evidence := &evaluationconsistency.ProjectionEvidence{
			RowCount: row.RowCount, UnlinkedRowCount: row.UnlinkedRowCount, DistinctOutcomeCount: row.DistinctOutcomeCount,
		}
		if row.OutcomeID != nil {
			evidence.OutcomeID = strconv.FormatUint(*row.OutcomeID, 10)
		}
		result[row.AssessmentID] = evidence
	}
	return result, nil
}

func (r *consistencyReadModel) listCommittedOutboxEvidence(ctx context.Context, assessmentIDs []uint64) (map[uint64]*evaluationconsistency.CommittedOutboxEvidence, error) {
	aggregateIDs := make([]string, 0, len(assessmentIDs))
	for _, assessmentID := range assessmentIDs {
		aggregateIDs = append(aggregateIDs, strconv.FormatUint(assessmentID, 10))
	}
	var rows []struct {
		ID          uint64 `gorm:"column:id"`
		AggregateID string `gorm:"column:aggregate_id"`
		PayloadJSON string `gorm:"column:payload_json"`
		Status      string `gorm:"column:status"`
	}
	if err := r.db.WithContext(ctx).
		Table("domain_event_outbox").
		Select("id", "aggregate_id", "payload_json", "status").
		Where("event_type = ? AND aggregate_type = ? AND aggregate_id IN ?", eventcatalog.EvaluationOutcomeCommitted, evalevent.AggregateType, aggregateIDs).
		Order("aggregate_id ASC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[uint64]*evaluationconsistency.CommittedOutboxEvidence, len(rows))
	for _, row := range rows {
		assessmentID, err := strconv.ParseUint(row.AggregateID, 10, 64)
		if err != nil {
			return nil, err
		}
		evidence := result[assessmentID]
		if evidence == nil {
			evidence = &evaluationconsistency.CommittedOutboxEvidence{Status: row.Status}
			var envelope struct {
				Data struct {
					OutcomeID       string `json:"outcome_id"`
					EvaluationRunID string `json:"evaluation_run_id"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(row.PayloadJSON), &envelope); err != nil {
				return nil, err
			}
			evidence.OutcomeID = envelope.Data.OutcomeID
			evidence.RunID = envelope.Data.EvaluationRunID
			result[assessmentID] = evidence
		}
		evidence.RowCount++
	}
	return result, nil
}

var _ evaluationconsistency.Reader = (*consistencyReadModel)(nil)
