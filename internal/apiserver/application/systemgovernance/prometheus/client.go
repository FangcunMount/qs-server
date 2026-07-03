package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client queries Prometheus HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a Prometheus query client.
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error     string `json:"error"`
	ErrorType string `json:"errorType"`
}

// QueryInstant executes an instant PromQL query at evaluation time.
func (c *Client) QueryInstant(ctx context.Context, query string, evalAt time.Time) (float64, bool, error) {
	if c == nil || c.baseURL == "" {
		return 0, false, fmt.Errorf("prometheus client unavailable")
	}
	endpoint := c.baseURL + "/api/v1/query"
	values := url.Values{}
	values.Set("query", query)
	if !evalAt.IsZero() {
		values.Set("time", strconv.FormatInt(evalAt.Unix(), 10))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
	if err != nil {
		return 0, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, false, fmt.Errorf("prometheus query failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var payload queryResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, false, err
	}
	if payload.Status != "success" {
		if payload.Error != "" {
			return 0, false, fmt.Errorf("prometheus error: %s", payload.Error)
		}
		return 0, false, fmt.Errorf("prometheus query unsuccessful")
	}
	if len(payload.Data.Result) == 0 {
		return 0, false, nil
	}
	value, ok, err := parseSampleValue(payload.Data.Result[0].Value)
	if err != nil {
		return 0, false, err
	}
	return value, ok, nil
}

func parseSampleValue(sample []interface{}) (float64, bool, error) {
	if len(sample) < 2 {
		return 0, false, nil
	}
	raw, ok := sample[1].(string)
	if !ok {
		return 0, false, fmt.Errorf("unexpected prometheus sample value type %T", sample[1])
	}
	if raw == "NaN" {
		return 0, false, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false, err
	}
	return value, true, nil
}
