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
