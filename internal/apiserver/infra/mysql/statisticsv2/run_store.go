package statisticsv2

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	appv2 "github.com/FangcunMount/qs-server/internal/apiserver/application/statisticsv2"
	domainv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

type runPO struct {
	ID                                                 uint64 `gorm:"primaryKey"`
	OrgID                                              int64
	BatchKey                                           string
	Attempt                                            uint32
	TriggerType                                        string
	WindowStart, WindowEnd, AsOfDate                   time.Time
	Status                                             string
	Stage                                              string
	SourceCountsJSON, FactCountsJSON, ResultCountsJSON []byte
	OperatorID                                         *uint64
	Reason                                             string
	StartedAt                                          time.Time
	DataCommittedAt, FinishedAt                        *time.Time
	ErrorCode, ErrorMessage                            string
}

func (runPO) TableName() string { return "statistics_sync_run" }

type RunStore struct{ db *gorm.DB }

func NewRunStore(db *gorm.DB) *RunStore { return &RunStore{db} }
func (s *RunStore) dbFor(ctx context.Context) *gorm.DB {
	if tx, ok := mysql.TxFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return s.db.WithContext(ctx)
}
func (s *RunStore) Create(ctx context.Context, in appv2.Run) (*appv2.Run, error) {
	var max uint32
	s.dbFor(ctx).Table("statistics_sync_run").Where("batch_key=?", in.BatchKey).Select("COALESCE(MAX(attempt),0)").Scan(&max)
	var operatorID *uint64
	if in.OperatorID != 0 {
		value := in.OperatorID
		operatorID = &value
	}
	po := runPO{ID: meta.New().Uint64(), OrgID: in.OrgID, BatchKey: in.BatchKey, Attempt: max + 1, TriggerType: in.TriggerType, WindowStart: in.Window.From, WindowEnd: in.Window.To, AsOfDate: in.AsOfDate, Status: string(in.Status), Stage: in.Stage, OperatorID: operatorID, Reason: in.Reason, StartedAt: in.StartedAt}
	if err := s.dbFor(ctx).Create(&po).Error; err != nil {
		return nil, err
	}
	return fromRunPO(po), nil
}
func (s *RunStore) UpdateProgress(ctx context.Context, id uint64, stage string, sources, facts, results map[string]int64) error {
	values := map[string]any{"stage": stage}
	if sources != nil {
		values["source_counts_json"], _ = json.Marshal(sources)
	}
	if facts != nil {
		values["fact_counts_json"], _ = json.Marshal(facts)
	}
	if results != nil {
		values["result_counts_json"], _ = json.Marshal(results)
	}
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(values).Error
}
func (s *RunStore) MarkDataCommitted(ctx context.Context, id uint64, at time.Time) error {
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(map[string]any{"status": domainv2.RunStatusDataCommitted, "stage": "cache_switch", "data_committed_at": at}).Error
}
func (s *RunStore) MarkSucceeded(ctx context.Context, id uint64, at time.Time) error {
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(map[string]any{"status": domainv2.RunStatusSucceeded, "stage": "completed", "finished_at": at}).Error
}
func (s *RunStore) MarkFailed(ctx context.Context, id uint64, stage, code, message string, at time.Time) error {
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(map[string]any{"status": domainv2.RunStatusFailed, "stage": stage, "error_code": code, "error_message": message, "finished_at": at}).Error
}
func (s *RunStore) Get(ctx context.Context, id uint64) (*appv2.Run, error) {
	var po runPO
	if err := s.dbFor(ctx).First(&po, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return fromRunPO(po), nil
}
func (s *RunStore) List(ctx context.Context, orgID int64, limit int) ([]appv2.Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []runPO
	if err := s.dbFor(ctx).Where("org_id=?", orgID).Order("started_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]appv2.Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, *fromRunPO(row))
	}
	return out, nil
}
func fromRunPO(po runPO) *appv2.Run {
	r := &appv2.Run{ID: po.ID, OrgID: po.OrgID, BatchKey: po.BatchKey, Attempt: po.Attempt, TriggerType: po.TriggerType, Window: domainv2.InstantRange{From: po.WindowStart, To: po.WindowEnd}, AsOfDate: po.AsOfDate, Status: domainv2.RunStatus(po.Status), Stage: po.Stage, Reason: po.Reason, StartedAt: po.StartedAt, DataCommittedAt: po.DataCommittedAt, FinishedAt: po.FinishedAt, ErrorCode: po.ErrorCode, ErrorMessage: po.ErrorMessage}
	if po.OperatorID != nil {
		r.OperatorID = *po.OperatorID
	}
	_ = json.Unmarshal(po.SourceCountsJSON, &r.SourceCounts)
	_ = json.Unmarshal(po.FactCountsJSON, &r.FactCounts)
	_ = json.Unmarshal(po.ResultCountsJSON, &r.ResultCounts)
	return r
}
