package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"gorm.io/gorm"
)

const testeeCreatedAtFixupBatchSize = 1000

var (
	testeeCreatedAtFixupLocation    = time.FixedZone("CST", 8*60*60)
	testeeCreatedAtFixupRangeStart  = time.Date(2019, 3, 25, 0, 0, 0, 0, testeeCreatedAtFixupLocation)
	testeeCreatedAtFixupRangeEnd    = time.Date(2026, 4, 15, 23, 59, 59, 0, testeeCreatedAtFixupLocation)
	testeeCreatedAtFixupYearWeights = []testeeCreatedAtYearWeight{
		{Year: 2019, Weight: 5},
		{Year: 2020, Weight: 6},
		{Year: 2021, Weight: 11},
		{Year: 2022, Weight: 18},
		{Year: 2023, Weight: 22},
		{Year: 2024, Weight: 25},
		{Year: 2025, Weight: 13},
		{Year: 2026, Weight: 2},
	}
)

type testeeCreatedAtYearWeight struct {
	Year   int
	Weight int
}

type testeeCreatedAtFixupRow struct {
	ID        uint64    `gorm:"column:id"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

type testeeCreatedAtFixupStats struct {
	TesteesLoaded     int
	TesteesProcessed  int
	TesteesUpdated    int
	TesteesSkipped    int
	UpdatedAtAdjusted int
}

func seedTesteeFixupCreatedAt(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for testee_fixup_created_at")
	}
	if strings.TrimSpace(deps.Config.Local.MySQLDSN) == "" {
		return fmt.Errorf("seeddata local.mysql_dsn is required for testee_fixup_created_at")
	}

	mysqlDB, err := openLocalSeedMySQL(deps.Config.Local.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after testee created_at fixup", "error", closeErr.Error())
		}
	}()

	rows, err := loadTesteeCreatedAtFixupRows(ctx, mysqlDB, deps.Config.Global.OrgID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		deps.Logger.Infow("No eligible testees found for created_at fixup", "org_id", deps.Config.Global.OrgID)
		return nil
	}

	totalBatches := (len(rows) + testeeCreatedAtFixupBatchSize - 1) / testeeCreatedAtFixupBatchSize
	targets, allocation, err := buildWeightedTesteeCreatedAtTargets(len(rows), testeeCreatedAtFixupRangeStart, testeeCreatedAtFixupRangeEnd, testeeCreatedAtFixupYearWeights)
	if err != nil {
		return err
	}
	deps.Logger.Infow("Testee created_at fixup started",
		"org_id", deps.Config.Global.OrgID,
		"total_testees", len(rows),
		"total_batches", totalBatches,
		"range_start", testeeCreatedAtFixupRangeStart.Format(time.RFC3339),
		"range_end", testeeCreatedAtFixupRangeEnd.Format(time.RFC3339),
		"year_allocation", allocation,
		"batch_size", testeeCreatedAtFixupBatchSize,
	)

	batchProgress := newSeedProgressBar("testee_fixup batches", totalBatches)
	defer batchProgress.Close()
	taskProgress := newSeedProgressBar("testee_fixup tasks", len(rows))
	defer taskProgress.Close()

	stats, err := runTesteeCreatedAtFixup(ctx, mysqlDB, rows, targets, batchProgress, taskProgress)
	if err != nil {
		return err
	}
	batchProgress.Complete()
	taskProgress.Complete()

	deps.Logger.Infow("Testee created_at fixup completed",
		"org_id", deps.Config.Global.OrgID,
		"total_testees", stats.TesteesLoaded,
		"testees_processed", stats.TesteesProcessed,
		"testees_updated", stats.TesteesUpdated,
		"testees_skipped", stats.TesteesSkipped,
		"updated_at_adjusted", stats.UpdatedAtAdjusted,
		"range_start", testeeCreatedAtFixupRangeStart.Format(time.RFC3339),
		"range_end", testeeCreatedAtFixupRangeEnd.Format(time.RFC3339),
	)
	return nil
}

func runTesteeCreatedAtFixup(
	ctx context.Context,
	mysqlDB *gorm.DB,
	rows []testeeCreatedAtFixupRow,
	targets []time.Time,
	batchProgress *seedProgressBar,
	taskProgress *seedProgressBar,
) (*testeeCreatedAtFixupStats, error) {
	stats := &testeeCreatedAtFixupStats{TesteesLoaded: len(rows)}
	if len(targets) != len(rows) {
		return nil, fmt.Errorf("target timestamp count %d does not match testee count %d", len(targets), len(rows))
	}
	for start := 0; start < len(rows); start += testeeCreatedAtFixupBatchSize {
		end := start + testeeCreatedAtFixupBatchSize
		if end > len(rows) {
			end = len(rows)
		}

		tx := mysqlDB.WithContext(ctx).Begin()
		if tx.Error != nil {
			return nil, fmt.Errorf("begin testee created_at fixup transaction: %w", tx.Error)
		}

		for idx, row := range rows[start:end] {
			globalIndex := start + idx
			targetCreatedAt := targets[globalIndex]

			stats.TesteesProcessed++
			if row.CreatedAt.Equal(targetCreatedAt) && !row.UpdatedAt.Before(targetCreatedAt) {
				stats.TesteesSkipped++
				taskProgress.Increment()
				continue
			}
			if row.UpdatedAt.Before(targetCreatedAt) {
				stats.UpdatedAtAdjusted++
			}
			if err := updateTesteeCreatedAt(ctx, tx, row.ID, targetCreatedAt); err != nil {
				_ = tx.Rollback()
				return nil, err
			}
			stats.TesteesUpdated++
			taskProgress.Increment()
		}

		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("commit testee created_at fixup transaction: %w", err)
		}
		batchProgress.Increment()
	}
	return stats, nil
}

func loadTesteeCreatedAtFixupRows(ctx context.Context, mysqlDB *gorm.DB, orgID int64) ([]testeeCreatedAtFixupRow, error) {
	rows := make([]testeeCreatedAtFixupRow, 0, 1024)
	err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.TesteePO{}).TableName()).
		Select("id, created_at, updated_at").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load testees for created_at fixup: %w", err)
	}
	return rows, nil
}

func updateTesteeCreatedAt(ctx context.Context, mysqlDB *gorm.DB, testeeID uint64, createdAt time.Time) error {
	err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.TesteePO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", testeeID).
		Updates(map[string]interface{}{
			"created_at": createdAt,
			"updated_at": gorm.Expr("CASE WHEN updated_at < ? THEN ? ELSE updated_at END", createdAt, createdAt),
		}).Error
	if err != nil {
		return fmt.Errorf("update testee %d created_at: %w", testeeID, err)
	}
	return nil
}

func deriveEvenlyDistributedTimestamp(index, total int, start, end time.Time) (time.Time, error) {
	if total <= 0 {
		return time.Time{}, fmt.Errorf("total must be positive")
	}
	if index < 0 || index >= total {
		return time.Time{}, fmt.Errorf("index %d out of range for total %d", index, total)
	}
	if start.IsZero() || end.IsZero() {
		return time.Time{}, fmt.Errorf("start and end must be non-zero")
	}
	if end.Before(start) {
		return time.Time{}, fmt.Errorf("end %s is before start %s", end.Format(time.RFC3339), start.Format(time.RFC3339))
	}
	if total == 1 {
		return start.Round(0), nil
	}

	span := end.Sub(start)
	ratio := float64(index) / float64(total-1)
	offset := time.Duration(math.Round(float64(span) * ratio))
	return start.Add(offset).Round(0), nil
}

func buildWeightedTesteeCreatedAtTargets(
	total int,
	rangeStart, rangeEnd time.Time,
	weights []testeeCreatedAtYearWeight,
) ([]time.Time, map[int]int, error) {
	if total < 0 {
		return nil, nil, fmt.Errorf("total must be non-negative")
	}
	if total == 0 {
		return nil, map[int]int{}, nil
	}

	buckets, err := buildTesteeCreatedAtYearBuckets(rangeStart, rangeEnd, weights)
	if err != nil {
		return nil, nil, err
	}
	counts, err := allocateTesteeCreatedAtCounts(total, buckets)
	if err != nil {
		return nil, nil, err
	}

	targets := make([]time.Time, 0, total)
	allocation := make(map[int]int, len(buckets))
	for _, bucket := range buckets {
		count := counts[bucket.Year]
		allocation[bucket.Year] = count
		for idx := 0; idx < count; idx++ {
			target, err := deriveEvenlyDistributedTimestamp(idx, count, bucket.Start, bucket.End)
			if err != nil {
				return nil, nil, fmt.Errorf("derive weighted timestamp for year %d: %w", bucket.Year, err)
			}
			targets = append(targets, target)
		}
	}
	if len(targets) != total {
		return nil, nil, fmt.Errorf("weighted target count mismatch: got %d want %d", len(targets), total)
	}
	return targets, allocation, nil
}

type testeeCreatedAtYearBucket struct {
	Year   int
	Weight int
	Start  time.Time
	End    time.Time
}

func buildTesteeCreatedAtYearBuckets(
	rangeStart, rangeEnd time.Time,
	weights []testeeCreatedAtYearWeight,
) ([]testeeCreatedAtYearBucket, error) {
	if rangeStart.IsZero() || rangeEnd.IsZero() {
		return nil, fmt.Errorf("range start/end must be non-zero")
	}
	if rangeEnd.Before(rangeStart) {
		return nil, fmt.Errorf("range end %s is before start %s", rangeEnd.Format(time.RFC3339), rangeStart.Format(time.RFC3339))
	}
	if len(weights) == 0 {
		return nil, fmt.Errorf("year weights must not be empty")
	}

	buckets := make([]testeeCreatedAtYearBucket, 0, len(weights))
	for _, item := range weights {
		if item.Weight <= 0 {
			return nil, fmt.Errorf("year %d has non-positive weight %d", item.Year, item.Weight)
		}

		yearStart := time.Date(item.Year, time.January, 1, 0, 0, 0, 0, rangeStart.Location())
		yearEnd := time.Date(item.Year, time.December, 31, 23, 59, 59, 0, rangeStart.Location())
		if item.Year == rangeStart.Year() && rangeStart.After(yearStart) {
			yearStart = rangeStart
		}
		if item.Year == rangeEnd.Year() && rangeEnd.Before(yearEnd) {
			yearEnd = rangeEnd
		}
		if yearEnd.Before(yearStart) {
			return nil, fmt.Errorf("invalid year bucket %d: end %s before start %s", item.Year, yearEnd.Format(time.RFC3339), yearStart.Format(time.RFC3339))
		}
		buckets = append(buckets, testeeCreatedAtYearBucket{
			Year:   item.Year,
			Weight: item.Weight,
			Start:  yearStart,
			End:    yearEnd,
		})
	}

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Year < buckets[j].Year
	})
	return buckets, nil
}

func allocateTesteeCreatedAtCounts(total int, buckets []testeeCreatedAtYearBucket) (map[int]int, error) {
	if total < 0 {
		return nil, fmt.Errorf("total must be non-negative")
	}
	if len(buckets) == 0 {
		return nil, fmt.Errorf("buckets must not be empty")
	}

	totalWeight := 0
	for _, bucket := range buckets {
		totalWeight += bucket.Weight
	}
	if totalWeight <= 0 {
		return nil, fmt.Errorf("total weight must be positive")
	}

	type remainderItem struct {
		Year      int
		BaseCount int
		Remainder float64
	}

	items := make([]remainderItem, 0, len(buckets))
	allocated := 0
	for _, bucket := range buckets {
		exact := float64(total) * float64(bucket.Weight) / float64(totalWeight)
		base := int(math.Floor(exact))
		items = append(items, remainderItem{
			Year:      bucket.Year,
			BaseCount: base,
			Remainder: exact - float64(base),
		})
		allocated += base
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Remainder == items[j].Remainder {
			return items[i].Year < items[j].Year
		}
		return items[i].Remainder > items[j].Remainder
	})

	remaining := total - allocated
	for idx := 0; idx < remaining; idx++ {
		items[idx%len(items)].BaseCount++
	}

	counts := make(map[int]int, len(items))
	for _, item := range items {
		counts[item.Year] = item.BaseCount
	}
	return counts, nil
}
