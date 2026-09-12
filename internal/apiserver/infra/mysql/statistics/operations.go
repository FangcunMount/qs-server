package statistics

import (
	"context"
	"fmt"
	errors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"time"
)

func (s *ReadStore) OperationsCoverage(ctx context.Context, org int64, run uint64, from, to time.Time) ([]domain.InstantRange, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var rows []struct{ WindowStart, WindowEnd time.Time }
	err = s.db.WithContext(ctx).Table("statistics_sync_run").Select("window_start,window_end").Where("org_id=? AND id<=? AND run_mode IN ('publish','repair') AND status IN ('succeeded','data_committed') AND window_start<? AND window_end>?", org, run, to, from).Where("JSON_EXTRACT(result_counts_json,'$.store_activity_daily') IS NOT NULL").Order("window_start").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.InstantRange, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.InstantRange{From: row.WindowStart, To: row.WindowEnd})
	}
	return result, nil
}
func (s *ReadStore) OperationsPopulation(ctx context.Context, org int64, stores authz.StoreRange) ([]app.OperationStore, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	query := s.db.WithContext(ctx).Table("actor_stores st").Select("st.id,st.code,st.name,st.is_active,COUNT(t.id) AS current_service_count").Joins("LEFT JOIN testee t ON t.org_id=st.org_id AND t.store_id=st.id AND t.deleted_at IS NULL").Where("st.org_id=?", org)
	if !stores.AllStores {
		if len(stores.StoreIDs) == 0 {
			return nil, fmt.Errorf("empty store scope")
		}
		query = query.Where("st.id IN ?", stores.StoreIDs)
	}
	var rows []app.OperationStore
	err = query.Group("st.id,st.code,st.name,st.is_active").Order("st.code,st.id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if !stores.AllStores && len(rows) != len(stores.StoreIDs) {
		return nil, errors.WithCode(code.ErrPermissionDenied, "门店不在当前公司可见范围")
	}
	if rows == nil {
		rows = []app.OperationStore{}
	}
	return rows, nil
}
func (s *ReadStore) OperationsActivity(ctx context.Context, org int64, stores authz.StoreRange, from, to time.Time) ([]app.ActivityRow, error) {
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	query := s.db.WithContext(ctx).Table("statistics_store_activity_daily").Select("conducting_store_id AS store_id,unknown_reason,stat_date AS date,answersheet_submitted_count AS submissions,assessment_completed_count AS completions").Where("org_id=? AND stat_date>=? AND stat_date<?", org, from, to)
	if !stores.AllStores {
		if len(stores.StoreIDs) == 0 {
			return nil, fmt.Errorf("empty store scope")
		}
		query = query.Where("conducting_store_id IN ?", stores.StoreIDs)
	}
	var rows []app.ActivityRow
	err = query.Order("stat_date,conducting_store_id,unknown_reason").Scan(&rows).Error
	if rows == nil {
		rows = []app.ActivityRow{}
	}
	return rows, err
}
