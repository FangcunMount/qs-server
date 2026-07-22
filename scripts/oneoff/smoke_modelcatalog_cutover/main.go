package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	exitOK          = 0
	exitUnavailable = 1
	exitFailed      = 2
)

type config struct {
	CollectionBaseURL string
	Token             string
	TokenFile         string
	TesteeID          string
	ModelCodes        []string
	Timeout           time.Duration
	PollInterval      time.Duration
	OutputPath        string
	Title             string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		_, _ = fmt.Fprintf(stderr, "modelcatalog smoke: configuration: %v\n", err)
		return exitUnavailable
	}

	client := newSmokeClient(cfg.CollectionBaseURL, cfg.Token, cfg.PollInterval)
	ctx, cancel := context.WithTimeout(context.Background(), minDuration(cfg.Timeout, 30*time.Second))
	err = client.checkReady(ctx)
	cancel()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "modelcatalog smoke: collection readiness unavailable: %v\n", err)
		return exitUnavailable
	}

	runResult := smokeRunResult{
		StartedAt:         time.Now().UTC(),
		CollectionBaseURL: cfg.CollectionBaseURL,
		TesteeID:          cfg.TesteeID,
		Results:           make([]smokeCaseResult, 0, len(cfg.ModelCodes)),
	}

	for _, modelCode := range cfg.ModelCodes {
		_, _ = fmt.Fprintf(stdout, "SMOKE start model=%s testee=%s\n", modelCode, cfg.TesteeID)
		caseCtx, caseCancel := context.WithTimeout(context.Background(), cfg.Timeout)
		result := client.runCase(caseCtx, cfg.TesteeID, modelCode, cfg.Title)
		caseCancel()
		runResult.Results = append(runResult.Results, result)
		if result.Passed {
			_, _ = fmt.Fprintf(stdout, "SMOKE PASS model=%s kind=%s algorithm=%s answersheet=%s assessment=%s level=%s norm_refs=%d duration=%s\n",
				result.Model.Code, result.Model.Kind, result.Model.Algorithm, result.AnswerSheetID,
				result.AssessmentID, result.Level.Code, result.NormReferenceCount, result.Duration)
			continue
		}
		_, _ = fmt.Fprintf(stdout, "SMOKE FAIL model=%s step=%s error=%s duration=%s\n",
			modelCode, result.FailedStep, result.Error, result.Duration)
	}

	runResult.FinishedAt = time.Now().UTC()
	runResult.Passed = allPassed(runResult.Results)
	if err := writeResult(cfg.OutputPath, runResult); err != nil {
		_, _ = fmt.Fprintf(stderr, "modelcatalog smoke: write result: %v\n", err)
		return exitUnavailable
	}
	if cfg.OutputPath != "" {
		_, _ = fmt.Fprintf(stdout, "SMOKE evidence=%s\n", cfg.OutputPath)
	}

	if !runResult.Passed {
		_, _ = fmt.Fprintf(stdout, "MODELCATALOG_SMOKE_FAILED passed=%d failed=%d\n", passedCount(runResult.Results), failedCount(runResult.Results))
		return exitFailed
	}
	_, _ = fmt.Fprintf(stdout, "MODELCATALOG_SMOKE_OK passed=%d failed=0\n", len(runResult.Results))
	return exitOK
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("smoke_modelcatalog_cutover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	baseURL := fs.String("collection-base-url", envOr("QS_MODELCATALOG_SMOKE_COLLECTION_URL", "https://collect.fangcunmount.cn"), "collection-server base URL")
	token := fs.String("token", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_SMOKE_TOKEN")), "Bearer token; prefer environment or --token-file")
	tokenFile := fs.String("token-file", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_SMOKE_TOKEN_FILE")), "text or perf tokens JSON file; first token is used")
	testeeID := fs.String("testee-id", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_SMOKE_TESTEE_ID")), "testee ID used for every smoke case")
	modelCodes := fs.String("model-codes", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_SMOKE_MODEL_CODES")), "comma-separated published model codes")
	timeout := fs.Duration("timeout", envDuration("QS_MODELCATALOG_SMOKE_TIMEOUT", 5*time.Minute), "timeout for each model")
	pollInterval := fs.Duration("poll-interval", envDuration("QS_MODELCATALOG_SMOKE_POLL_INTERVAL", time.Second), "minimum readiness/report polling interval")
	outputPath := fs.String("output", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_SMOKE_OUTPUT")), "optional JSON evidence output path")
	title := fs.String("title", "ModelCatalog cutover smoke", "answer sheet title")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	cfg := config{
		CollectionBaseURL: strings.TrimRight(strings.TrimSpace(*baseURL), "/"),
		Token:             strings.TrimSpace(*token),
		TokenFile:         strings.TrimSpace(*tokenFile),
		TesteeID:          strings.TrimSpace(*testeeID),
		ModelCodes:        splitCSV(*modelCodes),
		Timeout:           *timeout,
		PollInterval:      *pollInterval,
		OutputPath:        strings.TrimSpace(*outputPath),
		Title:             strings.TrimSpace(*title),
	}
	if cfg.CollectionBaseURL == "" {
		return config{}, errors.New("--collection-base-url is required")
	}
	if cfg.Token == "" && cfg.TokenFile != "" {
		var err error
		cfg.Token, err = readFirstToken(cfg.TokenFile)
		if err != nil {
			return config{}, fmt.Errorf("read --token-file: %w", err)
		}
	}
	if cfg.Token == "" {
		return config{}, errors.New("a collection Bearer token is required via --token, --token-file, QS_MODELCATALOG_SMOKE_TOKEN, or QS_MODELCATALOG_SMOKE_TOKEN_FILE")
	}
	if cfg.TesteeID == "" {
		return config{}, errors.New("--testee-id is required")
	}
	if len(cfg.ModelCodes) == 0 {
		return config{}, errors.New("--model-codes must contain at least one code")
	}
	if cfg.Timeout <= 0 {
		return config{}, errors.New("--timeout must be positive")
	}
	if cfg.PollInterval <= 0 {
		return config{}, errors.New("--poll-interval must be positive")
	}
	return cfg, nil
}

func readFirstToken(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(string(contents))
	if trimmed == "" {
		return "", errors.New("token file is empty")
	}
	var list []string
	if json.Unmarshal(contents, &list) == nil && len(list) > 0 && strings.TrimSpace(list[0]) != "" {
		return strings.TrimSpace(list[0]), nil
	}
	var object struct {
		Tokens []string `json:"tokens"`
	}
	if json.Unmarshal(contents, &object) == nil && len(object.Tokens) > 0 && strings.TrimSpace(object.Tokens[0]) != "" {
		return strings.TrimSpace(object.Tokens[0]), nil
	}
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return strings.TrimSpace(strings.Split(trimmed, "\n")[0]), nil
	}
	return "", errors.New("expected a token string, JSON token array, or {\"tokens\":[...]} object")
}

func writeResult(path string, result smokeRunResult) error {
	if path == "" {
		return nil
	}
	contents, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return os.WriteFile(path, contents, 0o600)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func allPassed(results []smokeCaseResult) bool {
	return len(results) > 0 && failedCount(results) == 0
}

func passedCount(results []smokeCaseResult) int {
	count := 0
	for _, result := range results {
		if result.Passed {
			count++
		}
	}
	return count
}

func failedCount(results []smokeCaseResult) int {
	return len(results) - passedCount(results)
}
