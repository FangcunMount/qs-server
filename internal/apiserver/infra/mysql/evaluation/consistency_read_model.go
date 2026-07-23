package evaluation

import (
	"context"
	"encoding/json"
	"strconv"

	evalevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"gorm.io/gorm"
)

type consistencyReadModel struct {
	db *gorm.DB
}

func NewConsistencyReadModel(db *gorm.DB) evaluationconsistency.Reader {
	return &consistencyReadModel{db: db}
}

func (r *consistencyReadModel) FindProjectionEvidence(ctx context.Context, assessmentID uint64) (*evaluationconsistency.ProjectionEvidence, error) {
	var row struct {
		RowCount             int64   `gorm:"column:row_count"`
		UnlinkedRowCount     int64   `gorm:"column:unlinked_row_count"`
		DistinctOutcomeCount int64   `gorm:"column:distinct_outcome_count"`
		OutcomeID            *uint64 `gorm:"column:outcome_id"`
	}
	err := r.db.WithContext(ctx).
		Table("assessment_score").
		Select(`
			COUNT(*) AS row_count,
			COALESCE(SUM(CASE WHEN evaluation_outcome_id IS NULL THEN 1 ELSE 0 END), 0) AS unlinked_row_count,
			COUNT(DISTINCT evaluation_outcome_id) AS distinct_outcome_count,
			MIN(evaluation_outcome_id) AS outcome_id`).
		Where("assessment_id = ? AND deleted_at IS NULL", assessmentID).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.RowCount == 0 {
		return nil, nil
	}
	evidence := &evaluationconsistency.ProjectionEvidence{
		RowCount:             row.RowCount,
		UnlinkedRowCount:     row.UnlinkedRowCount,
		DistinctOutcomeCount: row.DistinctOutcomeCount,
	}
	if row.OutcomeID != nil {
		evidence.OutcomeID = strconv.FormatUint(*row.OutcomeID, 10)
	}
	return evidence, nil
}

func (r *consistencyReadModel) FindCommittedOutboxEvidence(ctx context.Context, assessmentID uint64) (*evaluationconsistency.CommittedOutboxEvidence, error) {
	query := r.db.WithContext(ctx).
		Table("domain_event_outbox").
		Where(
			"event_type = ? AND aggregate_type = ? AND aggregate_id = ?",
			eventcatalog.EvaluationOutcomeCommitted,
			evalevent.AggregateType,
			strconv.FormatUint(assessmentID, 10),
		)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	var row struct {
		PayloadJSON string `gorm:"column:payload_json"`
		Status      string `gorm:"column:status"`
	}
	if err := query.Select("payload_json", "status").Order("id DESC").Take(&row).Error; err != nil {
		return nil, err
	}
	var envelope struct {
		Data struct {
			OutcomeID       string `json:"outcome_id"`
			EvaluationRunID string `json:"evaluation_run_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(row.PayloadJSON), &envelope); err != nil {
		return nil, err
	}
	return &evaluationconsistency.CommittedOutboxEvidence{
		RowCount:  count,
		OutcomeID: envelope.Data.OutcomeID,
		RunID:     envelope.Data.EvaluationRunID,
		Status:    row.Status,
	}, nil
}

var _ evaluationconsistency.Reader = (*consistencyReadModel)(nil)
