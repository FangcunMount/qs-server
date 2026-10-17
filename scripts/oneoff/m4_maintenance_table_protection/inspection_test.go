package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInspectionSelectsOnlyAnActualCompleteClean102Or103Ledger(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version uint64
		dirty   bool
		rows    int
		failure string
		want    uint64
	}{
		{"clean102", 102, false, 1, "", 102},
		{"clean103", 103, false, 1, "", 103},
		{"dirty102", 102, true, 1, "", 0},
		{"dirty103", 103, true, 1, "", 0},
		{"old101", 101, false, 1, "", 0},
		{"future104", 104, false, 1, "", 0},
		{"unknown", 999, false, 1, "", 0},
		{"empty", 0, false, 0, "", 0},
		{"multiple", 102, false, 2, "", 0},
		{"query_failure", 103, false, 1, "query", 0},
		{"row_failure", 103, false, 1, "row", 0},
		{"close_failure", 103, false, 1, "close", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := dbFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			query := mock.ExpectQuery(regexp.QuoteMeta("SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2"))
			if tc.failure == "query" {
				query.WillReturnError(errors.New("owned unavailable ledger"))
			} else {
				rows := sqlmock.NewRows([]string{"version", "dirty"})
				for range tc.rows {
					rows.AddRow(tc.version, tc.dirty)
				}
				if tc.failure == "row" {
					rows.RowError(0, errors.New("owned incomplete ledger"))
				}
				if tc.failure == "close" {
					rows.CloseError(errors.New("owned ledger release unknown"))
				}
				query.WillReturnRows(rows)
			}
			got, e := readInspectionVersion(ctx, db)
			if got != tc.want || (e == nil) != (tc.want != 0) {
				t.Fatalf("version selection=%d error=%v", got, e)
			}
		})
	}
}

func TestInspectionExpiredContextDoesNotQueryTheLedger(t *testing.T) {
	db, _ := dbFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if version, e := readInspectionVersion(ctx, db); e == nil || e.Error() != "inspection_deadline_exhausted" || version != 0 {
		t.Fatal("expired inspection selected a version", version, e)
	}
}

func TestInspectionFieldDoesNotChangeSuccessfulCaptureStdout(t *testing.T) {
	m := originalMaterial()
	r := unitCaptureReceipt(m, fingerprint([]byte("inspect backward-compatible capture")))
	var stdout bytes.Buffer
	if emitReceipt(r, &stdout) != 0 || fingerprint(bytes.Replace(stdout.Bytes(), []byte(dSource), []byte("c335b2a1a34dd69830261e2d8e11b16da3f8befb"), 1)) != "1a5905e6428e15c382b0e5c9c714d4101e8a4d756ca6abeb0d6425c13cbcfb71" || bytes.Contains(stdout.Bytes(), []byte("inspected_sql_version")) {
		t.Fatal("capture stdout encoding apart from the release binding changed from the independently recorded 506b baseline")
	}
	var decoded receipt
	if decodeCaptureDocument(stdout.Bytes(), &decoded) != nil || decoded.InspectedSQLVersion != nil {
		t.Fatal("original successful capture stdout no longer qualifies")
	}
}

func TestInspectionFieldCannotBecomeSuccessfulCaptureEvidence(t *testing.T) {
	for _, value := range []string{"103", "102", "0", "null", "true", `"103"`} {
		t.Run(value, func(t *testing.T) {
			m, a, hash, ap, ah := recoveryInputs(t)
			proofPath, proofHash, stdoutPath := unitCaptureEvidence(t, unitCaptureReceipt(m, hash), 0)
			if e := readCaptureEvidence(m, hash, testTool, proofPath, proofHash); e != nil {
				t.Fatal("qualifying capture evidence failed before the negative scenario", e)
			}
			stdout, e := os.ReadFile(stdoutPath)
			if e != nil {
				t.Fatal(e)
			}
			stdout = []byte(strings.TrimSuffix(strings.TrimSpace(string(stdout)), "}") + `,"inspected_sql_version":` + value + "}\n")
			if os.WriteFile(stdoutPath, stdout, 0600) != nil {
				t.Fatal("owned stdout input unavailable")
			}
			proofRaw, e := os.ReadFile(proofPath)
			var proof captureExecutionEvidence
			if e != nil || strictDecode(proofRaw, &proof) != nil {
				t.Fatal("owned execution input unavailable")
			}
			proof.ReceiptHash = fingerprint(stdout)
			proofRaw, e = json.Marshal(proof)
			if e != nil || os.WriteFile(proofPath, proofRaw, 0600) != nil {
				t.Fatal("owned frozen execution input unavailable")
			}
			for _, action := range []string{"verify-target", "restore", "verify-restored", "mark-first-runtime"} {
				args := materialConsumerArgs(t, action, m, a, hash, ap, ah)
				args = append(args, "--capture-evidence-file="+proofPath, "--capture-evidence-sha256="+fingerprint(proofRaw))
				r := run(t.Context(), args, func(string) string { t.Fatal("inspection-bearing capture proof reached ENV", action); return "" }, testTool)
				if r.Complete || r.Error != "original_capture_success_receipt_rejected" || r.ApprovalConsumed || r.RuntimeGateAcquired || r.MarkerWritten {
					t.Fatal("inspection field admitted into a capture-only success proof", action, r)
				}
			}
		})
	}
}
