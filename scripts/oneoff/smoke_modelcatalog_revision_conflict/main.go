// smoke_modelcatalog_revision_conflict verifies the deployed REST mapping for
// ModelCatalog and Questionnaire optimistic-lock conflicts.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	exitOK          = 0
	exitUnavailable = 1
	exitFailed      = 2
)

type config struct {
	APIBaseURL         string
	Token              string
	TokenFile          string
	ModelCode          string
	QuestionnaireCode  string
	RequiredCodePrefix string
	Concurrency        int
	Rounds             int
	Timeout            time.Duration
	OutputPath         string
	Apply              bool
}

type runEvidence struct {
	Mode            string           `json:"mode"`
	StartedAt       time.Time        `json:"started_at"`
	FinishedAt      time.Time        `json:"finished_at"`
	APIBaseURL      string           `json:"api_base_url"`
	Concurrency     int              `json:"concurrency"`
	MaxRounds       int              `json:"max_rounds"`
	PreflightPassed bool             `json:"preflight_passed"`
	Passed          bool             `json:"passed"`
	Targets         []targetEvidence `json:"targets"`
	Error           string           `json:"error,omitempty"`
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
		_, _ = fmt.Fprintf(stderr, "modelcatalog revision-conflict smoke: configuration: %v\n", err)
		return exitUnavailable
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	client := newRESTClient(cfg.APIBaseURL, cfg.Token)
	if err := client.checkReady(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "modelcatalog revision-conflict smoke: apiserver readiness unavailable: %v\n", err)
		return exitUnavailable
	}

	evidence := runEvidence{
		Mode:        "dry-run",
		StartedAt:   time.Now().UTC(),
		APIBaseURL:  cfg.APIBaseURL,
		Concurrency: cfg.Concurrency,
		MaxRounds:   cfg.Rounds,
		Targets:     make([]targetEvidence, 0, 2),
	}
	if cfg.Apply {
		evidence.Mode = "apply"
	}

	targets := []targetSpec{
		{Kind: targetModel, Code: cfg.ModelCode},
		{Kind: targetQuestionnaire, Code: cfg.QuestionnaireCode},
	}
	snapshots := make([]targetSnapshot, 0, len(targets))
	for _, target := range targets {
		snapshot, getErr := client.getSnapshot(ctx, target)
		if getErr != nil {
			evidence.Error = fmt.Sprintf("preflight %s %s: %v", target.Kind, target.Code, getErr)
			return finishRun(cfg, stdout, stderr, evidence, exitUnavailable)
		}
		if guardErr := validateDedicatedDraft(snapshot, cfg.RequiredCodePrefix); guardErr != nil {
			evidence.Error = fmt.Sprintf("preflight %s %s: %v", target.Kind, target.Code, guardErr)
			return finishRun(cfg, stdout, stderr, evidence, exitUnavailable)
		}
		snapshots = append(snapshots, snapshot)
		evidence.Targets = append(evidence.Targets, newPreflightEvidence(snapshot))
	}
	if snapshots[0].Model.QuestionnaireCode != cfg.QuestionnaireCode {
		evidence.Error = fmt.Sprintf("preflight model %s is bound to questionnaire %q, want dedicated target %q",
			cfg.ModelCode, snapshots[0].Model.QuestionnaireCode, cfg.QuestionnaireCode)
		return finishRun(cfg, stdout, stderr, evidence, exitUnavailable)
	}
	evidence.PreflightPassed = true

	if !cfg.Apply {
		_, _ = fmt.Fprintln(stdout, "REVISION_CONFLICT_SMOKE_PREFLIGHT_OK: targets are dedicated unpublished drafts; rerun with --apply")
		evidence.Passed = false
		return finishRun(cfg, stdout, stderr, evidence, exitOK)
	}

	for index, snapshot := range snapshots {
		_, _ = fmt.Fprintf(stdout, "SMOKE start target=%s code=%s concurrency=%d rounds=%d\n",
			snapshot.Spec.Kind, snapshot.Spec.Code, cfg.Concurrency, cfg.Rounds)
		result := exerciseTarget(ctx, client, snapshot, cfg.Concurrency, cfg.Rounds)
		evidence.Targets[index] = result
		if result.Passed {
			_, _ = fmt.Fprintf(stdout, "SMOKE PASS target=%s code=%s successes=%d revision_conflicts=%d rounds=%d restored=%t\n",
				result.Kind, result.Code, result.Successes, result.RevisionConflicts, len(result.Rounds), result.RestorePassed)
		} else {
			_, _ = fmt.Fprintf(stdout, "SMOKE FAIL target=%s code=%s successes=%d revision_conflicts=%d unexpected=%d restored=%t error=%s\n",
				result.Kind, result.Code, result.Successes, result.RevisionConflicts, result.Unexpected, result.RestorePassed, result.Error)
		}
		if result.RestoreAttempted && !result.RestorePassed {
			evidence.Error = fmt.Sprintf("restore failed for %s %s; stopped before mutating another target", result.Kind, result.Code)
			break
		}
	}

	evidence.Passed = len(evidence.Targets) == 2
	for _, target := range evidence.Targets {
		evidence.Passed = evidence.Passed && target.Passed
	}
	if !evidence.Passed {
		return finishRun(cfg, stdout, stderr, evidence, exitFailed)
	}
	_, _ = fmt.Fprintln(stdout, "MODELCATALOG_REVISION_CONFLICT_SMOKE_OK")
	return finishRun(cfg, stdout, stderr, evidence, exitOK)
}

