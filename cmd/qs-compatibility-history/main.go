// qs-compatibility-history hosts bounded historical qualification and the
// explicitly selected evidence CAS pipeline. It has no migration, send or DROP.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type historyError string

func (e historyError) Error() string { return string(e) }
func safeCategory(err error) string {
	if err == nil {
		return "none"
	}
	var fixed historyError
	if errors.As(err, &fixed) {
		return fixed.Error()
	}
	return "history_unclassified_failure"
}
func writeReadiness(directory string, r readiness) (result error) {
	path := filepath.Join(directory, "history.readiness.json")
	if privateParent(path) != nil {
		return fixedError("history_private_output_rejected")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fixedError("history_private_output_exists_or_unavailable")
	}
	defer func() {
		if f.Close() != nil && result == nil {
			result = fixedError("history_private_output_close_failed")
		}
	}()
	if json.NewEncoder(f).Encode(r) != nil || f.Sync() != nil {
		return fixedError("history_private_output_failed")
	}
	dir, err := os.Open(directory)
	if err != nil {
		return fixedError("history_private_output_failed")
	}
	defer func() {
		if dir.Close() != nil && result == nil {
			result = fixedError("history_private_output_close_failed")
		}
	}()
	if dir.Sync() != nil {
		return fixedError("history_private_output_failed")
	}
	return nil
}
func parseFlags(args []string) (map[string]string, error) {
	supported := map[string]bool{"request": true, "request-sha256": true, "operation": true, "run": true, "output": true}
	seen := map[string]bool{}
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) || !strings.HasPrefix(args[i], "--") {
			return nil, fixedError("history_arguments_rejected")
		}
		key := strings.TrimPrefix(args[i], "--")
		if !supported[key] || seen[key] || strings.HasPrefix(args[i+1], "--") {
			return nil, fixedError("history_arguments_rejected")
		}
		seen[key] = true
	}
	if len(seen) != len(supported) {
		return nil, fixedError("history_arguments_rejected")
	}
	set := flag.NewFlagSet("qs-compatibility-history", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	values := map[string]*string{}
	for key := range supported {
		values[key] = set.String(key, "", "")
	}
	if set.Parse(args) != nil || set.NArg() != 0 {
		return nil, fixedError("history_arguments_rejected")
	}
	out := map[string]string{}
	for key, p := range values {
		out[key] = *p
	}
	return out, nil
}

// Cleanup is shared by successful finalization and all early returns. Both
// resources are attempted exactly once, even when the first close fails.
func readOnlyCleanup(closeDatabase, closeInputs func() error) func() error {
	closed := false
	return func() error {
		if closed {
			return nil
		}
		closed = true
		var result error
		if closeDatabase != nil && closeDatabase() != nil {
			result = fixedError("history_connection_close_failed")
		}
		if closeInputs != nil && closeInputs() != nil && result == nil {
			result = fixedError("history_private_close_failed")
		}
		return result
	}
}

func finishReadOnlyCLI(directory string, r readiness, result error, cleanup func() error) (readiness, error) {
	if err := cleanup(); result == nil && err != nil {
		result = err
	}
	r.ErrorCategory = safeCategory(result)
	// The private artifact and final stdout now describe the same close result.
	if err := writeReadiness(directory, r); result == nil && err != nil {
		result = err
		r.ErrorCategory = safeCategory(result)
	}
	return r, result
}

func runCLI(ctx context.Context, args []string) (r readiness, result error) {
	r = emptyReadiness(nil)
	flags, err := parseFlags(args)
	if err != nil {
		return r, err
	}
	if privateParent(filepath.Join(flags["output"], "history.readiness.json")) != nil {
		return r, fixedError("history_private_output_rejected")
	}
	a, err := loadInputs(ctx, flags["request"], flags["request-sha256"], flags["operation"], flags["run"])
	if err != nil {
		return r, err
	}
	r = emptyReadiness(a)
	var db *historyDatabase
	cleanup := readOnlyCleanup(func() error {
		if db == nil {
			return nil
		}
		return db.close()
	}, a.close)
	defer func() {
		if err := cleanup(); result == nil && err != nil {
			result = err
			r.ErrorCategory = safeCategory(result)
		}
	}()
	db, err = openDatabases(ctx, a)
	if err != nil {
		return r, err
	}
	r, result = executePipeline(ctx, a, db)
	return finishReadOnlyCLI(flags["output"], r, result, cleanup)
}
func main() {
	if len(os.Args) == 2 && os.Args[1] == "--source-sha" {
		if !sourcePattern.MatchString(sourceSHA) {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error_category": "history_build_source_rejected"})
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"source_sha": sourceSHA})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	if len(os.Args) > 1 && os.Args[1] == "--write-mode" {
		r, err := runEvidenceWriteCLI(ctx, os.Args[1:])
		cancel()
		r.ErrorCategory = safeCategory(err)
		if json.NewEncoder(os.Stdout).Encode(r) != nil || err != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--ai-host-mode" {
		r, err := runAIHostCLI(ctx, os.Args[1:])
		cancel()
		r.ErrorCategory = safeCategory(err)
		if json.NewEncoder(os.Stdout).Encode(r) != nil || err != nil {
			os.Exit(1)
		}
		return
	}
	r, err := runCLI(ctx, os.Args[1:])
	cancel()
	r.ErrorCategory = safeCategory(err)
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
}
