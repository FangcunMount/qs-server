package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

const (
	batchSize  = 500
	openWindow = 24 * time.Hour
)

var shanghai = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return loc
}()

type config struct {
	mode, dsn, checkpointFile, auditFile, operator string
	orgID                                          int64
	cutoff                                         time.Time
	confirm                                        bool
	timeout                                        time.Duration
}

type checkpoint struct {
	RunID     string    `json:"run_id"`
	Mode      string    `json:"mode"`
	OrgID     int64     `json:"org_id"`
	CutoffAt  time.Time `json:"cutoff_at"`
	Phase     string    `json:"phase"`
	LastID    uint64    `json:"last_id"`
	Completed bool      `json:"completed"`
}

type auditRecord struct {
	RunID    string          `json:"run_id"`
	At       time.Time       `json:"at"`
	Operator string          `json:"operator"`
	OrgID    int64           `json:"org_id"`
	CutoffAt time.Time       `json:"cutoff_at"`
	Kind     string          `json:"kind"`
	Phase    string          `json:"phase"`
	EntityID uint64          `json:"entity_id,omitempty"`
	Before   json.RawMessage `json:"before,omitempty"`
	After    json.RawMessage `json:"after,omitempty"`
	Checksum string          `json:"checksum,omitempty"`
	Details  any             `json:"details,omitempty"`
}

