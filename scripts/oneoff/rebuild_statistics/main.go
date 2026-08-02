package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	dateLayout            = "2006-01-02"
	defaultRequestTimeout = 10 * time.Minute
)

type dateWindow struct {
	From time.Time
	To   time.Time
}

type runRequest struct {
	Mode         string `json:"mode"`
	FromDate     string `json:"from_date"`
	ToDate       string `json:"to_date"`
	Reason       string `json:"reason"`
	Confirm      bool   `json:"confirm"`
	ValidateOnly bool   `json:"validate_only"`
}

type resumeCacheRequest struct {
	Reason  string `json:"reason"`
	Confirm bool   `json:"confirm"`
}

type options struct {
	BaseURL        string
	Token          string
	OrgIDs         []int64
	From           time.Time
	To             time.Time
	ResumeFrom     time.Time
	WindowDays     int
	RequestTimeout time.Duration
	Reason         string
	Mode           string
	Confirm        bool
	ValidateOnly   bool
	Now            func() time.Time
}

const historicalBackfillMode = "historical-backfill"

type runResult struct {
	ID               uint64           `json:"id"`
	Mode             string           `json:"mode"`
	Status           string           `json:"status"`
	Stage            string           `json:"stage"`
	AsOfDate         string           `json:"as_of_date"`
	CacheGeneration  int64            `json:"cache_generation"`
	CachePublishedAt string           `json:"cache_published_at"`
	SourceCounts     map[string]int64 `json:"source_counts"`
	FactCounts       map[string]int64 `json:"fact_counts"`
	ResultCounts     map[string]int64 `json:"result_counts"`
	ErrorCode        string           `json:"error_code"`
	ErrorMessage     string           `json:"error_message"`
}

