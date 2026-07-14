// Command migrate_personality_runtime_adapters normalizes the four known
// historical typology DefinitionV2 adapter keys through the protected
// authoring and release APIs. It is dry-run by default.
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
)

type migrationTarget struct {
	Code            string
	ExpectedVersion string
	LegacyAdapter   string
	GenericAdapter  string
}

var defaultTargets = []migrationTarget{
	{Code: "MBTI_OEJTS", ExpectedVersion: "v25", LegacyAdapter: "mbti", GenericAdapter: "personality_type"},
	{Code: "MBTI_FC_93", ExpectedVersion: "v15", LegacyAdapter: "mbti", GenericAdapter: "personality_type"},
	{Code: "SBTI_FUN", ExpectedVersion: "v29", LegacyAdapter: "sbti", GenericAdapter: "personality_type"},
	{Code: "BIG5_IPIP_50", ExpectedVersion: "v9", LegacyAdapter: "bigfive", GenericAdapter: "trait_profile"},
}

type config struct {
	APIBase   string
	Token     string
	BackupDir string
	Apply     bool
	Publish   bool
	Timeout   time.Duration
	Targets   []migrationTarget
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

type modelWire struct {
	Code   string `json:"code"`
	Status string `json:"status"`
}

type publishedModelWire struct {
	Code       string          `json:"code"`
	Status     string          `json:"status"`
	Version    string          `json:"version"`
	Definition json.RawMessage `json:"definition"`
}

type validationResult struct {
	Passed bool              `json:"passed"`
	Issues []validationIssue `json:"issues"`
}

type validationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runCommand(ctx, os.Args[1:], os.Stdout, os.Getenv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "migrate personality runtime adapters failed: %v\n", err)
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
		BackupDir: "migration_backups",
		Timeout:   30 * time.Second,
		Targets:   append([]migrationTarget(nil), defaultTargets...),
	}
	flags := flag.NewFlagSet("migrate_personality_runtime_adapters", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.StringVar(&cfg.APIBase, "api-base", cfg.APIBase, "apiserver origin or /api/v1 base (default QS_APISERVER_URL)")
	flags.StringVar(&cfg.BackupDir, "backup-dir", cfg.BackupDir, "directory for pre-write DefinitionV2 backups")
	flags.BoolVar(&cfg.Apply, "apply", false, "save normalized DefinitionV2 drafts (default dry-run)")
	flags.BoolVar(&cfg.Publish, "publish", false, "publish every validated normalized draft; requires --apply")
	flags.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "per-request timeout")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(out, "Usage:")
		_, _ = fmt.Fprintln(out, "  QS_APISERVER_URL=https://qs.example.com QS_OPERATOR_TOKEN=... \\")
		_, _ = fmt.Fprintln(out, "    go run ./scripts/oneoff/migrate_personality_runtime_adapters/ [--apply] [--publish]")
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "The migration is hard-coded to MBTI_OEJTS@v25, MBTI_FC_93@v15, SBTI_FUN@v29, and BIG5_IPIP_50@v9.")
		_, _ = fmt.Fprintln(out, "The operator token is read only from QS_OPERATOR_TOKEN or QS_TOKEN and is never accepted as a flag.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	cfg.APIBase = normalizeAPIBase(cfg.APIBase)
	if cfg.APIBase == "" {
		return config{}, fmt.Errorf("--api-base or QS_APISERVER_URL is required")
	}
	if cfg.Token == "" {
		return config{}, fmt.Errorf("QS_OPERATOR_TOKEN or QS_TOKEN is required")
	}
	if cfg.Timeout <= 0 {
		return config{}, fmt.Errorf("--timeout must be positive")
	}
	if cfg.Publish && !cfg.Apply {
		return config{}, fmt.Errorf("--publish requires --apply")
	}
	if cfg.Apply && strings.TrimSpace(cfg.BackupDir) == "" {
		return config{}, fmt.Errorf("--backup-dir is required with --apply")
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	targets := cfg.Targets
	if len(targets) == 0 {
		targets = defaultTargets
	}
	client := apiClient{baseURL: cfg.APIBase, token: cfg.Token, client: &http.Client{Timeout: cfg.Timeout}}
	for _, target := range targets {
		if err := migrateOne(ctx, client, cfg, target, out); err != nil {
			return err
		}
	}
	if cfg.Publish {
		_, _ = fmt.Fprintln(out, "Migration and release completed for every changed target.")
	} else if cfg.Apply {
		_, _ = fmt.Fprintln(out, "Normalized drafts were saved and validated. Re-run with --apply --publish to release them.")
	} else {
		_, _ = fmt.Fprintln(out, "Dry run complete. No draft or published snapshot was changed.")
	}
	return nil
}

func migrateOne(ctx context.Context, client apiClient, cfg config, target migrationTarget, out io.Writer) error {
	modelData, err := client.request(ctx, http.MethodGet, "/assessment-models/"+url.PathEscape(target.Code), nil)
	if err != nil {
		return fmt.Errorf("load current model %s: %w", target.Code, err)
	}
	var model modelWire
	if err := json.Unmarshal(modelData, &model); err != nil {
		return fmt.Errorf("decode current model %s: %w", target.Code, err)
	}
	if model.Code != target.Code || model.Status != "published" {
		return fmt.Errorf("refusing %s: current model must be published without an existing draft (code=%q status=%q)", target.Code, model.Code, model.Status)
	}

	publishedData, err := client.request(ctx, http.MethodGet, "/assessment-models/published/"+url.PathEscape(target.Code), nil)
	if err != nil {
		return fmt.Errorf("load published DefinitionV2 for %s: %w", target.Code, err)
	}
	var published publishedModelWire
	if err := json.Unmarshal(publishedData, &published); err != nil {
		return fmt.Errorf("decode published model %s: %w", target.Code, err)
	}
	if published.Code != target.Code || published.Status != "published" {
		return fmt.Errorf("refusing %s: unexpected published response (code=%q status=%q)", target.Code, published.Code, published.Status)
	}
	if len(published.Definition) == 0 || string(published.Definition) == "null" {
		return fmt.Errorf("refusing %s: published DefinitionV2 is missing", target.Code)
	}

	normalized, summary, err := normalizeDefinition(published.Definition, target)
	if err != nil {
		return fmt.Errorf("plan adapter normalization for %s: %w", target.Code, err)
	}
	_, _ = fmt.Fprintf(out, "%s@%s: outcome=%s report=%s\n", target.Code, published.Version, summary.OutcomeAdapter, summary.ReportAdapter)
	if !summary.Changed() {
		_, _ = fmt.Fprintln(out, "  already normalized; skipped")
		return nil
	}
	if published.Version != target.ExpectedVersion {
		return fmt.Errorf("refusing %s: published version is %q, want pinned %q before normalizing legacy adapters", target.Code, published.Version, target.ExpectedVersion)
	}
	_, _ = fmt.Fprintf(out, "  planned: OutcomeMapping.DetailAdapterKey and ReportMap section AdapterKey -> %s\n", target.GenericAdapter)
	if !cfg.Apply {
		return nil
	}

	backupPath, err := writeBackup(cfg.BackupDir, target, published.Definition, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("write pre-migration backup for %s: %w", target.Code, err)
	}
	_, _ = fmt.Fprintf(out, "  backup: %s\n", backupPath)
	definitionPath := "/assessment-models/" + url.PathEscape(target.Code) + "/definition"
	written, err := client.request(ctx, http.MethodPut, definitionPath, normalized)
	if err != nil {
		return fmt.Errorf("save normalized DefinitionV2 for %s: %w", target.Code, err)
	}
	if err := verifyNormalizedDefinition(written, target); err != nil {
		return fmt.Errorf("server returned an unexpected normalized DefinitionV2 for %s: %w", target.Code, err)
	}

	validationData, err := client.request(ctx, http.MethodPost, definitionPath[:len(definitionPath)-len("/definition")]+"/validate", nil)
	if err != nil {
		return fmt.Errorf("DefinitionV2 for %s was saved as draft but validation could not run: %w", target.Code, err)
	}
	var validation validationResult
	if err := json.Unmarshal(validationData, &validation); err != nil {
		return fmt.Errorf("decode validation for %s: %w", target.Code, err)
	}
	for _, issue := range validation.Issues {
		_, _ = fmt.Fprintf(out, "  validation %s %s: %s\n", firstNonEmpty(issue.Level, "error"), issue.Field, issue.Message)
	}
	if !validation.Passed {
		return fmt.Errorf("DefinitionV2 for %s was saved as draft but validation did not pass; refusing to publish", target.Code)
	}
	if !cfg.Publish {
		_, _ = fmt.Fprintln(out, "  draft saved and validation passed")
		return nil
	}
	if _, err := client.request(ctx, http.MethodPost, "/assessment-releases/"+url.PathEscape(target.Code)+"/publish", nil); err != nil {
		return fmt.Errorf("publish normalized %s: %w", target.Code, err)
	}
	if err := verifyPublishedRelease(ctx, client, target, published.Version); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "  published and verified")
	return nil
}