type taskState struct {
	ID                uint64     `json:"id"`
	Version           uint64     `json:"version"`
	Status            string     `json:"status"`
	PlannedAt         time.Time  `json:"planned_at"`
	DueAt             *time.Time `json:"due_at,omitempty"`
	ScheduleRevision  uint32     `json:"schedule_revision"`
	ScheduleDefinedAt *time.Time `json:"schedule_defined_at,omitempty"`
	OpenAt            *time.Time `json:"open_at,omitempty"`
	ExpireAt          *time.Time `json:"expire_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	ExpiredAt         *time.Time `json:"expired_at,omitempty"`
	CanceledAt        *time.Time `json:"canceled_at,omitempty"`
	ExpirationReason  *string    `json:"expiration_reason,omitempty"`
	AssessmentID      *uint64    `json:"assessment_id,omitempty"`
	EntryToken        string     `json:"entry_token"`
	EntryURL          string     `json:"entry_url"`
}

type enrollmentState struct {
	ID       uint64     `json:"id"`
	Version  uint64     `json:"version"`
	Status   string     `json:"status"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

type auditCounts struct {
	Tasks                   int64 `json:"tasks"`
	DueNull                 int64 `json:"due_at_null"`
	ScheduleDefinedNull     int64 `json:"schedule_defined_at_null"`
	ScheduleRevisionInvalid int64 `json:"schedule_revision_invalid"`
	InferredRevision1       int64 `json:"inferred_revision_1"`
	InferredRevision2       int64 `json:"inferred_revision_2"`
	CollapsedLegacy         int64 `json:"collapsed_legacy_revisions"`
	ScheduleAmbiguous       int64 `json:"schedule_inference_ambiguous"`
	StalePending            int64 `json:"stale_pending"`
	EligibleMissed          int64 `json:"eligible_missed"`
	DirtyActive             int64 `json:"dirty_active"`
	InactiveParent          int64 `json:"inactive_plan_or_enrollment"`
	InvalidTerminal         int64 `json:"invalid_terminal_enrollment"`
	UnclosedTerminal        int64 `json:"unclosed_terminal_enrollment"`
}

type auditWriter struct {
	file    *os.File
	gzip    *gzip.Writer
	encoder *json.Encoder
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "repair_stranded_plan_tasks:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	cfg, err := parseConfig(args, out)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	normalizedDSN, err := normalizeDSN(cfg.dsn)
	if err != nil {
		return err
	}
	db, err := sql.Open("mysql", normalizedDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "SET time_zone = '+08:00'"); err != nil {
		return fmt.Errorf("set Shanghai session timezone: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}

	cp, err := loadOrCreateCheckpoint(cfg)
	if err != nil {
		return err
	}
	writer, err := openAuditWriter(cfg.auditFile)
	if err != nil {
		return err
	}
	defer writer.Close()

	switch cfg.mode {
	case "audit":
		return runAudit(ctx, db, cfg, cp, writer, out)
	case "apply":
		return runApply(ctx, db, cfg, cp, writer, out)
	case "verify":
		return runVerify(ctx, db, cfg, cp, writer, out)
	case "rollback":
		return runRollback(ctx, db, cfg, cp, writer, out)
	default:
		return fmt.Errorf("unsupported mode %q", cfg.mode)
	}
}

func normalizeDSN(raw string) (string, error) {
	cfg, err := mysqlDriver.ParseDSN(raw)
	if err != nil {
		return "", fmt.Errorf("parse mysql dsn: %w", err)
	}
	if !cfg.ParseTime {
		return "", errors.New("mysql-dsn must set parseTime=true")
	}
	if cfg.Loc == nil || cfg.Loc.String() != shanghai.String() {
		return "", errors.New("mysql-dsn must set loc=Asia%2FShanghai")
	}
	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	// The driver applies this setting whenever it creates or reconnects a
	// session, so every statement uses the same Shanghai wall-clock semantics.
	cfg.Params["time_zone"] = "'+08:00'"
	return cfg.FormatDSN(), nil
}

func parseConfig(args []string, out io.Writer) (config, error) {
	fs := flag.NewFlagSet("repair_stranded_plan_tasks", flag.ContinueOnError)
	fs.SetOutput(out)
	var cfg config
	var cutoff string
	fs.StringVar(&cfg.mode, "mode", "audit", "audit|apply|verify|rollback")
	fs.StringVar(&cfg.dsn, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN or MYSQL_DSN; must enable parseTime")
	fs.Int64Var(&cfg.orgID, "org-id", 0, "organization ID (required)")
	fs.StringVar(&cutoff, "cutoff-at", "", "fixed Shanghai cutoff, e.g. 2026-08-03T00:00:00+08:00")
	fs.StringVar(&cfg.checkpointFile, "checkpoint-file", "", "checkpoint JSON path (required)")
	fs.StringVar(&cfg.auditFile, "audit-file", "", "gzip JSONL backup/audit path (required)")
	fs.StringVar(&cfg.operator, "operator", os.Getenv("USER"), "audited operator identity")
	fs.BoolVar(&cfg.confirm, "confirm", false, "confirm apply or rollback writes")
	fs.DurationVar(&cfg.timeout, "timeout", 6*time.Hour, "overall operation timeout")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	cfg.mode = strings.ToLower(strings.TrimSpace(cfg.mode))
	var err error
	cfg.cutoff, err = parseCutoff(cutoff)
	if err != nil {
		return cfg, err
	}
	if cfg.dsn == "" || cfg.orgID <= 0 || cfg.cutoff.IsZero() || cfg.checkpointFile == "" || cfg.auditFile == "" {
		return cfg, errors.New("mysql-dsn, org-id, cutoff-at, checkpoint-file and audit-file are required")
	}
	if (cfg.mode == "apply" || cfg.mode == "rollback") && !cfg.confirm {
		return cfg, fmt.Errorf("%s requires --confirm", cfg.mode)
	}
	if cfg.mode != "audit" && cfg.mode != "apply" && cfg.mode != "verify" && cfg.mode != "rollback" {
		return cfg, errors.New("mode must be audit, apply, verify, or rollback")
	}
	if cfg.timeout <= 0 {
		return cfg, errors.New("timeout must be positive")
	}
	return cfg, nil
}

func parseCutoff(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
		if value, err := time.ParseInLocation(layout, strings.TrimSpace(raw), shanghai); err == nil {
			return value.In(shanghai), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid cutoff-at %q", raw)
}

func loadOrCreateCheckpoint(cfg config) (*checkpoint, error) {
	data, err := os.ReadFile(cfg.checkpointFile)
	if err == nil {
		var cp checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			return nil, fmt.Errorf("decode checkpoint: %w", err)
		}
		if cp.OrgID != cfg.orgID || !cp.CutoffAt.Equal(cfg.cutoff) {
			return nil, errors.New("checkpoint scope does not match org-id/cutoff-at")
		}
		if cp.Mode == cfg.mode && !cp.Completed {
			return &cp, nil
		}
		if cfg.mode == "rollback" && cp.RunID != "" {
			cp.Mode, cp.Phase, cp.LastID, cp.Completed = cfg.mode, "rollback", 0, false
			return &cp, saveCheckpoint(cfg.checkpointFile, &cp)
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	cp := &checkpoint{RunID: fmt.Sprintf("%d-%d", cfg.orgID, time.Now().UTC().UnixNano()), Mode: cfg.mode, OrgID: cfg.orgID, CutoffAt: cfg.cutoff, Phase: cfg.mode}
	return cp, saveCheckpoint(cfg.checkpointFile, cp)
}

func saveCheckpoint(path string, cp *checkpoint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func openAuditWriter(path string) (*auditWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	gz := gzip.NewWriter(file)
	return &auditWriter{file: file, gzip: gz, encoder: json.NewEncoder(gz)}, nil
}

func (w *auditWriter) Write(record auditRecord) error { return w.encoder.Encode(record) }
func (w *auditWriter) Sync() error {
	if err := w.gzip.Flush(); err != nil {
		return err
	}
	return w.file.Sync()
}
func (w *auditWriter) Close() error {
	if w == nil {
		return nil
	}
	gzErr := w.gzip.Close()
	fileErr := w.file.Close()
	return errors.Join(gzErr, fileErr)
}

func runAudit(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter, out io.Writer) error {
	counts, err := collectAuditCounts(ctx, db, cfg)
	if err != nil {
		return err
	}
	if err := writeScheduleInferenceAudit(ctx, db, cfg, cp, writer); err != nil {
		return err
	}
	if err := writer.Write(baseAudit(cfg, cp, "summary", "audit", 0, nil, nil, counts)); err != nil {
		return err
	}
	if err := writer.Sync(); err != nil {
		return err
	}
	cp.Completed = true
	if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(counts)
}

func runVerify(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter, out io.Writer) error {
	counts, err := collectAuditCounts(ctx, db, cfg)
	if err != nil {
		return err
	}
	if err := writer.Write(baseAudit(cfg, cp, "summary", "verify", 0, nil, nil, counts)); err != nil {
		return err
	}
	if err := writer.Sync(); err != nil {
		return err
	}
	if err := json.NewEncoder(out).Encode(counts); err != nil {
		return err
	}
	if counts.DueNull != 0 || counts.ScheduleDefinedNull != 0 || counts.ScheduleRevisionInvalid != 0 || counts.ScheduleAmbiguous != 0 || counts.StalePending != 0 || counts.DirtyActive != 0 || counts.InactiveParent != 0 || counts.InvalidTerminal != 0 || counts.UnclosedTerminal != 0 {
		return fmt.Errorf("verification failed: due_null=%d schedule_defined_null=%d schedule_revision_invalid=%d schedule_ambiguous=%d stale_pending=%d dirty=%d inactive_parent=%d invalid_terminal_enrollment=%d unclosed_terminal_enrollment=%d", counts.DueNull, counts.ScheduleDefinedNull, counts.ScheduleRevisionInvalid, counts.ScheduleAmbiguous, counts.StalePending, counts.DirtyActive, counts.InactiveParent, counts.InvalidTerminal, counts.UnclosedTerminal)
	}
	cp.Completed = true
	return saveCheckpoint(cfg.checkpointFile, cp)
}

func collectAuditCounts(ctx context.Context, db *sql.DB, cfg config) (auditCounts, error) {
	threshold := cfg.cutoff.Add(-openWindow)
	var result auditCounts
	queries := []struct {
		target *int64
		query  string
		args   []any
	}{
		{&result.Tasks, `SELECT COUNT(*) FROM assessment_task WHERE org_id=? AND deleted_at IS NULL`, []any{cfg.orgID}},
		{&result.DueNull, `SELECT COUNT(*) FROM assessment_task WHERE org_id=? AND due_at IS NULL AND deleted_at IS NULL`, []any{cfg.orgID}},
		{&result.ScheduleDefinedNull, `SELECT COUNT(*) FROM assessment_task WHERE org_id=? AND schedule_defined_at IS NULL AND deleted_at IS NULL`, []any{cfg.orgID}},
		{&result.ScheduleRevisionInvalid, `SELECT COUNT(*) FROM assessment_task WHERE org_id=? AND schedule_revision<1 AND deleted_at IS NULL`, []any{cfg.orgID}},
		{&result.StalePending, `SELECT COUNT(*) FROM assessment_task WHERE org_id=? AND status='pending' AND planned_at<=? AND deleted_at IS NULL`, []any{cfg.orgID, threshold}},
		{&result.EligibleMissed, cleanMissedCountSQL, []any{cfg.orgID, threshold}},
		{&result.DirtyActive, dirtyMissedCountSQL, []any{cfg.orgID, threshold}},
		{&result.InactiveParent, inactiveMissedCountSQL, []any{cfg.orgID, threshold}},
		{&result.InvalidTerminal, invalidTerminalEnrollmentCountSQL, []any{cfg.orgID}},
		{&result.UnclosedTerminal, unclosedTerminalEnrollmentCountSQL, []any{cfg.orgID}},
	}
	for _, item := range queries {
		if err := db.QueryRowContext(ctx, item.query, item.args...).Scan(item.target); err != nil {
			return result, err
		}
	}
	if err := inspectScheduleInferences(ctx, db, cfg, func(_ scheduleCandidate, inference scheduleInference) error {
		if inference.Ambiguity != "" {
			result.ScheduleAmbiguous++
			return nil
		}
		switch inference.Revision {
		case 1:
			result.InferredRevision1++
		case 2:
			result.InferredRevision2++
		}
		if inference.Reason == "collapsed_legacy_revisions" {
			result.CollapsedLegacy++
		}
		return nil
	}); err != nil {
		return result, err
	}
	return result, nil
}

func inspectScheduleInferences(ctx context.Context, db *sql.DB, cfg config, handle func(scheduleCandidate, scheduleInference) error) error {
	var lastID uint64
	for {
		batch, err := loadScheduleCandidatesPage(ctx, db, cfg.orgID, lastID)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, candidate := range batch {
			if err := handle(candidate, inferSchedule(candidate)); err != nil {
				return err
			}
		}
		lastID = batch[len(batch)-1].ID
	}
}

func writeScheduleInferenceAudit(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter) error {
	return inspectScheduleInferences(ctx, db, cfg, func(candidate scheduleCandidate, inference scheduleInference) error {
		if inference.Ambiguity == "" && inference.Reason != "collapsed_legacy_revisions" {
			return nil
		}
		return writer.Write(baseAudit(cfg, cp, "schedule_inference", "audit", candidate.ID, nil, nil, inference))
	})
}

const parentJoinSQL = ` FROM assessment_task t LEFT JOIN assessment_plan p ON p.id=t.plan_id AND p.org_id=t.org_id AND p.deleted_at IS NULL LEFT JOIN plan_enrollment e ON e.id=t.enrollment_id AND e.plan_id=t.plan_id AND e.org_id=t.org_id AND e.deleted_at IS NULL `
const cleanPredicate = ` t.open_at IS NULL AND t.expire_at IS NULL AND t.completed_at IS NULL AND t.expired_at IS NULL AND t.canceled_at IS NULL AND t.assessment_id IS NULL AND COALESCE(t.entry_token,'')='' AND COALESCE(t.entry_url,'')='' AND COALESCE(t.expiration_reason,'')='' `
const cleanMissedCountSQL = `SELECT COUNT(*)` + parentJoinSQL + `WHERE t.org_id=? AND t.status='pending' AND t.planned_at<=? AND t.deleted_at IS NULL AND p.status='active' AND e.status='active' AND ` + cleanPredicate
const dirtyMissedCountSQL = `SELECT COUNT(*)` + parentJoinSQL + `WHERE t.org_id=? AND t.status='pending' AND t.planned_at<=? AND t.deleted_at IS NULL AND p.status='active' AND e.status='active' AND NOT (` + cleanPredicate + `)`
const inactiveMissedCountSQL = `SELECT COUNT(*)` + parentJoinSQL + `WHERE t.org_id=? AND t.status='pending' AND t.planned_at<=? AND t.deleted_at IS NULL AND (p.id IS NULL OR e.id IS NULL OR p.status<>'active' OR e.status<>'active')`
const invalidTerminalEnrollmentCountSQL = `SELECT COUNT(*) FROM (SELECT e.id FROM plan_enrollment e JOIN assessment_task t ON t.enrollment_id=e.id AND t.org_id=e.org_id AND t.deleted_at IS NULL WHERE e.org_id=? AND e.status='active' AND e.deleted_at IS NULL GROUP BY e.id HAVING SUM(t.status NOT IN ('completed','expired','canceled'))=0 AND SUM(CASE WHEN t.status='completed' AND t.completed_at IS NULL THEN 1 WHEN t.status='expired' AND t.expired_at IS NULL THEN 1 WHEN t.status='canceled' AND t.canceled_at IS NULL THEN 1 ELSE 0 END)>0) invalid`
const unclosedTerminalEnrollmentCountSQL = `SELECT COUNT(*) FROM (SELECT e.id FROM plan_enrollment e JOIN assessment_task t ON t.enrollment_id=e.id AND t.org_id=e.org_id AND t.deleted_at IS NULL WHERE e.org_id=? AND e.status='active' AND e.deleted_at IS NULL GROUP BY e.id HAVING SUM(t.status NOT IN ('completed','expired','canceled'))=0) unclosed`

func runApply(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter, out io.Writer) error {
	counts, err := collectAuditCounts(ctx, db, cfg)
	if err != nil {
		return err
	}
	if counts.ScheduleRevisionInvalid != 0 || counts.ScheduleAmbiguous != 0 || counts.DirtyActive != 0 || counts.InactiveParent != 0 || counts.InvalidTerminal != 0 {
		return fmt.Errorf("preflight requires manual handling: schedule_revision_invalid=%d schedule_ambiguous=%d dirty=%d inactive_parent=%d invalid_terminal_enrollment=%d", counts.ScheduleRevisionInvalid, counts.ScheduleAmbiguous, counts.DirtyActive, counts.InactiveParent, counts.InvalidTerminal)
	}
	if cp.Phase == "apply" || cp.Phase == "backfill_schedule" {
		cp.Phase = "backfill_schedule"
		if err := backfillSchedule(ctx, db, cfg, cp, writer, out); err != nil {
			return err
		}
		cp.Phase, cp.LastID = "backfill_due", 0
		if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
			return err
		}
	}
	if cp.Phase == "backfill_due" {
		cp.Phase = "backfill_due"
		if err := backfillDue(ctx, db, cfg, cp, writer, out); err != nil {
			return err
		}
		cp.Phase, cp.LastID = "expire_missed", 0
		if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
			return err
		}
	}
	if cp.Phase == "expire_missed" {
		if err := expireMissed(ctx, db, cfg, cp, writer, out); err != nil {
			return err
		}
		cp.Phase, cp.LastID = "close_enrollments", 0
		if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
			return err
		}
	}
	if cp.Phase == "close_enrollments" {
		if err := closeEnrollments(ctx, db, cfg, cp, writer, out); err != nil {
			return err
		}
		cp.Phase = "verify"
		if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
			return err
		}
	}
	counts, err = collectAuditCounts(ctx, db, cfg)
	if err != nil {
		return err
	}
	if counts.DueNull != 0 || counts.ScheduleDefinedNull != 0 || counts.ScheduleRevisionInvalid != 0 || counts.ScheduleAmbiguous != 0 || counts.StalePending != 0 || counts.DirtyActive != 0 || counts.InactiveParent != 0 || counts.InvalidTerminal != 0 || counts.UnclosedTerminal != 0 {
		return fmt.Errorf("post-apply verification failed: due_null=%d schedule_defined_null=%d schedule_revision_invalid=%d schedule_ambiguous=%d stale_pending=%d dirty=%d inactive_parent=%d invalid_terminal_enrollment=%d unclosed_terminal_enrollment=%d", counts.DueNull, counts.ScheduleDefinedNull, counts.ScheduleRevisionInvalid, counts.ScheduleAmbiguous, counts.StalePending, counts.DirtyActive, counts.InactiveParent, counts.InvalidTerminal, counts.UnclosedTerminal)
	}
	cp.Completed = true
	if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(counts)
}

func backfillSchedule(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter, out io.Writer) error {
	for {
		batch, err := loadScheduleCandidatesPage(ctx, db, cfg.orgID, cp.LastID)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		inferences := make([]scheduleInference, len(batch))
		for index, candidate := range batch {
			inferences[index] = inferSchedule(candidate)
			if inferences[index].Ambiguity != "" {
				return fmt.Errorf("backfill_schedule task=%d is ambiguous: %s", candidate.ID, inferences[index].Ambiguity)
			}
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for index, candidate := range batch {
			inference := inferences[index]
			before := taskScheduleState{ID: candidate.ID, Version: candidate.Version, ScheduleRevision: candidate.ScheduleRevision}
			definedAt := inference.DefinedAt
			after := taskScheduleState{ID: candidate.ID, Version: candidate.Version + 1, ScheduleRevision: inference.Revision, ScheduleDefinedAt: &definedAt}
			res, err := tx.ExecContext(ctx, `UPDATE assessment_task SET schedule_revision=?,schedule_defined_at=?,version=version+1 WHERE org_id=? AND id=? AND version=? AND schedule_revision=? AND schedule_defined_at IS NULL AND status=? AND planned_at=? AND open_at <=> ? AND completed_at <=> ? AND expired_at <=> ? AND canceled_at <=> ? AND deleted_at IS NULL`, inference.Revision, definedAt, cfg.orgID, candidate.ID, candidate.Version, candidate.ScheduleRevision, candidate.Status, candidate.PlannedAt, nullableValue(candidate.OpenAt), nullableValue(candidate.CompletedAt), nullableValue(candidate.ExpiredAt), nullableValue(candidate.CanceledAt))
			if err != nil {
				tx.Rollback()
				return err
			}
			if changed, _ := res.RowsAffected(); changed != 1 {
				tx.Rollback()
				return fmt.Errorf("backfill_schedule CAS conflict task=%d", candidate.ID)
			}
			beforeJSON, _ := json.Marshal(before)
			afterJSON, _ := json.Marshal(after)
			if err := writer.Write(baseAudit(cfg, cp, "task_schedule", "backfill_schedule", candidate.ID, beforeJSON, afterJSON, inference)); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := writer.Sync(); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		cp.LastID = batch[len(batch)-1].ID
		if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
			return err
		}
		fmt.Fprintf(out, "phase=backfill_schedule checkpoint=%d updated=%d\n", cp.LastID, len(batch))
	}
}

func backfillDue(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter, out io.Writer) error {
	for {
		rows, err := db.QueryContext(ctx, `SELECT id,version,status,planned_at,due_at,schedule_revision,schedule_defined_at,open_at,expire_at,completed_at,expired_at,canceled_at,expiration_reason,assessment_id,COALESCE(entry_token,''),COALESCE(entry_url,'') FROM assessment_task WHERE org_id=? AND deleted_at IS NULL AND due_at IS NULL AND id>? ORDER BY id LIMIT ?`, cfg.orgID, cp.LastID, batchSize)
		if err != nil {
			return err
		}
		var batch []taskState
		for rows.Next() {
			item, err := scanTaskState(rows)
			if err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, item)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, before := range batch {
			due := before.PlannedAt.In(shanghai).AddDate(0, 0, 7)
			after := before
			after.Version++
			after.DueAt = &due
			res, err := tx.ExecContext(ctx, `UPDATE assessment_task SET due_at=?,version=version+1,updated_at=updated_at WHERE org_id=? AND id=? AND version=? AND due_at IS NULL AND status=? AND planned_at=? AND schedule_revision=? AND schedule_defined_at <=> ? AND deleted_at IS NULL`, due, cfg.orgID, before.ID, before.Version, before.Status, before.PlannedAt, before.ScheduleRevision, nullableValue(before.ScheduleDefinedAt))
			if err != nil {
				tx.Rollback()
				return err
			}
			if n, _ := res.RowsAffected(); n != 1 {
				tx.Rollback()
				return fmt.Errorf("backfill_due CAS conflict task=%d", before.ID)
			}
			if err := writeStateAudit(writer, cfg, cp, "task", "backfill_due", before.ID, before, after); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := writer.Sync(); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		cp.LastID = batch[len(batch)-1].ID
		if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
			return err
		}
		fmt.Fprintf(out, "phase=backfill_due checkpoint=%d updated=%d\n", cp.LastID, len(batch))
	}
}

func expireMissed(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter, out io.Writer) error {
	threshold := cfg.cutoff.Add(-openWindow)
	query := `SELECT t.id,t.version,t.status,t.planned_at,t.due_at,t.schedule_revision,t.schedule_defined_at,t.open_at,t.expire_at,t.completed_at,t.expired_at,t.canceled_at,t.expiration_reason,t.assessment_id,COALESCE(t.entry_token,''),COALESCE(t.entry_url,'')` + parentJoinSQL + `WHERE t.org_id=? AND t.status='pending' AND t.planned_at<=? AND t.id>? AND t.deleted_at IS NULL AND p.status='active' AND e.status='active' AND ` + cleanPredicate + ` ORDER BY t.id LIMIT ?`
	for {
		rows, err := db.QueryContext(ctx, query, cfg.orgID, threshold, cp.LastID, batchSize)
		if err != nil {
			return err
		}
		var batch []taskState
		for rows.Next() {
			state, err := scanTaskState(rows)
			if err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, state)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, before := range batch {
			expiredAt := before.PlannedAt.In(shanghai).Add(openWindow)
			reason := "missed_open_window"
			after := before
			after.Version++
			after.Status = "expired"
			after.ExpiredAt = &expiredAt
			after.ExpirationReason = &reason
			res, err := tx.ExecContext(ctx, `UPDATE assessment_task SET status='expired',expiration_reason='missed_open_window',expired_at=?,version=version+1 WHERE org_id=? AND id=? AND version=? AND status='pending' AND planned_at=? AND due_at <=> ? AND schedule_revision=? AND schedule_defined_at <=> ? AND open_at IS NULL AND expire_at IS NULL AND completed_at IS NULL AND expired_at IS NULL AND canceled_at IS NULL AND assessment_id IS NULL AND COALESCE(entry_token,'')='' AND COALESCE(entry_url,'')='' AND COALESCE(expiration_reason,'')='' AND deleted_at IS NULL`, expiredAt, cfg.orgID, before.ID, before.Version, before.PlannedAt, nullableValue(before.DueAt), before.ScheduleRevision, nullableValue(before.ScheduleDefinedAt))
			if err != nil {
				tx.Rollback()
				return err
			}
			if n, _ := res.RowsAffected(); n != 1 {
				tx.Rollback()
				return fmt.Errorf("expire_missed CAS conflict task=%d", before.ID)
			}
			if err := writeStateAudit(writer, cfg, cp, "task", "expire_missed", before.ID, before, after); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := writer.Sync(); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		cp.LastID = batch[len(batch)-1].ID
		if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
			return err
		}
		fmt.Fprintf(out, "phase=expire_missed checkpoint=%d updated=%d\n", cp.LastID, len(batch))
	}
}

type rowScanner interface{ Scan(...any) error }

func scanTaskState(row rowScanner) (taskState, error) {
	var s taskState
	var due, scheduleDefined, open, expire, completed, expired, canceled sql.NullTime
	var reason sql.NullString
	var assessmentID sql.NullInt64
	err := row.Scan(&s.ID, &s.Version, &s.Status, &s.PlannedAt, &due, &s.ScheduleRevision, &scheduleDefined, &open, &expire, &completed, &expired, &canceled, &reason, &assessmentID, &s.EntryToken, &s.EntryURL)
	if err != nil {
		return s, err
	}
	s.DueAt = nullTime(due)
	s.ScheduleDefinedAt = nullTime(scheduleDefined)
	s.OpenAt = nullTime(open)
	s.ExpireAt = nullTime(expire)
	s.CompletedAt = nullTime(completed)
	s.ExpiredAt = nullTime(expired)
	s.CanceledAt = nullTime(canceled)
	if reason.Valid {
		s.ExpirationReason = &reason.String
	}
	if assessmentID.Valid {
		id := uint64(assessmentID.Int64)
		s.AssessmentID = &id
	}
	return s, nil
}
func nullTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func closeEnrollments(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter, out io.Writer) error {
	rows, err := db.QueryContext(ctx, `SELECT e.id,e.version,e.status,e.closed_at,MAX(CASE WHEN t.status='completed' THEN t.completed_at WHEN t.status='expired' THEN t.expired_at WHEN t.status='canceled' THEN t.canceled_at END) terminal_at FROM plan_enrollment e JOIN assessment_task t ON t.enrollment_id=e.id AND t.org_id=e.org_id AND t.deleted_at IS NULL WHERE e.org_id=? AND e.status='active' AND e.deleted_at IS NULL AND e.id>? GROUP BY e.id,e.version,e.status,e.closed_at HAVING SUM(t.status NOT IN ('completed','expired','canceled'))=0 AND SUM(CASE WHEN t.status='completed' AND t.completed_at IS NULL THEN 1 WHEN t.status='expired' AND t.expired_at IS NULL THEN 1 WHEN t.status='canceled' AND t.canceled_at IS NULL THEN 1 ELSE 0 END)=0 ORDER BY e.id LIMIT ?`, cfg.orgID, cp.LastID, batchSize)
	if err != nil {
		return err
	}
	type candidate struct {
		before   enrollmentState
		terminal time.Time
	}
	var batch []candidate
	for rows.Next() {
		var c candidate
		var closed, terminal sql.NullTime
		if err := rows.Scan(&c.before.ID, &c.before.Version, &c.before.Status, &closed, &terminal); err != nil {
			rows.Close()
			return err
		}
		if !terminal.Valid {
			rows.Close()
			return fmt.Errorf("enrollment %d has terminal tasks without terminal timestamps", c.before.ID)
		}
		c.terminal = terminal.Time
		c.before.ClosedAt = nullTime(closed)
		batch = append(batch, c)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, c := range batch {
		after := c.before
		after.Version++
		after.Status = "closed"
		after.ClosedAt = &c.terminal
		res, err := tx.ExecContext(ctx, `UPDATE plan_enrollment SET status='closed',closed_at=?,version=version+1 WHERE org_id=? AND id=? AND version=? AND status='active' AND deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM assessment_task t WHERE t.enrollment_id=plan_enrollment.id AND t.org_id=plan_enrollment.org_id AND t.deleted_at IS NULL AND t.status NOT IN ('completed','expired','canceled'))`, c.terminal, cfg.orgID, c.before.ID, c.before.Version)
		if err != nil {
			tx.Rollback()
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			tx.Rollback()
			return fmt.Errorf("close enrollment CAS conflict id=%d", c.before.ID)
		}
		if err := writeStateAudit(writer, cfg, cp, "enrollment", "close_enrollment", c.before.ID, c.before, after); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := writer.Sync(); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	cp.LastID = batch[len(batch)-1].before.ID
	if err := saveCheckpoint(cfg.checkpointFile, cp); err != nil {
		return err
	}
	fmt.Fprintf(out, "phase=close_enrollments checkpoint=%d updated=%d\n", cp.LastID, len(batch))
	return closeEnrollments(ctx, db, cfg, cp, writer, out)
}

func writeStateAudit(writer *auditWriter, cfg config, cp *checkpoint, kind, phase string, id uint64, before, after any) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	return writer.Write(baseAudit(cfg, cp, kind, phase, id, beforeJSON, afterJSON, nil))
}
func baseAudit(cfg config, cp *checkpoint, kind, phase string, id uint64, before, after json.RawMessage, details any) auditRecord {
	record := auditRecord{RunID: cp.RunID, At: time.Now().UTC(), Operator: cfg.operator, OrgID: cfg.orgID, CutoffAt: cfg.cutoff, Kind: kind, Phase: phase, EntityID: id, Before: before, After: after, Details: details}
	sum := sha256.Sum256(append(append([]byte{}, before...), after...))
	record.Checksum = hex.EncodeToString(sum[:])
	return record
}

func runRollback(ctx context.Context, db *sql.DB, cfg config, cp *checkpoint, writer *auditWriter, out io.Writer) error {
	records, err := readAuditRecords(cfg.auditFile, cp.RunID)
	if err != nil {
		return err
	}
	if err := ensureScheduleFactsNotPublished(ctx, db, cfg.orgID, records); err != nil {
		return err
	}
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		switch record.Kind {
		case "task":
			var before, after taskState
			if err := json.Unmarshal(record.Before, &before); err != nil {
				return err
			}
			if err := json.Unmarshal(record.After, &after); err != nil {
				return err
			}
			changed, err := rollbackTask(ctx, db, cfg, before, after)
			if err != nil {
				return fmt.Errorf("rollback stopped at task %d: %w", before.ID, err)
			}
			if !changed {
				fmt.Fprintf(out, "rollback skipped_unapplied kind=task id=%d phase=%s\n", record.EntityID, record.Phase)
				continue
			}
		case "task_schedule":
			var before, after taskScheduleState
			if err := json.Unmarshal(record.Before, &before); err != nil {
				return err
			}
			if err := json.Unmarshal(record.After, &after); err != nil {
				return err
			}
			changed, err := rollbackTaskSchedule(ctx, db, cfg, before, after)
			if err != nil {
				return fmt.Errorf("rollback stopped at task schedule %d: %w", before.ID, err)
			}
			if !changed {
				fmt.Fprintf(out, "rollback skipped_unapplied kind=task_schedule id=%d phase=%s\n", record.EntityID, record.Phase)
				continue
			}
		case "enrollment":
			var before, after enrollmentState
			if err := json.Unmarshal(record.Before, &before); err != nil {
				return err
			}
			if err := json.Unmarshal(record.After, &after); err != nil {
				return err
			}
			changed, err := rollbackEnrollment(ctx, db, cfg, before, after)
			if err != nil {
				return fmt.Errorf("rollback stopped at enrollment %d: %w", before.ID, err)
			}
			if !changed {
				fmt.Fprintf(out, "rollback skipped_unapplied kind=enrollment id=%d phase=%s\n", record.EntityID, record.Phase)
				continue
			}
		default:
			continue
		}
		if err := writer.Write(baseAudit(cfg, cp, "rollback", record.Phase, record.EntityID, nil, nil, map[string]any{"restored_kind": record.Kind})); err != nil {
			return err
		}
		fmt.Fprintf(out, "rollback kind=%s id=%d phase=%s\n", record.Kind, record.EntityID, record.Phase)
	}
	if err := writer.Sync(); err != nil {
		return err
	}
	cp.Completed = true
	return saveCheckpoint(cfg.checkpointFile, cp)
}

func ensureScheduleFactsNotPublished(ctx context.Context, db *sql.DB, orgID int64, records []auditRecord) error {
	taskIDs := make([]uint64, 0, len(records))
	seen := make(map[uint64]struct{}, len(records))
	for _, record := range records {
		if record.Kind != "task" && record.Kind != "task_schedule" {
			continue
		}
		var identity struct {
			ID uint64 `json:"id"`
		}
		if err := json.Unmarshal(record.After, &identity); err != nil {
			return fmt.Errorf("decode rollback task identity: %w", err)
		}
		if identity.ID == 0 {
			continue
		}
		if _, exists := seen[identity.ID]; exists {
			continue
		}
		seen[identity.ID] = struct{}{}
		taskIDs = append(taskIDs, identity.ID)
	}
	for start := 0; start < len(taskIDs); start += batchSize {
		end := start + batchSize
		if end > len(taskIDs) {
			end = len(taskIDs)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", end-start), ",")
		args := make([]any, 0, end-start+1)
		args = append(args, orgID)
		for _, id := range taskIDs[start:end] {
			args = append(args, id)
		}
		var published int64
		query := `SELECT COUNT(*) FROM statistics_plan_fact WHERE org_id=? AND task_id IN (` + placeholders + `) AND fact_type IN ('task_schedule_defined','task_schedule_terminal')`
		if err := db.QueryRowContext(ctx, query, args...).Scan(&published); err != nil {
			return err
		}
		if published != 0 {
			return fmt.Errorf("rollback refused: %d immutable schedule facts already published for repaired tasks; restore a database snapshot or create a forward schedule revision", published)
		}
	}
	return nil
}

func readAuditRecords(path, runID string) ([]auditRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	gz.Multistream(true)
	decoder := json.NewDecoder(gz)
	var records []auditRecord
	for {
		var record auditRecord
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if record.RunID == runID && (record.Kind == "task" || record.Kind == "task_schedule" || record.Kind == "enrollment") {
			records = append(records, record)
		}
	}
	return records, nil
}

func rollbackTask(ctx context.Context, db *sql.DB, cfg config, before, after taskState) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE assessment_task SET status=?,due_at=?,schedule_revision=?,schedule_defined_at=?,open_at=?,expire_at=?,completed_at=?,expired_at=?,canceled_at=?,expiration_reason=?,assessment_id=?,entry_token=?,entry_url=?,version=? WHERE org_id=? AND id=? AND version=? AND status=? AND due_at <=> ? AND schedule_revision=? AND schedule_defined_at <=> ? AND open_at <=> ? AND expire_at <=> ? AND completed_at <=> ? AND expired_at <=> ? AND canceled_at <=> ? AND expiration_reason <=> ? AND assessment_id <=> ? AND entry_token=? AND entry_url=? AND deleted_at IS NULL`, before.Status, nullableValue(before.DueAt), before.ScheduleRevision, nullableValue(before.ScheduleDefinedAt), nullableValue(before.OpenAt), nullableValue(before.ExpireAt), nullableValue(before.CompletedAt), nullableValue(before.ExpiredAt), nullableValue(before.CanceledAt), nullableValue(before.ExpirationReason), nullableValue(before.AssessmentID), before.EntryToken, before.EntryURL, before.Version, cfg.orgID, before.ID, after.Version, after.Status, nullableValue(after.DueAt), after.ScheduleRevision, nullableValue(after.ScheduleDefinedAt), nullableValue(after.OpenAt), nullableValue(after.ExpireAt), nullableValue(after.CompletedAt), nullableValue(after.ExpiredAt), nullableValue(after.CanceledAt), nullableValue(after.ExpirationReason), nullableValue(after.AssessmentID), after.EntryToken, after.EntryURL)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		current, loadErr := loadTaskState(ctx, db, cfg.orgID, before.ID)
		if loadErr != nil {
			return false, loadErr
		}
		if taskStatesEqual(current, before) {
			return false, nil
		}
		return false, errors.New("record changed after repair")
	}
	return true, nil
}

func rollbackTaskSchedule(ctx context.Context, db *sql.DB, cfg config, before, after taskScheduleState) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE assessment_task SET schedule_revision=?,schedule_defined_at=?,version=? WHERE org_id=? AND id=? AND version=? AND schedule_revision=? AND schedule_defined_at <=> ? AND deleted_at IS NULL`, before.ScheduleRevision, nullableValue(before.ScheduleDefinedAt), before.Version, cfg.orgID, before.ID, after.Version, after.ScheduleRevision, nullableValue(after.ScheduleDefinedAt))
	if err != nil {
		return false, err
	}
	if changed, _ := res.RowsAffected(); changed == 1 {
		return true, nil
	}
	var current taskScheduleState
	var definedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT id,version,schedule_revision,schedule_defined_at FROM assessment_task WHERE org_id=? AND id=? AND deleted_at IS NULL`, cfg.orgID, before.ID).Scan(&current.ID, &current.Version, &current.ScheduleRevision, &definedAt); err != nil {
		return false, err
	}
	current.ScheduleDefinedAt = nullTime(definedAt)
	if current.ID == before.ID && current.Version == before.Version && current.ScheduleRevision == before.ScheduleRevision && timesEqual(current.ScheduleDefinedAt, before.ScheduleDefinedAt) {
		return false, nil
	}
	return false, errors.New("schedule fields changed after repair")
}
func rollbackEnrollment(ctx context.Context, db *sql.DB, cfg config, before, after enrollmentState) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE plan_enrollment SET status=?,closed_at=?,version=? WHERE org_id=? AND id=? AND version=? AND status=? AND closed_at <=> ? AND deleted_at IS NULL`, before.Status, nullableValue(before.ClosedAt), before.Version, cfg.orgID, before.ID, after.Version, after.Status, nullableValue(after.ClosedAt))
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		var current enrollmentState
		var closed sql.NullTime
		if err := db.QueryRowContext(ctx, `SELECT id,version,status,closed_at FROM plan_enrollment WHERE org_id=? AND id=? AND deleted_at IS NULL`, cfg.orgID, before.ID).Scan(&current.ID, &current.Version, &current.Status, &closed); err != nil {
			return false, err
		}
		current.ClosedAt = nullTime(closed)
		if current.ID == before.ID && current.Version == before.Version && current.Status == before.Status && timesEqual(current.ClosedAt, before.ClosedAt) {
			return false, nil
		}
		return false, errors.New("enrollment changed after repair")
	}
	return true, nil
}

func loadTaskState(ctx context.Context, db *sql.DB, orgID int64, id uint64) (taskState, error) {
	row := db.QueryRowContext(ctx, `SELECT id,version,status,planned_at,due_at,schedule_revision,schedule_defined_at,open_at,expire_at,completed_at,expired_at,canceled_at,expiration_reason,assessment_id,COALESCE(entry_token,''),COALESCE(entry_url,'') FROM assessment_task WHERE org_id=? AND id=? AND deleted_at IS NULL`, orgID, id)
	return scanTaskState(row)
}

func taskStatesEqual(left, right taskState) bool {
	return left.ID == right.ID && left.Version == right.Version && left.Status == right.Status && left.PlannedAt.Equal(right.PlannedAt) &&
		left.ScheduleRevision == right.ScheduleRevision && timesEqual(left.ScheduleDefinedAt, right.ScheduleDefinedAt) &&
		timesEqual(left.DueAt, right.DueAt) && timesEqual(left.OpenAt, right.OpenAt) && timesEqual(left.ExpireAt, right.ExpireAt) &&
		timesEqual(left.CompletedAt, right.CompletedAt) && timesEqual(left.ExpiredAt, right.ExpiredAt) && timesEqual(left.CanceledAt, right.CanceledAt) &&
		stringsEqual(left.ExpirationReason, right.ExpirationReason) && uint64sEqual(left.AssessmentID, right.AssessmentID) &&
		left.EntryToken == right.EntryToken && left.EntryURL == right.EntryURL
}

func timesEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func stringsEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func uint64sEqual(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func nullableValue[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}
