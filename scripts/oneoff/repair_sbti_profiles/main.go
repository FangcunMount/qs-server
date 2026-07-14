// Command repair_sbti_profiles backfills canonical SBTI outcome patterns and
// special-result flags through the protected DefinitionV2 authoring API.
// It is dry-run by default and never publishes a repaired draft.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	rulesetinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
)

type config struct {
	APIBase   string
	ModelCode string
	Token     string
	BackupDir string
	Apply     bool
	Timeout   time.Duration
}

type apiClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type validationResult struct {
	Passed bool              `json:"passed"`
	Issues []validationIssue `json:"issues"`
}

type validationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Level   string `json:"level"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runCommand(ctx, os.Args[1:], os.Stdout, os.Getenv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "repair SBTI profiles failed: %v\n", err)
		os.Exit(1)
	}
}

func runCommand(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	cfg, err := parseConfig(args, out, getenv)
	if err != nil {
		return err
	}
	return run(ctx, cfg, out)
}

func parseConfig(args []string, out io.Writer, getenv func(string) string) (config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	cfg := config{
		APIBase:   firstNonEmpty(getenv("QS_APISERVER_URL"), getenv("QS_API_BASE")),
		Token:     firstNonEmpty(getenv("QS_OPERATOR_TOKEN"), getenv("QS_TOKEN")),
		BackupDir: "repair_backups",
		Timeout:   30 * time.Second,
	}
	flags := flag.NewFlagSet("repair_sbti_profiles", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.StringVar(&cfg.APIBase, "api-base", cfg.APIBase, "apiserver origin or /api/v1 base (default QS_APISERVER_URL)")
	flags.StringVar(&cfg.ModelCode, "model-code", "", "target assessment model code (required)")
	flags.StringVar(&cfg.BackupDir, "backup-dir", cfg.BackupDir, "directory for the pre-write DefinitionV2 backup")
	flags.BoolVar(&cfg.Apply, "apply", false, "save the repaired DefinitionV2 draft (default dry-run)")
	flags.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "per-request timeout")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage:")
		fmt.Fprintln(out, "  QS_APISERVER_URL=https://qs.example.com QS_OPERATOR_TOKEN=... \\")
		fmt.Fprintln(out, "    go run ./scripts/oneoff/repair_sbti_profiles/ --model-code SBTI_FUN [--apply]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "The operator token is read only from QS_OPERATOR_TOKEN or QS_TOKEN and is never accepted as a flag.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	cfg.APIBase = normalizeAPIBase(cfg.APIBase)
	cfg.ModelCode = strings.TrimSpace(cfg.ModelCode)
	if cfg.APIBase == "" {
		return config{}, fmt.Errorf("--api-base or QS_APISERVER_URL is required")
	}
	if cfg.ModelCode == "" {
		return config{}, fmt.Errorf("--model-code is required")
	}
	if cfg.Token == "" {
		return config{}, fmt.Errorf("QS_OPERATOR_TOKEN or QS_TOKEN is required")
	}
	if cfg.Timeout <= 0 {
		return config{}, fmt.Errorf("--timeout must be positive")
	}
	if cfg.Apply && strings.TrimSpace(cfg.BackupDir) == "" {
		return config{}, fmt.Errorf("--backup-dir is required with --apply")
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	seed, err := rulesetinfra.LoadDefaultSBTILegacyModel()
	if err != nil {
		return err
	}
	catalog, err := catalogFromSBTI(seed)
	if err != nil {
		return fmt.Errorf("build canonical SBTI repair catalog: %w", err)
	}
	client := apiClient{
		baseURL: cfg.APIBase,
		token:   cfg.Token,
		client:  &http.Client{Timeout: cfg.Timeout},
	}
	definitionPath := "/assessment-models/" + url.PathEscape(cfg.ModelCode) + "/definition"
	original, err := client.request(ctx, http.MethodGet, definitionPath, nil)
	if err != nil {
		return fmt.Errorf("load DefinitionV2 for %s: %w", cfg.ModelCode, err)
	}
	repaired, summary, err := repairDefinition(original, catalog)
	if err != nil {
		return fmt.Errorf("plan repair for %s: %w", cfg.ModelCode, err)
	}
	printSummary(out, cfg.ModelCode, summary)
	if !summary.Changed() {
		fmt.Fprintln(out, "DefinitionV2 already matches the canonical SBTI profile data; nothing to write.")
		return nil
	}
	if !cfg.Apply {
		fmt.Fprintln(out, "Dry run complete. No draft or published snapshot was changed. Pass --apply to save the repaired draft.")
		return nil
	}

	backupPath, err := writeBackup(cfg.BackupDir, cfg.ModelCode, original, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("write pre-repair backup: %w", err)
	}
	fmt.Fprintf(out, "Backup written: %s\n", backupPath)
	written, err := client.request(ctx, http.MethodPut, definitionPath, repaired)
	if err != nil {
		return fmt.Errorf("save repaired DefinitionV2 for %s: %w", cfg.ModelCode, err)
	}
	if err := verifyRepairedDefinition(written, catalog); err != nil {
		return fmt.Errorf("server returned an unexpected DefinitionV2 after save: %w", err)
	}

	validationData, err := client.request(ctx, http.MethodPost, "/assessment-models/"+url.PathEscape(cfg.ModelCode)+"/validate", nil)
	if err != nil {
		return fmt.Errorf("DefinitionV2 was saved as a draft, but server validation could not run: %w", err)
	}
	var validation validationResult
	if err := json.Unmarshal(validationData, &validation); err != nil {
		return fmt.Errorf("DefinitionV2 was saved as a draft, but validation response could not be decoded: %w", err)
	}
	for _, issue := range validation.Issues {
		fmt.Fprintf(out, "validation %s %s: %s\n", firstNonEmpty(issue.Level, "error"), issue.Field, issue.Message)
	}
	if !validation.Passed {
		return fmt.Errorf("DefinitionV2 was saved as a draft, but server validation did not pass; inspect the issues above and do not publish")
	}
	fmt.Fprintln(out, "Repair saved and server validation passed. The draft was not published.")
	return nil
}

func (c apiClient) request(ctx context.Context, method, requestPath string, body []byte) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, fmt.Errorf("%s %s returned HTTP %d with invalid JSON: %w", method, requestPath, res.StatusCode, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || envelope.Code != 0 {
		return nil, fmt.Errorf("%s %s returned HTTP %d, code=%d, message=%s", method, requestPath, res.StatusCode, envelope.Code, envelope.Message)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil, fmt.Errorf("%s %s returned no data", method, requestPath)
	}
	return envelope.Data, nil
}

func printSummary(out io.Writer, modelCode string, summary repairSummary) {
	fmt.Fprintf(out, "Model: %s\n", modelCode)
	fmt.Fprintf(out, "Profiles: total=%d normal=%d special=%d\n", summary.ProfileCount, summary.NormalCount, summary.SpecialCount)
	fmt.Fprintf(out, "Planned changes: patterns=%d special_flags=%d total=%d\n", summary.PatternChanges, summary.SpecialFlagChanges, len(summary.Changes))
	for _, change := range summary.Changes {
		fmt.Fprintf(out, "  %s.%s: %s -> %s\n", change.OutcomeCode, change.Field, change.Before, change.After)
	}
}

func writeBackup(directory, modelCode string, definition []byte, now time.Time) (string, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%s-definition-before-sbti-profile-repair-%s.json", safeFilename(modelCode), now.Format("20060102T150405Z"))
	path := filepath.Join(directory, filename)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, definition, "", "  "); err != nil {
		return "", err
	}
	pretty.WriteByte('\n')
	if _, err := file.Write(pretty.Bytes()); err != nil {
		return "", err
	}
	return path, nil
}

func normalizeAPIBase(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, "/api/v1") {
		return value
	}
	return value + "/api/v1"
}

func safeFilename(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "model"
	}
	return builder.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
