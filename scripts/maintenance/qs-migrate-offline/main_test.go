package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/pkg/version"
)

func TestOfflineArgumentsRejectAmbiguousOrBypassModes(t *testing.T) {
	base := []string{"--mysql-database=qs", "--mongo-database=qs"}
	for _, extra := range [][]string{{"--force=100"}, {"--down"}, {"--source-sha=" + strings.Repeat("a", 40)}, {"--auto-seed"}, {"--timeout=31m"}, {"--timeout=0"}, {"--apply"}, {"--approval-file=/tmp/x"}, {"extra"}} {
		if _, err := parseArguments(append(append([]string{}, base...), extra...)); err == nil {
			t.Fatal("unsafe invocation accepted", extra)
		}
	}
	if a, err := parseArguments(base); err != nil || a.apply {
		t.Fatal("default must be inspection", a, err)
	}
	if a, err := parseArguments(append(base, "--apply", "--approval-file=/tmp/approval", "--approval-sha256="+strings.Repeat("a", 64))); err != nil || !a.apply {
		t.Fatal(a, err)
	}
}

func TestOfflineInvalidInputDoesNotReadCredentialsOrConnect(t *testing.T) {
	reads := 0
	getenv := func(string) string { reads++; return "sensitive" }
	old := version.GitCommit
	t.Cleanup(func() { version.GitCommit = old })
	version.GitCommit = "development"
	if err := run(context.Background(), []string{"--mysql-database=qs", "--mongo-database=qs"}, getenv, io.Discard); err == nil || reads != 0 {
		t.Fatal(err, reads)
	}
	version.GitCommit = strings.Repeat("a", 40)
	if err := run(context.Background(), []string{"--force=100"}, getenv, io.Discard); err == nil || reads != 0 {
		t.Fatal(err, reads)
	}
}

func TestOfflineProtocolActionIsBoundedAndCannotApply(t *testing.T) {
	base := []string{"--mysql-database=qs", "--mongo-database=qs", "--protocol-observation", "--timeout=30s"}
	if a, err := parseArguments(base); err != nil || !a.protocolObservation || a.apply {
		t.Fatal(a, err)
	}
	for _, extra := range [][]string{{"--apply", "--approval-file=/tmp/approval", "--approval-sha256=" + strings.Repeat("a", 64)}, {"--timeout=2m"}, {"--approval-file=/tmp/approval"}} {
		if _, err := parseArguments(append(append([]string{}, base...), extra...)); err == nil {
			t.Fatal("ambiguous observation accepted", extra)
		}
	}
}

func TestRetainedBaselineCannotMutateOrSilentlyTruncate(t *testing.T) {
	base := []string{"--mysql-database=qs", "--mongo-database=qs", "--retained-baseline", "--timeout=2m"}
	if a, err := parseArguments(base); err != nil || !a.retainedBaseline {
		t.Fatal(a, err)
	}
	for _, extra := range [][]string{{"--protocol-observation"}, {"--apply", "--approval-file=/tmp/x", "--approval-sha256=" + strings.Repeat("a", 64)}, {"--baseline-max-rows=0"}, {"--baseline-max-rows=1000001"}, {"--timeout=3m"}} {
		if _, err := parseArguments(append(append([]string{}, base...), extra...)); err == nil {
			t.Fatal("unsafe baseline invocation", extra)
		}
	}
}

func TestOriginalRestoreRequiresSeparatePrivateMaterialAndApproval(t *testing.T) {
	base := []string{"--mysql-database=qs", "--mongo-database=qs", "--timeout=90s"}
	material := append(append([]string{}, base...), "--restore-original=/private/original.json", "--restore-original-sha256="+strings.Repeat("a", 64))
	if a, err := parseArguments(material); err != nil || a.restore.apply {
		t.Fatal(a, err)
	}
	for _, extra := range [][]string{{"--restore-apply"}, {"--retained-baseline"}, {"--protocol-observation"}, {"--capture-original-operation=owned"}, {"--restore-approval-file=/private/approval.json", "--restore-approval-sha256=" + strings.Repeat("b", 64)}, {"--timeout=11m"}} {
		if _, err := parseArguments(append(append([]string{}, material...), extra...)); err == nil {
			t.Fatal("ambiguous restoration accepted", extra)
		}
	}
	if a, err := parseArguments(append(material, "--restore-apply", "--restore-approval-file=/private/approval.json", "--restore-approval-sha256="+strings.Repeat("b", 64))); err != nil || !a.restore.apply {
		t.Fatal(a, err)
	}
	if _, err := parseArguments(append(base, "--capture-original-operation=owned-m5")); err != nil {
		t.Fatal(err)
	}
	if _, err := parseArguments(append(base, "--capture-original-operation=owned-m5", "--retained-baseline")); err == nil {
		t.Fatal("ambiguous original capture accepted")
	}
}