func verifyPublishedRelease(ctx context.Context, client apiClient, target migrationTarget, previousVersion string) error {
	data, err := client.request(ctx, http.MethodGet, "/assessment-models/published/"+url.PathEscape(target.Code), nil)
	if err != nil {
		return fmt.Errorf("verify published %s: %w", target.Code, err)
	}
	var published publishedModelWire
	if err := json.Unmarshal(data, &published); err != nil {
		return fmt.Errorf("decode verified published %s: %w", target.Code, err)
	}
	if published.Code != target.Code || published.Status != "published" {
		return fmt.Errorf("verify published %s: unexpected response code=%q status=%q", target.Code, published.Code, published.Status)
	}
	if published.Version == "" || published.Version == previousVersion {
		return fmt.Errorf("verify published %s: version is %q, want a new published snapshot after %q", target.Code, published.Version, previousVersion)
	}
	if err := verifyNormalizedDefinition(published.Definition, target); err != nil {
		return fmt.Errorf("verify published %s DefinitionV2: %w", target.Code, err)
	}
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
	defer func() { _ = res.Body.Close() }()
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

func writeBackup(directory string, target migrationTarget, definition []byte, now time.Time) (string, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%s-%s-definition-before-runtime-adapter-migration-%s.json", target.Code, target.ExpectedVersion, now.Format("20060102T150405Z"))
	path := filepath.Join(directory, filename)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