type runResponse struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Data    runResult `json:"data"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "statistics rebuild:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("rebuild_statistics", flag.ContinueOnError)
	flags.SetOutput(output)
	var rawOrgIDs, from, to, resumeFrom string
	var cfg options
	flags.StringVar(&cfg.BaseURL, "base-url", "", "apiserver base URL")
	flags.StringVar(&cfg.Token, "token", os.Getenv("QS_STATISTICS_TOKEN"), "bearer token (or QS_STATISTICS_TOKEN)")
	flags.StringVar(&rawOrgIDs, "org-ids", "", "comma-separated organization IDs")
	flags.StringVar(&from, "from", "", "first Shanghai business date, inclusive")
	flags.StringVar(&to, "to", "", "last Shanghai business date, inclusive")
	flags.StringVar(&resumeFrom, "resume-from", "", "historical-backfill date to resume from, inclusive")
	flags.IntVar(&cfg.WindowDays, "window-days", 7, "dates per run, maximum 31")
	flags.DurationVar(&cfg.RequestTimeout, "timeout", defaultRequestTimeout, "HTTP timeout for each Statistics run")
	flags.StringVar(&cfg.Reason, "reason", "statistics_rebuild", "audited run reason")
	flags.StringVar(&cfg.Mode, "mode", "repair", "run mode: validate, repair, publish, or historical-backfill")
	flags.BoolVar(&cfg.Confirm, "confirm", false, "confirm writes")
	flags.BoolVar(&cfg.ValidateOnly, "validate-only", false, "read, map and validate without writing")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var err error
	cfg.OrgIDs, err = parseOrgIDs(rawOrgIDs)
	if err != nil {
		return err
	}
	cfg.From, err = parseShanghaiDate(from)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	cfg.To, err = parseShanghaiDate(to)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if strings.TrimSpace(resumeFrom) != "" {
		cfg.ResumeFrom, err = parseShanghaiDate(resumeFrom)
		if err != nil {
			return fmt.Errorf("resume-from: %w", err)
		}
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	client := &http.Client{Timeout: cfg.RequestTimeout}
	if cfg.ValidateOnly {
		cfg.Mode = "validate"
	}
	for _, orgID := range cfg.OrgIDs {
		if cfg.Mode == historicalBackfillMode {
			if err := executeHistoricalBackfill(client, cfg, orgID, output); err != nil {
				return fmt.Errorf("org %d historical backfill statistics: %w", orgID, err)
			}
			continue
		}
		for _, window := range splitWindows(cfg.From, cfg.To, cfg.WindowDays) {
			_, _ = fmt.Fprintf(output, "org=%d window=%s..%s mode=%s\n", orgID, window.From.Format(dateLayout), window.To.Format(dateLayout), cfg.Mode)
			result, err := executeRunWithCacheRecovery(client, cfg, orgID, window, output)
			if err != nil {
				return fmt.Errorf("org %d window %s..%s: %w", orgID, window.From.Format(dateLayout), window.To.Format(dateLayout), err)
			}
			if cfg.Mode == "validate" {
				if err := validateRunCompleteness(result); err != nil {
					return fmt.Errorf("org %d window %s..%s: %w", orgID, window.From.Format(dateLayout), window.To.Format(dateLayout), err)
				}
			}
			encoded, _ := json.Marshal(result)
			_, _ = fmt.Fprintln(output, string(encoded))
		}
	}
	return nil
}

func (o options) validate() error {
	if strings.TrimSpace(o.BaseURL) == "" {
		return errors.New("base-url is required")
	}
	if strings.TrimSpace(o.Token) == "" {
		return errors.New("token is required")
	}
	if len(o.OrgIDs) == 0 {
		return errors.New("at least one org-id is required")
	}
	if o.From.IsZero() || o.To.IsZero() || o.To.Before(o.From) {
		return errors.New("invalid inclusive date range")
	}
	if o.WindowDays < 1 || o.WindowDays > 31 {
		return errors.New("window-days must be between 1 and 31")
	}
	if o.RequestTimeout <= 0 {
		return errors.New("timeout must be positive")
	}
	mode := strings.TrimSpace(o.Mode)
	if o.ValidateOnly {
		mode = "validate"
	}
	if mode != "validate" && mode != "repair" && mode != "publish" && mode != historicalBackfillMode {
		return errors.New("mode must be validate, repair, publish, or historical-backfill")
	}
	if mode != "validate" && !o.Confirm {
		return errors.New("write mode requires --confirm")
	}
	if !o.ResumeFrom.IsZero() {
		if mode != historicalBackfillMode {
			return errors.New("resume-from is only supported in historical-backfill mode")
		}
		if o.ResumeFrom.Before(o.From) {
			return errors.New("resume-from cannot be before from")
		}
	}
	if strings.TrimSpace(o.Reason) == "" {
		return errors.New("reason is required")
	}
	if len([]rune(o.Reason)) > 500 {
		return errors.New("reason exceeds 500 characters")
	}
	return nil
}

func executeHistoricalBackfill(client *http.Client, cfg options, orgID int64, output io.Writer) error {
	latestCompleteDay, err := latestCompleteShanghaiDay(cfg.Now)
	if err != nil {
		return err
	}
	if latestCompleteDay.Before(cfg.To) {
		return fmt.Errorf("latest complete Shanghai business day %s is before historical backfill end %s", latestCompleteDay.Format(dateLayout), cfg.To.Format(dateLayout))
	}
	resumeFrom := cfg.From
	if !cfg.ResumeFrom.IsZero() {
		resumeFrom = cfg.ResumeFrom
		if resumeFrom.After(latestCompleteDay) {
			return fmt.Errorf("resume-from %s exceeds latest complete Shanghai business day %s", resumeFrom.Format(dateLayout), latestCompleteDay.Format(dateLayout))
		}
		_, _ = fmt.Fprintf(output, "org=%d phase=resume resume_from=%s\n", orgID, resumeFrom.Format(dateLayout))
	}
	if !resumeFrom.After(cfg.To) {
		if err := executeRepairAndValidateWindows(client, cfg, orgID, output, "historical", splitWindows(resumeFrom, cfg.To, cfg.WindowDays)); err != nil {
			return err
		}
	}
	if latestCompleteDay.After(cfg.To) {
		catchupFrom := cfg.To.AddDate(0, 0, 1)
		if resumeFrom.After(catchupFrom) {
			catchupFrom = resumeFrom
		}
		if err := executeRepairAndValidateWindows(client, cfg, orgID, output, "catchup", splitWindows(catchupFrom, latestCompleteDay, cfg.WindowDays)); err != nil {
			return err
		}
	}

	publish := cfg
	publish.Mode = "publish"
	publish.ValidateOnly = false
	finalWindow := dateWindow{From: latestCompleteDay, To: latestCompleteDay}
	_, _ = fmt.Fprintf(output, "org=%d phase=publish as_of_date=%s\n", orgID, latestCompleteDay.Format(dateLayout))
	result, err := executeRunWithCacheRecovery(client, publish, orgID, finalWindow, output)
	if err != nil {
		return fmt.Errorf("publish as_of_date %s: %w", latestCompleteDay.Format(dateLayout), err)
	}
	if strings.TrimSpace(result.AsOfDate) == "" || result.AsOfDate != latestCompleteDay.Format(dateLayout) {
		return fmt.Errorf("publish watermark %q does not match latest complete Shanghai business day %s", result.AsOfDate, latestCompleteDay.Format(dateLayout))
	}
	encoded, _ := json.Marshal(result)
	_, _ = fmt.Fprintln(output, string(encoded))
	return nil
}

func executeRepairAndValidateWindows(client *http.Client, cfg options, orgID int64, output io.Writer, phase string, windows []dateWindow) error {
	for index, window := range windows {
		repair := cfg
		repair.Mode = "repair"
		repair.ValidateOnly = false
		_, _ = fmt.Fprintf(output, "org=%d phase=%s_repair window=%s..%s index=%d\n", orgID, phase, window.From.Format(dateLayout), window.To.Format(dateLayout), index+1)
		result, err := executeRunWithCacheRecovery(client, repair, orgID, window, output)
		if err != nil {
			return fmt.Errorf("repair window %s..%s: %w; resume with --resume-from %s", window.From.Format(dateLayout), window.To.Format(dateLayout), err, window.From.Format(dateLayout))
		}
		encoded, _ := json.Marshal(result)
		_, _ = fmt.Fprintln(output, string(encoded))

		validate := cfg
		validate.Mode = "validate"
		validate.Confirm = false
		validate.ValidateOnly = true
		_, _ = fmt.Fprintf(output, "org=%d phase=%s_validate window=%s..%s index=%d\n", orgID, phase, window.From.Format(dateLayout), window.To.Format(dateLayout), index+1)
		result, err = executeRunWithCacheRecovery(client, validate, orgID, window, output)
		if err != nil {
			return fmt.Errorf("validate window %s..%s: %w; resume with --resume-from %s", window.From.Format(dateLayout), window.To.Format(dateLayout), err, window.From.Format(dateLayout))
		}
		if err := validateRunCompleteness(result); err != nil {
			return fmt.Errorf("validate window %s..%s: %w; resume with --resume-from %s", window.From.Format(dateLayout), window.To.Format(dateLayout), err, window.From.Format(dateLayout))
		}
		encoded, _ = json.Marshal(result)
		_, _ = fmt.Fprintln(output, string(encoded))
	}
	return nil
}

func validateRunCompleteness(result runResult) error {
	var issues []string
	for _, collector := range []string{"access", "plan", "assessment"} {
		for _, metric := range []string{"inserted", "conflict"} {
			key := collector + "." + metric
			value, exists := result.FactCounts[key]
			if !exists {
				issues = append(issues, key+" is missing")
				continue
			}
			if value != 0 {
				issues = append(issues, fmt.Sprintf("%s=%d", key, value))
			}
		}
	}
	if len(issues) == 0 {
		return nil
	}
	sort.Strings(issues)
	return fmt.Errorf("validation is incomplete: %s", strings.Join(issues, ", "))
}

func latestCompleteShanghaiDay(nowFn func() time.Time) (time.Time, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, fmt.Errorf("load Asia/Shanghai: %w", err)
	}
	now := time.Now()
	if nowFn != nil {
		now = nowFn()
	}
	local := now.In(location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return today.AddDate(0, 0, -1), nil
}

func executeRun(client *http.Client, cfg options, orgID int64, window dateWindow) (runResult, error) {
	body, err := json.Marshal(runRequest{
		Mode:     cfg.Mode,
		FromDate: window.From.Format(dateLayout), ToDate: window.To.Format(dateLayout),
		Reason: cfg.Reason, Confirm: cfg.Confirm, ValidateOnly: cfg.ValidateOnly,
	})
	if err != nil {
		return runResult{}, err
	}
	url := strings.TrimRight(cfg.BaseURL, "/") + "/internal/v2/statistics/runs"
	return executeRunRequest(client, cfg, orgID, url, body)
}

func executeRunWithCacheRecovery(client *http.Client, cfg options, orgID int64, window dateWindow, output io.Writer) (runResult, error) {
	result, err := executeRun(client, cfg, orgID, window)
	if err == nil {
		return result, nil
	}
	if result.ID == 0 || result.Status != "data_committed" {
		return result, err
	}
	_, _ = fmt.Fprintf(output, "org=%d phase=resume_cache run_id=%d\n", orgID, result.ID)
	resumed, resumeErr := executeResumeCache(client, cfg, orgID, result.ID)
	if resumeErr != nil {
		return resumed, fmt.Errorf("run %d is data_committed and cache resume failed: %w", result.ID, resumeErr)
	}
	return resumed, nil
}

func executeResumeCache(client *http.Client, cfg options, orgID int64, runID uint64) (runResult, error) {
	body, err := json.Marshal(resumeCacheRequest{Reason: cfg.Reason, Confirm: true})
	if err != nil {
		return runResult{}, err
	}
	url := fmt.Sprintf("%s/internal/v2/statistics/runs/%d/resume-cache", strings.TrimRight(cfg.BaseURL, "/"), runID)
	return executeRunRequest(client, cfg, orgID, url, body)
}

func executeRunRequest(client *http.Client, cfg options, orgID int64, url string, body []byte) (runResult, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return runResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	// Organization is supplied through the protected request scope, never in
	// the JSON body. This header is the existing internal caller scope carrier.
	req.Header.Set("X-Org-ID", strconv.FormatInt(orgID, 10))
	resp, err := client.Do(req)
	if err != nil {
		return runResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return runResult{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return runResult{}, fmt.Errorf("server returned %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	var envelope runResponse
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return runResult{}, fmt.Errorf("decode run response: %w", err)
	}
	if envelope.Code != 0 {
		return runResult{}, fmt.Errorf("server returned business code %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data.ID == 0 {
		return runResult{}, errors.New("server response does not contain a run id")
	}
	if envelope.Data.Status != "succeeded" {
		if envelope.Data.Status == "data_committed" {
			return envelope.Data, fmt.Errorf("run %d is data_committed; resume cache before continuing", envelope.Data.ID)
		}
		return envelope.Data, fmt.Errorf("run %d ended status=%s stage=%s code=%s: %s", envelope.Data.ID, envelope.Data.Status, envelope.Data.Stage, envelope.Data.ErrorCode, envelope.Data.ErrorMessage)
	}
	return envelope.Data, nil
}

func splitWindows(from, to time.Time, days int) []dateWindow {
	var windows []dateWindow
	for start := from; !start.After(to); {
		end := start.AddDate(0, 0, days-1)
		if end.After(to) {
			end = to
		}
		windows = append(windows, dateWindow{From: start, To: end})
		start = end.AddDate(0, 0, 1)
	}
	return windows
}

func parseShanghaiDate(raw string) (time.Time, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation(dateLayout, raw, location)
}

func parseOrgIDs(raw string) ([]int64, error) {
	seen := map[int64]struct{}{}
	var result []int64
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id, err := strconv.ParseInt(item, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid org-id %q", item)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}
