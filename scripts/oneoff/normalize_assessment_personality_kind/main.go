// normalize_assessment_personality_kind rewrites MySQL Assessment /
// evaluation_outcome model_kind from retired "personality" to canonical
// "typology" (+ sub_kind=typology). Default dry-run.
//
// Does NOT rewrite model_algorithm (mbti/sbti/bigfive stay; dual-identity).
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

	"github.com/FangcunMount/qs-server/scripts/oneoff/internal/personalitykind"
)

type config struct {
	mysqlDSN string
	apply    bool
	jsonOut  bool
	limit    int
	timeout  time.Duration
}

type bucket struct {
	Source    string `json:"source"`
	Algorithm string `json:"algorithm"`
	Count     int    `json:"count"`
	Eligible  bool   `json:"eligible"`
	Reason    string `json:"reason"`
}

type report struct {
	Scanned   int      `json:"scanned"`
	Eligible  int      `json:"eligible"`
	Skipped   int      `json:"skipped"`
	Applied   int      `json:"applied"`
	Buckets   []bucket `json:"buckets"`
}

func main() {
	cfg := parseFlags()
	if cfg.mysqlDSN == "" {
		fmt.Fprintln(os.Stderr, "normalize assessment personality kind failed: --mysql-dsn is required (or set MYSQL_DSN)")
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
		fmt.Fprintln(os.Stderr, "normalize failed:", err)
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
	flag.BoolVar(&cfg.apply, "apply", false, "persist eligible kind rewrites")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit JSON report")
	flag.IntVar(&cfg.limit, "limit", 0, "max rows to update per table when applying (0 = all eligible)")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "operation timeout")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, db *sql.DB, cfg config) (*report, error) {
	out := &report{}
	assessmentBuckets, err := inventory(ctx, db, `
SELECT COALESCE(evaluation_model_algorithm, ''), COUNT(*)
FROM assessment
WHERE deleted_at IS NULL AND evaluation_model_kind = ?
GROUP BY evaluation_model_algorithm`, personalitykind.LegacyPersonalityKind)
	if err != nil {
		return nil, fmt.Errorf("inventory assessment: %w", err)
	}
	outcomeBuckets, err := inventory(ctx, db, `
SELECT COALESCE(model_algorithm, ''), COUNT(*)
FROM evaluation_outcome
WHERE model_kind = ?
GROUP BY model_algorithm`, personalitykind.LegacyPersonalityKind)
	if err != nil {
		return nil, fmt.Errorf("inventory evaluation_outcome: %w", err)
	}

	for _, item := range mergeBuckets("assessment", assessmentBuckets) {
		out.Buckets = append(out.Buckets, item)
		out.Scanned += item.Count
		if item.Eligible {
			out.Eligible += item.Count
		} else {
			out.Skipped += item.Count
		}
	}
	for _, item := range mergeBuckets("evaluation_outcome", outcomeBuckets) {
		out.Buckets = append(out.Buckets, item)
		out.Scanned += item.Count
		if item.Eligible {
			out.Eligible += item.Count
		} else {
			out.Skipped += item.Count
		}
	}
	sort.Slice(out.Buckets, func(i, j int) bool {
		if out.Buckets[i].Source != out.Buckets[j].Source {
			return out.Buckets[i].Source < out.Buckets[j].Source
		}
		return out.Buckets[i].Algorithm < out.Buckets[j].Algorithm
	})

	if !cfg.apply {
		return out, nil
	}

	n, err := applyAssessment(ctx, db, cfg.limit)
	if err != nil {
		return nil, err
	}
	out.Applied += n
	n, err = applyOutcome(ctx, db, cfg.limit)
	if err != nil {
		return nil, err
	}
	out.Applied += n
	return out, nil
}

func inventory(ctx context.Context, db *sql.DB, query, kind string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, query, kind)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var algorithm sql.NullString
		var cnt int
		if err := rows.Scan(&algorithm, &cnt); err != nil {
			return nil, err
		}
		out[algorithm.String] = cnt
	}
	return out, rows.Err()
}

func mergeBuckets(source string, counts map[string]int) []bucket {
	out := make([]bucket, 0, len(counts))
	for algorithm, cnt := range counts {
		decision := personalitykind.EvaluateAssessmentPersonalityKindRewrite(personalitykind.LegacyPersonalityKind, algorithm)
		out = append(out, bucket{
			Source: source, Algorithm: algorithm, Count: cnt,
			Eligible: decision.Eligible, Reason: decision.Reason,
		})
	}
	return out
}

func applyAssessment(ctx context.Context, db *sql.DB, limit int) (int, error) {
	now := time.Now().UTC()
	query := `
UPDATE assessment
SET evaluation_model_kind = 'typology',
    evaluation_model_sub_kind = 'typology',
    updated_at = ?
WHERE deleted_at IS NULL
  AND evaluation_model_kind = 'personality'
  AND (
    evaluation_model_algorithm IS NULL
    OR evaluation_model_algorithm = ''
    OR evaluation_model_algorithm IN ('mbti','sbti','bigfive','personality_typology')
  )`
	args := []any{now}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("update assessment: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func applyOutcome(ctx context.Context, db *sql.DB, limit int) (int, error) {
	query := `
UPDATE evaluation_outcome
SET model_kind = 'typology',
    model_sub_kind = 'typology'
WHERE model_kind = 'personality'
  AND (
    model_algorithm IS NULL
    OR model_algorithm = ''
    OR model_algorithm IN ('mbti','sbti','bigfive','personality_typology')
  )`
	args := []any{}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("update evaluation_outcome: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func printReport(out *report, apply bool) {
	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	fmt.Printf("mode=%s scanned=%d eligible=%d skipped=%d applied=%d\n", mode, out.Scanned, out.Eligible, out.Skipped, out.Applied)
	for _, b := range out.Buckets {
		status := "SKIP"
		if b.Eligible {
			status = "OK"
		}
		alg := b.Algorithm
		if alg == "" {
			alg = "(empty)"
		}
		fmt.Printf("  %-8s %-22s algorithm=%-22s count=%d %s\n", status, b.Source, alg, b.Count, b.Reason)
	}
	if !apply && out.Eligible > 0 {
		fmt.Println("re-run with --apply after review")
	}
}
