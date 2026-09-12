package statistics

import (
	"context"
	"database/sql"
	"fmt"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"gorm.io/gorm"
	"time"
)

func (s *ReadStore) ScopedContentBatch(ctx context.Context, orgID int64, asOf time.Time, refs []app.ScopedContentRef) ([]app.ContentItem, error) {
	if orgID <= 0 || len(refs) == 0 || len(refs) > 100 {
		return nil, fmt.Errorf("invalid scoped content batch")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	items := make([]app.ContentItem, 0, len(refs))
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reader := &ReadStore{db: tx}
		for _, ref := range refs {
			item := app.ContentItem{Kind: ref.Kind, Code: ref.Code, HasCompletion: ref.Kind != "questionnaire"}
			query := reader.scopedFacts(ctx, orgID, ref.Stores, "statistics_assessment_fact").Where("f.stat_date<=?", asOf)
			if ref.Kind == "questionnaire" {
				query = query.Where("f.questionnaire_code=?", ref.Code).Select("COALESCE(SUM(fact_type='answersheet_submitted'),0) total_submissions")
			} else {
				query = query.Where("f.model_kind=? AND f.model_code=?", ref.Kind, ref.Code).Select("COALESCE(SUM(fact_type='assessment_created'),0) total_submissions,COALESCE(SUM(fact_type='outcome_committed'),0) total_completions")
			}
			if err := query.Scan(&item).Error; err != nil {
				return err
			}
			if item.HasCompletion && item.TotalSubmissions > 0 {
				item.CompletionRate = float64(item.TotalCompletions) * 100 / float64(item.TotalSubmissions)
			}
			items = append(items, item)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return items, nil
}
