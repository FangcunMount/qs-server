package main

import (
	"context"
	"fmt"
	"strings"

	planMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"gorm.io/gorm"
)

func seedStatisticsBackfill(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for statistics_backfill")
	}

	deps.Logger.Infow("Statistics backfill started", "org_id", deps.Config.Global.OrgID)

	if err := deps.APIClient.SyncStatisticsDaily(ctx); err != nil {
		return err
	}
	if err := deps.APIClient.SyncStatisticsAccumulated(ctx); err != nil {
		return err
	}
	if err := deps.APIClient.SyncStatisticsPlan(ctx); err != nil {
		return err
	}
	if err := deps.APIClient.ValidateStatisticsConsistency(ctx); err != nil {
		return err
	}

	if err := warmStatisticsReads(ctx, deps); err != nil {
		return err
	}

	deps.Logger.Infow("Statistics backfill completed", "org_id", deps.Config.Global.OrgID)
	return nil
}

func warmStatisticsReads(ctx context.Context, deps *dependencies) error {
	paths := []string{
		"/api/v1/statistics/overview?preset=30d",
		"/api/v1/statistics/clinicians?preset=30d&page=1&page_size=20",
		"/api/v1/statistics/entries?preset=30d&page=1&page_size=20",
	}
	for _, path := range paths {
		if _, err := deps.APIClient.doRequest(ctx, "GET", path, nil); err != nil {
			return fmt.Errorf("warm statistics path %s: %w", path, err)
		}
	}

	testees, err := deps.APIClient.ListTesteesByOrg(ctx, deps.Config.Global.OrgID, 1, 1)
	if err != nil {
		return fmt.Errorf("warm periodic statistics by loading testees: %w", err)
	}
	if testees != nil && len(testees.Items) > 0 && testees.Items[0] != nil {
		testeeID := strings.TrimSpace(testees.Items[0].ID)
		if testeeID != "" {
			if _, err := deps.APIClient.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/statistics/testees/%s/periodic?preset=30d", testeeID), nil); err != nil {
				return fmt.Errorf("warm testee periodic statistics for %s: %w", testeeID, err)
			}
		}
	}

	planID, err := discoverWarmPlanID(ctx, deps.Config.Local.MySQLDSN, deps.Config.Global.OrgID)
	if err != nil {
		return err
	}
	if planID != "" {
		if _, err := deps.APIClient.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/statistics/plans/%s?preset=30d", planID), nil); err != nil {
			return fmt.Errorf("warm plan statistics for %s: %w", planID, err)
		}
	}
	return nil
}

func discoverWarmPlanID(ctx context.Context, mysqlDSN string, orgID int64) (string, error) {
	if strings.TrimSpace(mysqlDSN) == "" {
		return "", nil
	}
	mysqlDB, err := openLocalSeedMySQL(mysqlDSN)
	if err != nil {
		return "", err
	}
	defer closeLocalSeedMySQL(mysqlDB)

	var row struct {
		ID uint64 `gorm:"column:id"`
	}
	err = mysqlDB.WithContext(ctx).
		Table((planMySQL.AssessmentPlanPO{}).TableName()).
		Select("id").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Order("created_at ASC, id ASC").
		Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", fmt.Errorf("discover warm plan id: %w", err)
	}
	if row.ID == 0 {
		return "", nil
	}
	return fmt.Sprintf("%d", row.ID), nil
}
