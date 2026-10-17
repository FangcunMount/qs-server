package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

type graphApproval struct {
	approval
	BeforeReopen    bool      `json:"restoration_before_business_reopen"`
	RecoveryStarted time.Time `json:"recovery_started_at"`
	RecoveryExpires time.Time `json:"recovery_expires_at"`
}
type graphRecoveryBoundary struct {
	Format     int       `json:"format_version"`
	Kind       string    `json:"kind"`
	Operation  string    `json:"operation_id"`
	GroupSHA   string    `json:"group_manifest_sha256"`
	ArchiveSHA string    `json:"archive_sha256"`
	Started    time.Time `json:"recovery_started_at"`
	Expires    time.Time `json:"recovery_expires_at"`
}
type graphCaptureExecution struct {
	Format      int    `json:"format_version"`
	Kind        string `json:"kind"`
	ReceiptFile string `json:"receipt_file"`
	ReceiptSHA  string `json:"receipt_sha256"`
	Termination string `json:"termination"`
	Exit        *int   `json:"exit_code"`
}

func graphForwardGuard(dir string) error {
	_, e := os.Lstat(filepath.Join(dir, "forward-only.json"))
	if e == nil {
		return errors.New("business reopen boundary exists; only forward recovery is allowed")
	}
	if !errors.Is(e, os.ErrNotExist) {
		return errors.New("business reopen boundary cannot be proven absent")
	}
	return nil
}
func graphCheckApproval(a graphApproval, m graphArchive, g group, archiveSHA, action, source string, now time.Time) error {
	if a.Format != 1 || a.Kind != "mock_cohort_action_approval" || a.Action != action || a.Operation != m.Operation || a.GroupSHA != m.GroupSHA || a.ProtectedSHA != m.ProtectionSHA || a.ArchiveSHA != archiveSHA || a.ToolSource != source || m.Source != source || g.Operation != m.Operation || !hashPattern.MatchString(a.FreezeSHA) || a.FreezeSHA != a.SettlementSHA || !a.Issued.Before(now) || !now.Before(a.Expires) {
		return errors.New("independent full graph approval does not bind original exact operation")
	}
	if action == "restore" {
		if !a.BeforeReopen || a.RecoveryStarted.Before(m.Graph.WindowStarted) || a.RecoveryStarted.After(a.Issued) || a.RecoveryStarted.After(m.Graph.WindowExpires) || !a.RecoveryStarted.Before(a.RecoveryExpires) || a.RecoveryExpires.Sub(a.RecoveryStarted) > time.Hour || a.Expires.After(a.RecoveryExpires) {
			return errors.New("recovery must bind one fixed decision and at most sixty minutes")
		}
		return graphForwardGuard(m.Graph.ArchiveDirectory)
	}
	if a.BeforeReopen || !a.RecoveryStarted.IsZero() || !a.RecoveryExpires.IsZero() || a.Issued.Before(m.Graph.WindowStarted) || a.Expires.After(m.Graph.WindowExpires) || a.Expires.Sub(a.Issued) > 30*time.Minute {
		return errors.New("apply cannot reset or extend the original maintenance window")
	}
	return graphForwardGuard(m.Graph.ArchiveDirectory)
}
func graphRecoveryClaim(dir string, m graphArchive, archiveSHA string, a graphApproval) error {
	if e := graphForwardGuard(dir); e != nil {
		return e
	}
	expected := graphRecoveryBoundary{1, "mock_cohort_recovery_boundary", m.Operation, m.GroupSHA, archiveSHA, a.RecoveryStarted, a.RecoveryExpires}
	path := filepath.Join(dir, "recovery-boundary.json")
	raw, e := privateRead(path, 32*1024)
	if errors.Is(e, os.ErrNotExist) {
		if !time.Now().Before(m.Graph.WindowExpires) {
			return errors.New("first recovery entry cannot begin after the original thirty-minute window")
		}
		return writeNew(path, encode(expected))
	}
	if e != nil {
		return e
	}
	var actual graphRecoveryBoundary
	if e = strictDecode(raw, &actual); e != nil {
		return e
	}
	if !reflect.DeepEqual(actual, expected) {
		return errors.New("recovery retry cannot reset the original decision/deadline")
	}
	return nil
}
func graphActionContext(ctx context.Context, m graphArchive, a graphApproval, restore bool) (context.Context, context.CancelFunc, error) {
	deadline := m.Graph.WindowExpires.Add(-5 * time.Minute)
	if restore {
		deadline = a.RecoveryExpires.Add(-5 * time.Minute)
	}
	if !time.Now().Before(deadline) {
		return nil, nil, errors.New("fixed action deadline or startup reserve is exhausted")
	}
	if a.Expires.Before(deadline) {
		deadline = a.Expires
	}
	next, cancel := context.WithDeadline(ctx, deadline)
	return next, cancel, nil
}
func readGraphCaptureProof(file, sha string, m graphArchive, materialSHA string) error {
	var evidence graphCaptureExecution
	if e := readExact(file, sha, 32*1024, &evidence); e != nil {
		return e
	}
	if evidence.Format != 1 || evidence.Kind != "mock_cohort_capture_execution" || evidence.Termination != "exited" || evidence.Exit == nil || *evidence.Exit != 0 || !filepath.IsAbs(evidence.ReceiptFile) || !hashPattern.MatchString(evidence.ReceiptSHA) {
		return errors.New("original successful capture process evidence required")
	}
	var r actionReceipt
	if e := readExact(evidence.ReceiptFile, evidence.ReceiptSHA, 64*1024, &r); e != nil {
		return e
	}
	if r.Format != 1 || r.Kind != "mock_cohort_qs_action_result" || r.Action != "capture" || r.Operation != m.Operation || r.GroupSHA != m.GroupSHA || r.Source != m.Source || r.MaterialSHA != materialSHA || !r.Complete || r.DryRun || r.Whole || r.Pending || r.Exclusions != 0 || !r.Released {
		return errors.New("original capture stdout cannot qualify this complete graph archive")
	}
	return nil
}

func graphReadContext(ctx context.Context, m graphArchive, materialSHA string, original bool) (context.Context, context.CancelFunc, error) {
	if e := graphForwardGuard(m.Graph.ArchiveDirectory); e != nil {
		return nil, nil, e
	}
	if !original {
		return graphAbsoluteContext(ctx, m.Graph)
	}
	raw, e := privateRead(filepath.Join(m.Graph.ArchiveDirectory, "recovery-boundary.json"), 32*1024)
	if errors.Is(e, os.ErrNotExist) {
		return graphAbsoluteContext(ctx, m.Graph)
	}
	if e != nil {
		return nil, nil, e
	}
	var boundary graphRecoveryBoundary
	if e = strictDecode(raw, &boundary); e != nil {
		return nil, nil, e
	}
	if boundary.Format != 1 || boundary.Kind != "mock_cohort_recovery_boundary" || boundary.Operation != m.Operation || boundary.GroupSHA != m.GroupSHA || boundary.ArchiveSHA != materialSHA || boundary.Started.Before(m.Graph.WindowStarted) || boundary.Started.After(m.Graph.WindowExpires) || !boundary.Started.Before(boundary.Expires) || boundary.Expires.Sub(boundary.Started) > time.Hour || time.Now().Before(boundary.Started) || !time.Now().Before(boundary.Expires.Add(-5*time.Minute)) {
		return nil, nil, errors.New("original verification must retain the fixed recovery decision/deadline")
	}
	next, cancel := context.WithDeadline(ctx, boundary.Expires.Add(-5*time.Minute))
	return next, cancel, nil
}
