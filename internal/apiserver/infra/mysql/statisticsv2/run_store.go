package statisticsv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	appv2 "github.com/FangcunMount/qs-server/internal/apiserver/application/statisticsv2"
	domainv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type runPO struct {
	ID                                                 uint64 `gorm:"primaryKey"`
	OrgID                                              int64
	BatchKey                                           string
	Attempt                                            uint32
	TriggerType                                        string
	RunMode                                            string
	WindowStart, WindowEnd, AsOfDate                   time.Time
	CacheGeneration                                    int64
	CachePublishedAt                                   *time.Time
	Status                                             string
	Stage                                              string
	SourceCountsJSON, FactCountsJSON, ResultCountsJSON []byte
	OperatorID                                         *uint64
	Reason                                             string
	StartedAt                                          time.Time
	DataCommittedAt, FinishedAt                        *time.Time
	ErrorCode, ErrorMessage                            string
	CacheResumeCount                                   uint32
	LastCacheResumeOperatorID                          *uint64
	LastCacheResumeReason                              string
	LastCacheResumeAt                                  *time.Time
	LastCacheResumeStatus                              string
	CacheResumeAuditJSON                               []byte
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
	var operatorID *uint64
	if in.OperatorID != 0 {
		value := in.OperatorID
		operatorID = &value
	}
	for retry := 0; retry < 8; retry++ {
		var latestAttempt uint32
		if err := s.dbFor(ctx).Table("statistics_sync_run").Where("batch_key=?", in.BatchKey).Select("COALESCE(MAX(attempt),0)").Scan(&latestAttempt).Error; err != nil {
			return nil, err
		}
		po := runPO{ID: meta.New().Uint64(), OrgID: in.OrgID, BatchKey: in.BatchKey, Attempt: latestAttempt + 1, TriggerType: in.TriggerType, RunMode: string(in.Mode), WindowStart: in.Window.From, WindowEnd: in.Window.To, AsOfDate: in.AsOfDate, Status: string(in.Status), Stage: in.Stage, OperatorID: operatorID, Reason: in.Reason, StartedAt: in.StartedAt}
		if err := s.dbFor(ctx).Create(&po).Error; err != nil {
			if mysql.IsDuplicateError(err) {
				continue
			}
			return nil, err
		}
		return fromRunPO(po), nil
	}
	return nil, fmt.Errorf("allocate statistics run attempt for batch %q after concurrent retries", in.BatchKey)
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
func (s *RunStore) AssertPublishable(ctx context.Context, orgID int64, target time.Time) error {
	var row struct{ AsOfDate time.Time }
	err := s.dbFor(ctx).Table("statistics_v2_org_snapshot").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("as_of_date").Where("org_id = ?", orgID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	current := domainv2.BusinessDate(row.AsOfDate)
	target = domainv2.BusinessDate(target)
	if current.After(target) {
		return fmt.Errorf("published watermark regression: current=%s target=%s", current.Format("2006-01-02"), target.Format("2006-01-02"))
	}
	return nil
}
func (s *RunStore) MarkDataCommitted(ctx context.Context, id uint64, at time.Time) error {
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(map[string]any{"status": domainv2.RunStatusDataCommitted, "stage": "data_committed", "data_committed_at": at}).Error
}
func (s *RunStore) MarkCachePublished(ctx context.Context, id uint64, generation int64, at time.Time) error {
	if generation <= 0 {
		return fmt.Errorf("cache generation must be positive")
	}
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(map[string]any{
		"cache_generation": generation, "cache_published_at": at,
		"error_code": "", "error_message": "",
	}).Error
}
func (s *RunStore) MarkCachePublishFailed(ctx context.Context, id uint64, generation int64, message string, at time.Time) error {
	values := map[string]any{
		"status": domainv2.RunStatusDataCommitted, "stage": "publishing_cache",
		"error_code": "cache_publish_failed", "error_message": truncateRunText(message, 1000),
	}
	if generation > 0 {
		values["cache_generation"] = generation
		values["cache_published_at"] = at
	}
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(values).Error
}
func (s *RunStore) RecordCacheResume(ctx context.Context, id uint64, operatorID uint64, reason, status string, generation int64, at time.Time) error {
	values := map[string]any{
		"cache_resume_count":       gorm.Expr("cache_resume_count + 1"),
		"last_cache_resume_reason": reason, "last_cache_resume_at": at,
		"last_cache_resume_status": status,
		"cache_resume_audit_json": gorm.Expr(
			"JSON_ARRAY_APPEND(COALESCE(cache_resume_audit_json, JSON_ARRAY()), '$', JSON_OBJECT('operator_id', ?, 'reason', ?, 'status', ?, 'generation', ?, 'occurred_at', ?))",
			operatorID, reason, status, generation, at.Format(time.RFC3339Nano),
		),
	}
	if operatorID != 0 {
		values["last_cache_resume_operator_id"] = operatorID
	}
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(values).Error
}
func (s *RunStore) MarkSucceeded(ctx context.Context, id uint64, at time.Time) error {
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(map[string]any{"status": domainv2.RunStatusSucceeded, "stage": "completed", "finished_at": at}).Error
}
func (s *RunStore) MarkFailed(ctx context.Context, id uint64, stage, code, message string, at time.Time) error {
	return s.dbFor(ctx).Table("statistics_sync_run").Where("id=?", id).Updates(map[string]any{"status": domainv2.RunStatusFailed, "stage": stage, "error_code": code, "error_message": truncateRunText(message, 1000), "finished_at": at}).Error
}

func truncateRunText(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
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
	mode := domainv2.RunMode(po.RunMode)
	if mode == "" {
		mode = domainv2.RunModePublish
	}
	r := &appv2.Run{ID: po.ID, OrgID: po.OrgID, BatchKey: po.BatchKey, Attempt: po.Attempt, TriggerType: po.TriggerType, Mode: mode, Window: domainv2.InstantRange{From: po.WindowStart, To: po.WindowEnd}, AsOfDate: po.AsOfDate, CacheGeneration: po.CacheGeneration, CachePublishedAt: po.CachePublishedAt, Status: domainv2.RunStatus(po.Status), Stage: po.Stage, Reason: po.Reason, StartedAt: po.StartedAt, DataCommittedAt: po.DataCommittedAt, FinishedAt: po.FinishedAt, ErrorCode: po.ErrorCode, ErrorMessage: po.ErrorMessage}
	r.CacheResumeCount = po.CacheResumeCount
	r.LastCacheResumeReason = po.LastCacheResumeReason
	r.LastCacheResumeAt = po.LastCacheResumeAt
	r.LastCacheResumeStatus = po.LastCacheResumeStatus
	if po.LastCacheResumeOperatorID != nil {
		r.LastCacheResumeOperatorID = *po.LastCacheResumeOperatorID
	}
	if po.OperatorID != nil {
		r.OperatorID = *po.OperatorID
	}
	_ = json.Unmarshal(po.SourceCountsJSON, &r.SourceCounts)
	_ = json.Unmarshal(po.FactCountsJSON, &r.FactCounts)
	_ = json.Unmarshal(po.ResultCountsJSON, &r.ResultCounts)
	return r
}
