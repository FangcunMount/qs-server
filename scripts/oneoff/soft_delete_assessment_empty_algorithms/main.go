// soft_delete_assessment_empty_algorithms soft-deletes Assessment rows whose
// evaluation_model_algorithm is NULL or empty. Default dry-run.
//
// Also removes evaluation_outcome rows with empty/NULL model_algorithm
// (outcome table has no deleted_at).
//
// Destructive. Apply requires --confirm=DELETE_EMPTY_ALGORITHM_ASSESSMENTS.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const confirmToken = "DELETE_EMPTY_ALGORITHM_ASSESSMENTS"

type config struct {
	mysqlDSN string
	apply    bool
	confirm  string
	jsonOut  bool
	limit    int
	timeout  time.Duration
}

type bucket struct {
	Source    string `json:"source"`
	Kind      string `json:"kind"`
	Algorithm string `json:"algorithm"`
	Count     int    `json:"count"`
}

type report struct {
	AssessmentCandidates int      `json:"assessment_candidates"`
	OutcomeCandidates    int      `json:"outcome_candidates"`
	AssessmentsDeleted   int      `json:"assessments_deleted"`
	OutcomesDeleted      int      `json:"outcomes_deleted"`
	Buckets              []bucket `json:"buckets"`
}

func main() {
	cfg := parseFlags()
	if cfg.mysqlDSN == "" {
		fmt.Fprintln(os.Stderr, "soft_delete assessment empty algorithms failed: --mysql-dsn is required (or set MYSQL_DSN)")
		os.Exit(1)
	}
	if cfg.apply && cfg.confirm != confirmToken {
		fmt.Fprintf(os.Stderr, "refuse --apply without --confirm=%s\n", confirmToken)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	db, err := sql.Open("mysql", cfg.mysqlDSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open mysql:", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ping mysql:", err)
		os.Exit(1)
	}
	out, err := run(ctx, db, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "soft_delete failed:", err)
		os.Exit(1)
	}
	if cfg.jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	printReport(out, cfg.apply)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN")
	flag.BoolVar(&cfg.apply, "apply", false, "soft-delete eligible Assessment rows and delete matching Outcomes")
	flag.StringVar(&cfg.confirm, "confirm", "", "required with --apply: "+confirmToken)
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit JSON report")
	flag.IntVar(&cfg.limit, "limit", 0, "max Assessment rows to soft-delete (0 = all); Outcomes always all empty-algorithm rows")
	flag.DurationVar(&cfg.timeout, "timeout", 15*time.Minute, "operation timeout")
	flag.Parse()
	return cfg
}

func emptyAlgorithmPredicate(column string) string {
	return fmt.Sprintf("(%s IS NULL OR %s = '')", column, column)
}

func run(ctx context.Context, db *sql.DB, cfg config) (*report, error) {
	out := &report{Buckets: []bucket{}}
	aBuckets, aTotal, err := inventory(ctx, db, fmt.Sprintf(`
SELECT COALESCE(evaluation_model_kind, ''), COALESCE(evaluation_model_algorithm, ''), COUNT(*)
FROM assessment
WHERE deleted_at IS NULL
  AND %s
GROUP BY evaluation_model_kind, evaluation_model_algorithm`, emptyAlgorithmPredicate("evaluation_model_algorithm")))
	if err != nil {
		return nil, fmt.Errorf("inventory assessment: %w", err)
	}
	for _, b := range aBuckets {
		b.Source = "assessment"
		out.Buckets = append(out.Buckets, b)
	}
	out.AssessmentCandidates = aTotal

	oBuckets, oTotal, err := inventory(ctx, db, fmt.Sprintf(`
SELECT COALESCE(model_kind, ''), COALESCE(model_algorithm, ''), COUNT(*)
FROM evaluation_outcome
WHERE %s
GROUP BY model_kind, model_algorithm`, emptyAlgorithmPredicate("model_algorithm")))
	if err != nil {
		return nil, fmt.Errorf("inventory evaluation_outcome: %w", err)
	}
	for _, b := range oBuckets {
		b.Source = "evaluation_outcome"
		out.Buckets = append(out.Buckets, b)
	}
	out.OutcomeCandidates = oTotal

	sort.Slice(out.Buckets, func(i, j int) bool {
		if out.Buckets[i].Source != out.Buckets[j].Source {
			return out.Buckets[i].Source < out.Buckets[j].Source
		}
		if out.Buckets[i].Kind != out.Buckets[j].Kind {
			return out.Buckets[i].Kind < out.Buckets[j].Kind
		}
		return out.Buckets[i].Algorithm < out.Buckets[j].Algorithm
	})

	if !cfg.apply {
		return out, nil
	}

	n, err := softDeleteAssessments(ctx, db, cfg.limit)
	if err != nil {
		return nil, err
	}
	out.AssessmentsDeleted = n

	n, err = deleteOutcomes(ctx, db)
	if err != nil {
		return nil, err
	}
	out.OutcomesDeleted = n
	return out, nil
}

func inventory(ctx context.Context, db *sql.DB, query string) ([]bucket, int, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var out []bucket
	total := 0
	for rows.Next() {
		var kind, algorithm sql.NullString
		var cnt int
		if err := rows.Scan(&kind, &algorithm, &cnt); err != nil {
			return nil, 0, err
		}
		out = append(out, bucket{Kind: kind.String, Algorithm: algorithm.String, Count: cnt})
		total += cnt
	}
	return out, total, rows.Err()
}

func softDeleteAssessments(ctx context.Context, db *sql.DB, limit int) (int, error) {
	now := time.Now().UTC()
	query := fmt.Sprintf(`
UPDATE assessment
SET deleted_at = ?, updated_at = ?
WHERE deleted_at IS NULL
  AND %s`, emptyAlgorithmPredicate("evaluation_model_algorithm"))
	args := []any{now, now}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("soft-delete assessment: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func deleteOutcomes(ctx context.Context, db *sql.DB) (int, error) {
	res, err := db.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM evaluation_outcome
WHERE %s`, emptyAlgorithmPredicate("model_algorithm")))
	if err != nil {
		return 0, fmt.Errorf("delete evaluation_outcome: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func printReport(out *report, apply bool) {
	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	fmt.Printf("mode=%s assessment_candidates=%d outcome_candidates=%d assessments_deleted=%d outcomes_deleted=%d\n",
		mode, out.AssessmentCandidates, out.OutcomeCandidates, out.AssessmentsDeleted, out.OutcomesDeleted)
	for _, b := range out.Buckets {
		alg := b.Algorithm
		if alg == "" {
			alg = "(empty)"
		}
		fmt.Printf("  %-22s kind=%-20s algorithm=%-28s count=%d\n", b.Source, b.Kind, alg, b.Count)
	}
	if !apply {
		fmt.Printf("re-run with --apply --confirm=%s after backup/review\n", confirmToken)
	}
}