func finishRun(cfg config, stdout, stderr io.Writer, evidence runEvidence, exitCode int) int {
	evidence.FinishedAt = time.Now().UTC()
	if err := writeEvidence(cfg.OutputPath, evidence); err != nil {
		_, _ = fmt.Fprintf(stderr, "modelcatalog revision-conflict smoke: write evidence: %v\n", err)
		return exitUnavailable
	}
	if cfg.OutputPath != "" {
		_, _ = fmt.Fprintf(stdout, "SMOKE evidence=%s\n", cfg.OutputPath)
	}
	if evidence.Error != "" {
		_, _ = fmt.Fprintf(stderr, "modelcatalog revision-conflict smoke: %s\n", evidence.Error)
	}
	return exitCode
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("smoke_modelcatalog_revision_conflict", flag.ContinueOnError)
	fs.SetOutput(stderr)
	apiBaseURL := fs.String("api-base-url", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_CONFLICT_API_URL")), "qs-apiserver base URL")
	token := fs.String("token", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_CONFLICT_TOKEN")), "Bearer token; prefer environment or --token-file")
	tokenFile := fs.String("token-file", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_CONFLICT_TOKEN_FILE")), "text or perf tokens JSON file; first token is used")
	modelCode := fs.String("model-code", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_CONFLICT_MODEL_CODE")), "dedicated unpublished draft model code")
	questionnaireCode := fs.String("questionnaire-code", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_CONFLICT_QUESTIONNAIRE_CODE")), "dedicated unpublished draft questionnaire code bound to the model")
	requiredPrefix := fs.String("required-code-prefix", envOr("QS_MODELCATALOG_CONFLICT_CODE_PREFIX", "SMOKE_"), "required prefix for both dedicated target codes")
	concurrency := fs.Int("concurrency", envInt("QS_MODELCATALOG_CONFLICT_CONCURRENCY", 16), "simultaneous PUT requests per round")
	rounds := fs.Int("rounds", envInt("QS_MODELCATALOG_CONFLICT_ROUNDS", 5), "maximum rounds per target")
	timeout := fs.Duration("timeout", envDuration("QS_MODELCATALOG_CONFLICT_TIMEOUT", 2*time.Minute), "overall smoke timeout")
	output := fs.String("output", strings.TrimSpace(os.Getenv("QS_MODELCATALOG_CONFLICT_OUTPUT")), "optional JSON evidence output path")
	apply := fs.Bool("apply", false, "execute concurrent writes and restore the original basic info")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	cfg := config{
		APIBaseURL:         strings.TrimRight(strings.TrimSpace(*apiBaseURL), "/"),
		Token:              strings.TrimSpace(*token),
		TokenFile:          strings.TrimSpace(*tokenFile),
		ModelCode:          strings.TrimSpace(*modelCode),
		QuestionnaireCode:  strings.TrimSpace(*questionnaireCode),
		RequiredCodePrefix: strings.TrimSpace(*requiredPrefix),
		Concurrency:        *concurrency,
		Rounds:             *rounds,
		Timeout:            *timeout,
		OutputPath:         strings.TrimSpace(*output),
		Apply:              *apply,
	}
	if err := validateBaseURL(cfg.APIBaseURL); err != nil {
		return config{}, err
	}
	if cfg.Token == "" && cfg.TokenFile != "" {
		var readErr error
		cfg.Token, readErr = readFirstToken(cfg.TokenFile)
		if readErr != nil {
			return config{}, fmt.Errorf("read --token-file: %w", readErr)
		}
	}
	if cfg.Token == "" {
		return config{}, errors.New("a Bearer token is required via --token, --token-file, or QS_MODELCATALOG_CONFLICT_TOKEN[_FILE]")
	}
	if cfg.ModelCode == "" || cfg.QuestionnaireCode == "" {
		return config{}, errors.New("--model-code and --questionnaire-code are required")
	}
	if cfg.RequiredCodePrefix == "" {
		return config{}, errors.New("--required-code-prefix must not be empty")
	}
	if !strings.HasPrefix(cfg.ModelCode, cfg.RequiredCodePrefix) || !strings.HasPrefix(cfg.QuestionnaireCode, cfg.RequiredCodePrefix) {
		return config{}, fmt.Errorf("--model-code and --questionnaire-code must use required dedicated prefix %q", cfg.RequiredCodePrefix)
	}
	if cfg.Concurrency < 2 || cfg.Concurrency > 128 {
		return config{}, errors.New("--concurrency must be between 2 and 128")
	}
	if cfg.Rounds < 1 || cfg.Rounds > 20 {
		return config{}, errors.New("--rounds must be between 1 and 20")
	}
	if cfg.Timeout <= 0 {
		return config{}, errors.New("--timeout must be positive")
	}
	return cfg, nil
}

func validateBaseURL(value string) error {
	if value == "" {
		return errors.New("--api-base-url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("--api-base-url must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("--api-base-url must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("--api-base-url must not contain credentials, query parameters, or a fragment")
	}
	return nil
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

func writeEvidence(path string, evidence runEvidence) error {
	if path == "" {
		return nil
	}
	contents, err := json.MarshalIndent(evidence, "", "  ")
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

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
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
