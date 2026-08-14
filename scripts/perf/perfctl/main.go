package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root, _ := os.Getwd()
	plan := flag.String("plan", envString("PLAN", "quick"), "quick, baseline, ceiling-120, admission, or diagnose")
	diagnostic := flag.String("case", envString("CASE", ""), "registered diagnostic case")
	config := flag.String("config", envString("PERF_CONFIG_FILE", filepath.Join(root, "tmp/perf/qs-perf.config.json")), "perf JSON config")
	output := flag.String("output", envString("PERF_RUN_ROOT", filepath.Join(root, "tmp/perf/runs")), "run output root")
	script := flag.String("script", envString("PERF_K6_SCRIPT", filepath.Join(root, "scripts/perf/k6/mixed.js")), "k6 entry script")
	dryRun := flag.Bool("dry-run", envBool("PERF_DRY_RUN"), "print the plan without running load")
	flag.Parse()
	_, exitCode, err := execute(context.Background(), runOptions{
		Plan: *plan, Case: *diagnostic, Root: root, ConfigFile: *config,
		OutputRoot: *output, K6Script: *script, DryRun: *dryRun,
		Stdout: os.Stdout, Stderr: os.Stderr,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
	os.Exit(exitCode)
}

func envString(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
