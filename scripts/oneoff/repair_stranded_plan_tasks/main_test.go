package main

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func requiredArgs(mode string) []string {
	return []string{"--mode", mode, "--mysql-dsn", "user:pass@tcp(localhost:3306)/qs?parseTime=true&loc=Asia%2FShanghai", "--org-id", "1", "--cutoff-at", "2026-08-03T00:00:00+08:00", "--checkpoint-file", filepath.Join("tmp", "checkpoint.json"), "--audit-file", filepath.Join("tmp", "audit.jsonl.gz")}
}

func TestParseConfigDefaultsToReadOnlyAudit(t *testing.T) {
	cfg, err := parseConfig(requiredArgs("audit"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.mode != "audit" || cfg.confirm || cfg.orgID != 1 {
		t.Fatalf("config=%+v", cfg)
	}
	if cfg.cutoff.Location().String() != "Asia/Shanghai" {
		t.Fatalf("cutoff location=%s", cfg.cutoff.Location())
	}
}

func TestParseConfigRequiresConfirmationForWrites(t *testing.T) {
	for _, mode := range []string{"apply", "rollback"} {
		if _, err := parseConfig(requiredArgs(mode), &bytes.Buffer{}); err == nil {
			t.Fatalf("%s without --confirm must fail", mode)
		}
		args := append(requiredArgs(mode), "--confirm")
		if _, err := parseConfig(args, &bytes.Buffer{}); err != nil {
			t.Fatalf("%s with confirmation: %v", mode, err)
		}
	}
}

func TestParseCutoffUsesShanghaiForLocalTimestamp(t *testing.T) {
	got, err := parseCutoff("2026-08-03 12:30:00.000")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 3, 12, 30, 0, 0, shanghai)
	if !got.Equal(want) {
		t.Fatalf("cutoff=%s want=%s", got, want)
	}
}

func TestTaskBackupChecksumChangesWithAfterState(t *testing.T) {
	cfg := config{orgID: 1, cutoff: time.Now()}
	cp := &checkpoint{RunID: "run"}
	before := []byte(`{"id":1,"version":1}`)
	first := baseAudit(cfg, cp, "task", "backfill_due", 1, before, []byte(`{"id":1,"version":2}`), nil)
	second := baseAudit(cfg, cp, "task", "backfill_due", 1, before, []byte(`{"id":1,"version":3}`), nil)
	if first.Checksum == second.Checksum {
		t.Fatal("checksum must cover before and after state")
	}
}

func TestNormalizeDSNRequiresShanghaiTimeParsing(t *testing.T) {
	for _, dsn := range []string{
		"user:pass@tcp(localhost:3306)/qs?loc=Asia%2FShanghai",
		"user:pass@tcp(localhost:3306)/qs?parseTime=true",
	} {
		if _, err := normalizeDSN(dsn); err == nil {
			t.Fatalf("unsafe DSN must fail: %s", dsn)
		}
	}
	normalized, err := normalizeDSN("user:pass@tcp(localhost:3306)/qs?parseTime=true&loc=Asia%2FShanghai")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(normalized, "time_zone=%27%2B08%3A00%27") {
		t.Fatalf("normalized DSN does not pin session time zone: %s", normalized)
	}
}

func TestRollbackTaskSkipsDurableAuditRecordWhoseTransactionDidNotCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	planned := time.Date(2026, 8, 1, 19, 0, 0, 0, shanghai)
	before := taskState{ID: 10, Version: 3, Status: "pending", PlannedAt: planned}
	due := planned.AddDate(0, 0, 7)
	after := before
	after.Version = 4
	after.DueAt = &due

	mock.ExpectExec(regexp.QuoteMeta("UPDATE assessment_task SET")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,version,status,planned_at,due_at,open_at,expire_at,completed_at,expired_at,canceled_at,expiration_reason,assessment_id,COALESCE(entry_token,''),COALESCE(entry_url,'') FROM assessment_task WHERE org_id=? AND id=? AND deleted_at IS NULL")).
		WithArgs(int64(1), uint64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version", "status", "planned_at", "due_at", "open_at", "expire_at", "completed_at", "expired_at", "canceled_at", "expiration_reason", "assessment_id", "entry_token", "entry_url"}).
			AddRow(10, 3, "pending", planned, nil, nil, nil, nil, nil, nil, nil, nil, "", ""))

	changed, err := rollbackTask(context.Background(), db, config{orgID: 1}, before, after)
	if err != nil || changed {
		t.Fatalf("uncommitted audited row should be skipped: changed=%v err=%v", changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
