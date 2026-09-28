// assessment_run_gap_audit performs a bounded, read-only Assessment to Run audit.
// It never publishes messages, mutates Outbox, or starts an evaluation.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const query = `SELECT a.id, a.status,
  a.submitted_at IS NOT NULL,
  COALESCE(a.submitted_at <= ?, 0),
  a.evaluation_model_kind IS NOT NULL AND a.evaluation_model_kind <> ''
    AND a.evaluation_model_code IS NOT NULL AND a.evaluation_model_code <> '',
  EXISTS (SELECT 1 FROM runtime_checkpoint rc
    WHERE rc.assessment_id = a.id AND rc.scope = 'evaluation_run' AND rc.deleted_at IS NULL)
FROM assessment a
WHERE a.id > ? AND a.id <= ? AND a.deleted_at IS NULL
ORDER BY a.id LIMIT ?`

type config struct {
	dsn        string
	afterID    uint64
	upperID    uint64
	cutoff     time.Time
	maxRows    int
	timeout    time.Duration
	includeIDs bool
}

type report struct {
	AfterID         uint64         `json:"after_id"`
	UpperID         uint64         `json:"upper_id"`
	NextAfterID     uint64         `json:"next_after_id"`
	SubmittedBefore time.Time      `json:"submitted_before"`
	Scanned         int            `json:"scanned"`
	Complete        bool           `json:"complete"`
	Counts          map[string]int `json:"counts"`
	CandidateIDs    []uint64       `json:"candidate_ids,omitempty"`
}

type publicReport struct {
	SubmittedBefore time.Time      `json:"submitted_before"`
	Scanned         int            `json:"scanned"`
	Complete        bool           `json:"complete"`
	Counts          map[string]int `json:"counts"`
}

func main() { os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func runCLI(parent context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stdin, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assessment run gap audit: invalid input")
		return 1
	}
	ctx, cancel := context.WithTimeout(parent, cfg.timeout)
	defer cancel()
	db, err := sql.Open("mysql", cfg.dsn)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assessment run gap audit: database open failed")
		return 1
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, "assessment run gap audit: database unavailable")
		return 1
	}
	result, err := scan(ctx, db, cfg)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assessment run gap audit: read failed")
		return 1
	}
	if err := encodeReport(stdout, result, cfg.includeIDs); err != nil {
		_, _ = fmt.Fprintln(stderr, "assessment run gap audit: output failed")
		return 1
	}
	if !result.Complete {
		return 3
	}
	if result.Counts["candidate_never_claimed"] != 0 || result.Counts["manual_required"] != 0 {
		return 2
	}
	return 0
}

func encodeReport(output io.Writer, result report, includeIDs bool) error {
	if includeIDs {
		return json.NewEncoder(output).Encode(result)
	}
	return json.NewEncoder(output).Encode(publicReport{
		SubmittedBefore: result.SubmittedBefore,
		Scanned:         result.Scanned,
		Complete:        result.Complete,
		Counts:          result.Counts,
	})
}

func parseConfig(args []string, stdin io.Reader, stderr io.Writer) (config, error) {
	var cfg config
	var cutoff string
	var connectionsStdin bool
	flags := flag.NewFlagSet("assessment_run_gap_audit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Uint64Var(&cfg.afterID, "after-id", 0, "exclusive Assessment ID lower bound")
	flags.Uint64Var(&cfg.upperID, "upper-id", 0, "inclusive fixed Assessment ID upper bound")
	flags.StringVar(&cutoff, "submitted-before", "", "fixed RFC3339 submission cutoff")
	flags.IntVar(&cfg.maxRows, "max-rows", 500, "maximum Assessment rows per invocation (1-1000)")
	flags.DurationVar(&cfg.timeout, "timeout", time.Minute, "overall deadline (1s-5m)")
	flags.BoolVar(&cfg.includeIDs, "include-ids", false, "include candidate IDs in private output")
	flags.BoolVar(&connectionsStdin, "connections-stdin", false, "read MySQL DSN JSON from stdin")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if connectionsStdin {
		var input struct {
			MySQLDSN string `json:"mysql_dsn"`
		}
		decoder := json.NewDecoder(io.LimitReader(stdin, 16385))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return config{}, err
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return config{}, errors.New("trailing connection input")
		}
		cfg.dsn = input.MySQLDSN
	} else {
		cfg.dsn = os.Getenv("MYSQL_DSN")
	}
	var err error
	cfg.cutoff, err = time.Parse(time.RFC3339, cutoff)
	if err != nil || flags.NArg() != 0 || cfg.dsn == "" || cfg.afterID >= cfg.upperID || cfg.upperID > math.MaxInt64 ||
		cfg.maxRows < 1 || cfg.maxRows > 1000 || cfg.timeout < time.Second || cfg.timeout > 5*time.Minute ||
		cfg.cutoff.After(time.Now().Add(-10*time.Minute)) {
		return config{}, errors.New("invalid bounded read-only scan configuration")
	}
	return cfg, nil
}

func scan(ctx context.Context, db *sql.DB, cfg config) (report, error) {
	result := report{
		AfterID: cfg.afterID, UpperID: cfg.upperID, NextAfterID: cfg.afterID,
		SubmittedBefore: cfg.cutoff, Complete: true, Counts: make(map[string]int),
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return report{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// Assessment timestamps are stored as UTC+8 wall-clock DATETIME values.
	cutoffWall := cfg.cutoff.In(time.FixedZone("UTC+8", 8*60*60)).Format("2006-01-02 15:04:05")
	rows, err := tx.QueryContext(ctx, query, cutoffWall, cfg.afterID, cfg.upperID, cfg.maxRows+1)
	if err != nil {
		return report{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id uint64
		var status string
		var hasSubmittedAt, matured, hasModel, hasRun bool
		if err := rows.Scan(&id, &status, &hasSubmittedAt, &matured, &hasModel, &hasRun); err != nil {
			return report{}, err
		}
		if result.Scanned == cfg.maxRows {
			result.Complete = false
			break
		}
		result.Scanned++
		result.NextAfterID = id
		category := classify(status, hasSubmittedAt, matured, hasModel, hasRun)
		result.Counts[category]++
		if cfg.includeIDs && category == "candidate_never_claimed" {
			result.CandidateIDs = append(result.CandidateIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return report{}, err
	}
	if err := rows.Close(); err != nil {
		return report{}, err
	}
	if err := tx.Commit(); err != nil {
		return report{}, err
	}
	return result, nil
}

func classify(status string, hasSubmittedAt, matured, hasModel, hasRun bool) string {
	switch status {
	case "evaluated":
		return "evaluated"
	case "failed":
		return "failed"
	case "submitted":
		if !hasSubmittedAt {
			return "manual_required"
		}
		if !hasModel {
			return "no_model"
		}
		if !matured {
			return "within_grace"
		}
		if hasRun {
			return "run_present"
		}
		return "candidate_never_claimed"
	case "pending":
		return "pending"
	default:
		return "manual_required"
	}
}
