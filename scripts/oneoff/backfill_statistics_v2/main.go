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
	"strconv"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

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

type options struct {
	BaseURL      string
	Token        string
	OrgIDs       []int64
	From         time.Time
	To           time.Time
	WindowDays   int
	Reason       string
	Mode         string
	Confirm      bool
	ValidateOnly bool
}

type runResult struct {
	ID           uint64           `json:"id"`
	Mode         string           `json:"mode"`
	Status       string           `json:"status"`
	Stage        string           `json:"stage"`
	AsOfDate     string           `json:"as_of_date"`
	SourceCounts map[string]int64 `json:"source_counts"`
	FactCounts   map[string]int64 `json:"fact_counts"`
	ResultCounts map[string]int64 `json:"result_counts"`
	ErrorCode    string           `json:"error_code"`
	ErrorMessage string           `json:"error_message"`
}

type runResponse struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Data    runResult `json:"data"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "statistics v2 backfill:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("backfill_statistics_v2", flag.ContinueOnError)
	flags.SetOutput(output)
	var rawOrgIDs, from, to string
	var cfg options
	flags.StringVar(&cfg.BaseURL, "base-url", "", "apiserver base URL")
	flags.StringVar(&cfg.Token, "token", os.Getenv("QS_STATISTICS_V2_TOKEN"), "bearer token (or QS_STATISTICS_V2_TOKEN)")
	flags.StringVar(&rawOrgIDs, "org-ids", "", "comma-separated organization IDs")
	flags.StringVar(&from, "from", "", "first Shanghai business date, inclusive")
	flags.StringVar(&to, "to", "", "last Shanghai business date, inclusive")
	flags.IntVar(&cfg.WindowDays, "window-days", 7, "dates per run, maximum 31")
	flags.StringVar(&cfg.Reason, "reason", "statistics_v2_backfill", "audited run reason")
	flags.StringVar(&cfg.Mode, "mode", "repair", "run mode: validate, repair, or publish")
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
	if err := cfg.validate(); err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Minute}
	if cfg.ValidateOnly {
		cfg.Mode = "validate"
	}
	for _, orgID := range cfg.OrgIDs {
		for _, window := range splitWindows(cfg.From, cfg.To, cfg.WindowDays) {
			fmt.Fprintf(output, "org=%d window=%s..%s mode=%s\n", orgID, window.From.Format(dateLayout), window.To.Format(dateLayout), cfg.Mode)
			result, err := executeRun(client, cfg, orgID, window)
			if err != nil {
				return fmt.Errorf("org %d window %s..%s: %w", orgID, window.From.Format(dateLayout), window.To.Format(dateLayout), err)
			}
			encoded, _ := json.Marshal(result)
			fmt.Fprintln(output, string(encoded))
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
	mode := strings.TrimSpace(o.Mode)
	if o.ValidateOnly {
		mode = "validate"
	}
	if mode != "validate" && mode != "repair" && mode != "publish" {
		return errors.New("mode must be validate, repair, or publish")
	}
	if mode != "validate" && !o.Confirm {
		return errors.New("write mode requires --confirm")
	}
	if strings.TrimSpace(o.Reason) == "" {
		return errors.New("reason is required")
	}
	if len([]rune(o.Reason)) > 500 {
		return errors.New("reason exceeds 500 characters")
	}
	return nil
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
	defer resp.Body.Close()
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
