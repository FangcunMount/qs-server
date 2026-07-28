package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const dateLayout = "2006-01-02"

type options struct {
	Mode, MySQLDSN, BatchID, From, To, BaselinePath, OutputPath string
	OrgID                                                       int64
}

type dailyFactCount struct {
	Date     string `json:"date"`
	FactType string `json:"fact_type"`
	Count    int64  `json:"count"`
}

type baseline struct {
	Version    int              `json:"version"`
	OrgID      int64            `json:"org_id"`
	From       string           `json:"from"`
	To         string           `json:"to"`
	CapturedAt time.Time        `json:"captured_at"`
	Counts     []dailyFactCount `json:"counts"`
}

type exactFactCount struct {
	Date     string `json:"date"`
	FactType string `json:"fact_type"`
	Expected int64  `json:"expected"`
	Matched  int64  `json:"matched"`
}

type deltaCheck struct {
	Date              string `json:"date"`
	FactType          string `json:"fact_type"`
	Baseline          int64  `json:"baseline"`
	Current           int64  `json:"current"`
	Delta             int64  `json:"delta"`
	BatchExpected     int64  `json:"batch_expected"`
	UnattributedDelta int64  `json:"unattributed_delta"`
}

type verification struct {
	Version     int              `json:"version"`
	BatchID     string           `json:"batch_id"`
	OrgID       int64            `json:"org_id"`
	VerifiedAt  time.Time        `json:"verified_at"`
	Complete    bool             `json:"complete"`
	ExactFacts  []exactFactCount `json:"exact_facts"`
	DeltaChecks []deltaCheck     `json:"delta_checks"`
	Problems    []string         `json:"problems,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "verify historical statistics:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("verify_historical_statistics", flag.ContinueOnError)
	flags.SetOutput(output)
	cfg := options{}
	flags.StringVar(&cfg.Mode, "mode", "verify", "capture-baseline or verify")
	flags.StringVar(&cfg.MySQLDSN, "mysql-dsn", os.Getenv("QS_MYSQL_DSN"), "MySQL DSN or QS_MYSQL_DSN")
	flags.Int64Var(&cfg.OrgID, "org-id", 0, "organization ID")
	flags.StringVar(&cfg.BatchID, "batch-id", "", "historical seed batch ID")
	flags.StringVar(&cfg.From, "from", "", "first Shanghai business date")
	flags.StringVar(&cfg.To, "to", "", "last Shanghai business date")
	flags.StringVar(&cfg.BaselinePath, "baseline", "", "captured baseline JSON (verify mode)")
	flags.StringVar(&cfg.OutputPath, "output", "", "write JSON to this path; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect mysql: %w", err)
	}

	switch cfg.Mode {
	case "capture-baseline":
		counts, err := loadCurrentFactCounts(ctx, db, cfg.OrgID, cfg.From, cfg.To)
		if err != nil {
			return err
		}
		result := baseline{Version: 1, OrgID: cfg.OrgID, From: cfg.From, To: cfg.To, CapturedAt: time.Now().UTC(), Counts: counts}
		return writeJSON(output, cfg.OutputPath, result)
	case "verify":
		base, err := loadBaseline(cfg.BaselinePath)
		if err != nil {
			return err
		}
		if base.OrgID != cfg.OrgID || base.From != cfg.From || base.To != cfg.To {
			return errors.New("baseline org/date range does not match verification request")
		}
		exact, err := loadExactBatchFactCounts(ctx, db, cfg.OrgID, cfg.BatchID)
		if err != nil {
			return err
		}
		current, err := loadCurrentFactCounts(ctx, db, cfg.OrgID, cfg.From, cfg.To)
		if err != nil {
			return err
		}
		result := reconcile(cfg, base, exact, current)
		if err := writeJSON(output, cfg.OutputPath, result); err != nil {
			return err
		}
		if !result.Complete {
			return fmt.Errorf("batch %s statistics verification failed with %d problem(s)", cfg.BatchID, len(result.Problems))
		}
		return nil
	default:
		return fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
}

func (o *options) validate() error {
	if o == nil {
		return errors.New("options are required")
	}
	o.Mode, o.MySQLDSN, o.BatchID = strings.TrimSpace(o.Mode), strings.TrimSpace(o.MySQLDSN), strings.TrimSpace(o.BatchID)
	o.From, o.To = strings.TrimSpace(o.From), strings.TrimSpace(o.To)
	o.BaselinePath, o.OutputPath = strings.TrimSpace(o.BaselinePath), strings.TrimSpace(o.OutputPath)
	if o.Mode != "capture-baseline" && o.Mode != "verify" {
		return errors.New("mode must be capture-baseline or verify")
	}
	if o.MySQLDSN == "" || o.OrgID <= 0 {
		return errors.New("mysql-dsn and positive org-id are required")
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	from, err := time.ParseInLocation(dateLayout, o.From, location)
	if err != nil {
		return fmt.Errorf("invalid from date: %w", err)
	}
	to, err := time.ParseInLocation(dateLayout, o.To, location)
	if err != nil || to.Before(from) {
		return errors.New("invalid inclusive to date")
	}
	if o.Mode == "verify" && (o.BatchID == "" || o.BaselinePath == "") {
		return errors.New("verify mode requires batch-id and baseline")
	}
	return nil
}

func loadCurrentFactCounts(ctx context.Context, db *sql.DB, orgID int64, from, to string) ([]dailyFactCount, error) {
	var result []dailyFactCount
	for _, table := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact"} {
		query := `SELECT DATE_FORMAT(stat_date, '%Y-%m-%d'), fact_type, COUNT(*)
FROM ` + table + ` WHERE org_id=? AND stat_date>=? AND stat_date<=? GROUP BY stat_date,fact_type ORDER BY stat_date,fact_type`
		rows, err := db.QueryContext(ctx, query, orgID, from, to)
		if err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		for rows.Next() {
			var item dailyFactCount
			if err := rows.Scan(&item.Date, &item.FactType, &item.Count); err != nil {
				_ = rows.Close()
				return nil, err
			}
			result = append(result, item)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return aggregateCounts(result), nil
}

func loadExactBatchFactCounts(ctx context.Context, db *sql.DB, orgID int64, batchID string) ([]exactFactCount, error) {
	const query = `
SELECT business_date,fact_type,SUM(expected_count),SUM(matched_count) FROM (
  SELECT DATE_FORMAT(s.business_at,'%Y-%m-%d') business_date,
         CASE s.stage WHEN 'entry_resolve' THEN 'entry_opened' ELSE 'intake_confirmed' END fact_type,
         COUNT(*) expected_count, COALESCE(SUM(f.id IS NOT NULL),0) matched_count
  FROM seed_backfill_stage s
  LEFT JOIN statistics_access_fact f ON f.org_id=s.org_id
    AND f.fact_type=CASE s.stage WHEN 'entry_resolve' THEN 'entry_opened' ELSE 'intake_confirmed' END
    AND f.source_type=CASE s.stage WHEN 'entry_resolve' THEN 'entry_resolve' ELSE 'entry_intake' END
    AND f.source_ref=CASE s.stage WHEN 'entry_resolve' THEN JSON_UNQUOTE(JSON_EXTRACT(s.payload_json,'$.resolve_log_id')) ELSE JSON_UNQUOTE(JSON_EXTRACT(s.payload_json,'$.intake_log_id')) END
    AND f.stat_date=DATE(s.business_at)
  WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.stage IN ('entry_resolve','entry_intake')
  GROUP BY business_date,fact_type
  UNION ALL
  SELECT DATE_FORMAT(s.business_at,'%Y-%m-%d'),
         CASE s.stage WHEN 'plan_enrollment' THEN 'enrollment_joined' WHEN 'task_open' THEN 'task_opened' ELSE 'task_completed' END,
         COUNT(*), COALESCE(SUM(f.id IS NOT NULL),0)
  FROM seed_backfill_stage s
  LEFT JOIN statistics_plan_fact f ON f.org_id=s.org_id
    AND f.fact_type=CASE s.stage WHEN 'plan_enrollment' THEN 'enrollment_joined' WHEN 'task_open' THEN 'task_opened' ELSE 'task_completed' END
    AND f.source_type=CASE s.stage WHEN 'plan_enrollment' THEN 'plan_enrollment' ELSE 'assessment_task' END
    AND f.source_ref=s.resource_id AND f.stat_date=DATE(s.business_at)
  WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.stage IN ('plan_enrollment','task_open','task_complete')
  GROUP BY business_date,fact_type
  UNION ALL
  SELECT DATE_FORMAT(s.business_at,'%Y-%m-%d'),'task_created',COUNT(*),COALESCE(SUM(f.id IS NOT NULL),0)
  FROM seed_backfill_stage s
  JOIN JSON_TABLE(s.payload_json,'$.task_ids[*]' COLUMNS(task_id VARCHAR(128) PATH '$')) tasks
  LEFT JOIN statistics_plan_fact f ON f.org_id=s.org_id AND f.fact_type='task_created'
    AND f.source_type='assessment_task' AND f.source_ref=tasks.task_id AND f.stat_date=DATE(s.business_at)
  WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.stage='plan_enrollment'
  GROUP BY business_date
  UNION ALL
  SELECT DATE_FORMAT(s.business_at,'%Y-%m-%d'),
         CASE s.stage WHEN 'answersheet_submit' THEN 'answersheet_submitted' WHEN 'assessment_created' THEN 'assessment_created' WHEN 'outcome_committed' THEN 'outcome_committed' ELSE 'report_generated' END,
         COUNT(*), COALESCE(SUM(f.id IS NOT NULL),0)
  FROM seed_backfill_stage s
  LEFT JOIN statistics_assessment_fact f ON f.org_id=s.org_id
    AND f.fact_type=CASE s.stage WHEN 'answersheet_submit' THEN 'answersheet_submitted' WHEN 'assessment_created' THEN 'assessment_created' WHEN 'outcome_committed' THEN 'outcome_committed' ELSE 'report_generated' END
    AND f.source_type=CASE s.stage WHEN 'answersheet_submit' THEN 'answersheet' WHEN 'assessment_created' THEN 'assessment' WHEN 'outcome_committed' THEN 'evaluation_outcome' ELSE 'interpret_report' END
    AND f.source_ref=s.resource_id AND f.stat_date=DATE(s.business_at)
  WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.stage IN ('answersheet_submit','assessment_created','outcome_committed','report_generated')
  GROUP BY business_date,fact_type
) matched GROUP BY business_date,fact_type ORDER BY business_date,fact_type`
	rows, err := db.QueryContext(ctx, query, orgID, batchID, orgID, batchID, orgID, batchID, orgID, batchID)
	if err != nil {
		return nil, fmt.Errorf("match batch facts: %w", err)
	}
	defer rows.Close()
	var result []exactFactCount
	for rows.Next() {
		var item exactFactCount
		if err := rows.Scan(&item.Date, &item.FactType, &item.Expected, &item.Matched); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func reconcile(cfg options, base baseline, exact []exactFactCount, current []dailyFactCount) verification {
	result := verification{Version: 1, BatchID: cfg.BatchID, OrgID: cfg.OrgID, VerifiedAt: time.Now().UTC(), Complete: true, ExactFacts: exact}
	baseMap, currentMap, expectedMap := countMap(base.Counts), countMap(current), map[string]int64{}
	for _, item := range exact {
		key := item.Date + "\x00" + item.FactType
		expectedMap[key] += item.Expected
		if item.Matched != item.Expected {
			result.Problems = append(result.Problems, fmt.Sprintf("%s %s exact facts matched=%d expected=%d", item.Date, item.FactType, item.Matched, item.Expected))
		}
	}
	for key, expected := range expectedMap {
		parts := strings.SplitN(key, "\x00", 2)
		before, now := baseMap[key], currentMap[key]
		check := deltaCheck{Date: parts[0], FactType: parts[1], Baseline: before, Current: now, Delta: now - before, BatchExpected: expected, UnattributedDelta: now - before - expected}
		result.DeltaChecks = append(result.DeltaChecks, check)
		if check.Delta < expected {
			result.Problems = append(result.Problems, fmt.Sprintf("%s %s fact delta=%d below batch expected=%d", check.Date, check.FactType, check.Delta, expected))
		}
	}
	sort.Slice(result.DeltaChecks, func(i, j int) bool {
		if result.DeltaChecks[i].Date == result.DeltaChecks[j].Date {
			return result.DeltaChecks[i].FactType < result.DeltaChecks[j].FactType
		}
		return result.DeltaChecks[i].Date < result.DeltaChecks[j].Date
	})
	result.Complete = len(result.Problems) == 0 && len(exact) > 0
	if len(exact) == 0 {
		result.Problems = append(result.Problems, "batch has no Statistics-relevant completed stages")
	}
	return result
}

func aggregateCounts(values []dailyFactCount) []dailyFactCount {
	counts := countMap(values)
	result := make([]dailyFactCount, 0, len(counts))
	for key, count := range counts {
		parts := strings.SplitN(key, "\x00", 2)
		result = append(result, dailyFactCount{Date: parts[0], FactType: parts[1], Count: count})
	}
	sortDailyCounts(result)
	return result
}

func countMap(values []dailyFactCount) map[string]int64 {
	result := make(map[string]int64, len(values))
	for _, item := range values {
		result[item.Date+"\x00"+item.FactType] += item.Count
	}
	return result
}

func sortDailyCounts(values []dailyFactCount) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && (values[j].Date < values[j-1].Date || values[j].Date == values[j-1].Date && values[j].FactType < values[j-1].FactType); j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func loadBaseline(path string) (baseline, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return baseline{}, err
	}
	var value baseline
	if err := json.Unmarshal(data, &value); err != nil {
		return baseline{}, err
	}
	if value.Version != 1 {
		return baseline{}, fmt.Errorf("unsupported baseline version %d", value.Version)
	}
	return value, nil
}

func writeJSON(output io.Writer, path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if strings.TrimSpace(path) == "" {
		_, err = output.Write(data)
		return err
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".historical-statistics-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
