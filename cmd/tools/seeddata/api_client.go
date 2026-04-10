package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
)

// APIClient HTTP API 客户端
type APIClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     log.Logger
	tokenMu    sync.RWMutex
	refresher  func(context.Context) (string, error)
	provider   *seedTokenProvider

	retryMax      int
	retryMinDelay time.Duration
	retryMaxDelay time.Duration

	scaleCacheMu         sync.RWMutex
	scaleCache           map[string]*ScaleResponse
	questionnaireCacheMu sync.RWMutex
	questionnaireCache   map[string]*QuestionnaireDetailResponse
}

type seedTokenProvider struct {
	tokenMu   sync.RWMutex
	refreshMu sync.Mutex
	token     string
	expiresAt time.Time
	refresher func(context.Context) (string, error)
}

const (
	defaultHTTPTimeout         = 30 * time.Second
	planScheduleRequestTimeout = 5 * time.Minute
	seedTokenRefreshSkew       = 2 * time.Minute
)

// NewAPIClient 创建 API 客户端
func NewAPIClient(baseURL, token string, logger log.Logger) *APIClient {
	// 确保 baseURL 不以斜杠结尾
	baseURL = strings.TrimSuffix(baseURL, "/")

	retryMax, retryMinDelay, retryMaxDelay := defaultRetryConfig()

	return &APIClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		logger:             logger,
		retryMax:           retryMax,
		retryMinDelay:      retryMinDelay,
		retryMaxDelay:      retryMaxDelay,
		scaleCache:         make(map[string]*ScaleResponse),
		questionnaireCache: make(map[string]*QuestionnaireDetailResponse),
	}
}

// SetTokenRefresher sets a callback to refresh token when needed.
func (c *APIClient) SetTokenRefresher(fn func(context.Context) (string, error)) {
	c.refresher = fn
	if c.provider != nil {
		c.provider.SetRefresher(fn)
	}
}

func newSeedTokenProvider(initialToken string, refresher func(context.Context) (string, error)) *seedTokenProvider {
	provider := &seedTokenProvider{
		refresher: refresher,
	}
	provider.SetToken(initialToken)
	return provider
}

func (p *seedTokenProvider) SetRefresher(fn func(context.Context) (string, error)) {
	if p == nil {
		return
	}
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	p.refresher = fn
}

func (p *seedTokenProvider) Token() string {
	if p == nil {
		return ""
	}
	p.tokenMu.RLock()
	defer p.tokenMu.RUnlock()
	return p.token
}

func (p *seedTokenProvider) SetToken(token string) {
	if p == nil {
		return
	}
	token = strings.TrimSpace(token)
	identity := parseSeedTokenIdentity(token)
	p.tokenMu.Lock()
	p.token = token
	p.expiresAt = identity.ExpiresAt
	p.tokenMu.Unlock()
}

func (p *seedTokenProvider) ExpiresAt() time.Time {
	if p == nil {
		return time.Time{}
	}
	p.tokenMu.RLock()
	defer p.tokenMu.RUnlock()
	return p.expiresAt
}

func (p *seedTokenProvider) RemainingTTL(now time.Time) time.Duration {
	expiresAt := p.ExpiresAt()
	if expiresAt.IsZero() {
		return 0
	}
	return expiresAt.Sub(now)
}

func (p *seedTokenProvider) shouldRefresh(now time.Time, minTTL time.Duration) bool {
	if p == nil {
		return false
	}
	expiresAt := p.ExpiresAt()
	if expiresAt.IsZero() {
		return false
	}
	if minTTL < 0 {
		minTTL = 0
	}
	return !expiresAt.After(now.Add(minTTL))
}

func (p *seedTokenProvider) RefreshIfNeeded(ctx context.Context, minTTL time.Duration) (bool, error) {
	if p == nil {
		return false, nil
	}

	now := time.Now()
	if !p.shouldRefresh(now, minTTL) {
		return false, nil
	}

	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	now = time.Now()
	if !p.shouldRefresh(now, minTTL) {
		return false, nil
	}
	if p.refresher == nil {
		if expiresAt := p.ExpiresAt(); expiresAt.IsZero() || expiresAt.After(now) {
			return false, nil
		}
		return false, fmt.Errorf("token refresher not configured")
	}

	token, err := p.refresher(ctx)
	if err != nil {
		if expiresAt := p.ExpiresAt(); expiresAt.IsZero() || expiresAt.After(now) {
			return false, nil
		}
		return false, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false, fmt.Errorf("token refresher returned empty token")
	}
	p.SetToken(token)
	return true, nil
}

func (p *seedTokenProvider) Refresh(ctx context.Context, staleToken string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("token provider is nil")
	}
	current := p.Token()
	if current != "" && staleToken != "" && current != staleToken {
		return current, nil
	}

	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()

	current = p.Token()
	if current != "" && staleToken != "" && current != staleToken {
		return current, nil
	}
	if p.refresher == nil {
		return "", fmt.Errorf("token refresher not configured")
	}

	token, err := p.refresher(ctx)
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("token refresher returned empty token")
	}
	p.SetToken(token)
	return token, nil
}

func (c *APIClient) SetTokenProvider(provider *seedTokenProvider) {
	c.provider = provider
	if provider == nil {
		return
	}
	if c.refresher != nil {
		provider.SetRefresher(c.refresher)
	}
	if token := strings.TrimSpace(c.getToken()); token != "" {
		provider.SetToken(token)
	}
}

// SetRetryConfig updates retry settings for the client.
func (c *APIClient) SetRetryConfig(cfg RetryConfig) {
	retryMax, retryMinDelay, retryMaxDelay := defaultRetryConfig()
	if cfg.MaxRetries >= 0 {
		retryMax = cfg.MaxRetries
	}
	if strings.TrimSpace(cfg.MinDelay) != "" {
		if d, err := time.ParseDuration(strings.TrimSpace(cfg.MinDelay)); err == nil {
			retryMinDelay = d
		}
	}
	if strings.TrimSpace(cfg.MaxDelay) != "" {
		if d, err := time.ParseDuration(strings.TrimSpace(cfg.MaxDelay)); err == nil {
			retryMaxDelay = d
		}
	}
	if retryMinDelay <= 0 {
		retryMinDelay = 200 * time.Millisecond
	}
	if retryMaxDelay <= 0 {
		retryMaxDelay = 5 * time.Second
	}
	if retryMax < 0 {
		retryMax = 0
	}
	c.retryMax = retryMax
	c.retryMinDelay = retryMinDelay
	c.retryMaxDelay = retryMaxDelay
}

// SetToken updates the client token safely.
func (c *APIClient) SetToken(token string) {
	if c.provider != nil {
		c.provider.SetToken(token)
	}
	c.tokenMu.Lock()
	c.token = strings.TrimSpace(token)
	c.tokenMu.Unlock()
}

func (c *APIClient) getLocalToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.token
}

func (c *APIClient) getToken() string {
	if c.provider != nil {
		if token := c.provider.Token(); token != "" {
			return token
		}
	}
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.token
}

func normalizeSeedCacheKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// Response 通用 API 响应
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// QuestionnaireResponse 问卷响应
type QuestionnaireResponse struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

// ScaleResponse 量表响应
type ScaleResponse struct {
	Code                 string `json:"code"`
	Title                string `json:"title"`
	Status               string `json:"status"`
	Version              string `json:"version"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
}

// PlanResponse 计划响应。
type PlanResponse struct {
	ID            string   `json:"id"`
	OrgID         int64    `json:"org_id"`
	ScaleCode     string   `json:"scale_code"`
	ScheduleType  string   `json:"schedule_type"`
	TriggerTime   string   `json:"trigger_time"`
	Interval      int      `json:"interval"`
	TotalTimes    int      `json:"total_times"`
	FixedDates    []string `json:"fixed_dates"`
	RelativeWeeks []int    `json:"relative_weeks"`
	Status        string   `json:"status"`
}

// TaskResponse 计划任务响应。
type TaskResponse struct {
	ID           string  `json:"id"`
	PlanID       string  `json:"plan_id"`
	Seq          int     `json:"seq"`
	OrgID        int64   `json:"org_id"`
	TesteeID     string  `json:"testee_id"`
	ScaleCode    string  `json:"scale_code"`
	PlannedAt    string  `json:"planned_at"`
	OpenAt       *string `json:"open_at,omitempty"`
	ExpireAt     *string `json:"expire_at,omitempty"`
	CompletedAt  *string `json:"completed_at,omitempty"`
	Status       string  `json:"status"`
	AssessmentID *string `json:"assessment_id,omitempty"`
	EntryToken   string  `json:"entry_token,omitempty"`
	EntryURL     string  `json:"entry_url,omitempty"`
}

// TaskScheduleStatsResponse 任务调度统计。
type TaskScheduleStatsResponse struct {
	PendingCount      int `json:"pending_count"`
	OpenedCount       int `json:"opened_count"`
	FailedCount       int `json:"failed_count"`
	ExpiredCount      int `json:"expired_count"`
	ExpireFailedCount int `json:"expire_failed_count"`
}

// TaskListResponse 任务列表响应。
type TaskListResponse struct {
	Tasks      []TaskResponse             `json:"tasks"`
	TotalCount int64                      `json:"total_count"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"page_size"`
	Stats      *TaskScheduleStatsResponse `json:"stats,omitempty"`
}

// EnrollmentResponse 加入计划响应。
type EnrollmentResponse struct {
	PlanID string         `json:"plan_id"`
	Tasks  []TaskResponse `json:"tasks"`
}

// SchedulePendingTasksRequest 调度待开放任务请求。
type SchedulePendingTasksRequest struct {
	Before    string   `json:"before,omitempty"`
	Source    string   `json:"source,omitempty"`
	PlanID    string   `json:"plan_id,omitempty"`
	TesteeIDs []string `json:"testee_ids,omitempty"`
}

// CollectionScaleSummary 量表摘要（collection-server）
type CollectionScaleSummary struct {
	Code                 string   `json:"code"`
	Title                string   `json:"title"`
	Category             string   `json:"category"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	Status               string   `json:"status"`
	QuestionCount        int32    `json:"question_count"`
	Tags                 []string `json:"tags"`
	Reporters            []string `json:"reporters"`
}

// CollectionListScalesResponse 量表列表响应（collection-server）
type CollectionListScalesResponse struct {
	Scales   []CollectionScaleSummary `json:"scales"`
	Total    int64                    `json:"total"`
	Page     int32                    `json:"page"`
	PageSize int32                    `json:"page_size"`
}

// ApiserverTesteeResponse 受试者响应（apiserver）
type ApiserverTesteeResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ApiserverTesteeListResponse 受试者列表响应（apiserver）
type ApiserverTesteeListResponse struct {
	Items      []*ApiserverTesteeResponse `json:"items"`
	Total      int64                      `json:"total"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"page_size"`
	TotalPages int                        `json:"total_pages"`
}

// TesteeResponse 受试者响应（collection-server）
type TesteeResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// ListTesteesResponse 受试者列表响应（collection-server）
type ListTesteesResponse struct {
	Items []*TesteeResponse `json:"items"`
	Total int64             `json:"total"`
}

// QuestionnaireSummaryResponse 问卷摘要响应（collection-server）
type QuestionnaireSummaryResponse struct {
	Code          string `json:"code"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	Type          string `json:"type"`
	QuestionCount int32  `json:"question_count"`
}

// ListQuestionnairesResponse 问卷列表响应（collection-server）
type ListQuestionnairesResponse struct {
	Questionnaires []QuestionnaireSummaryResponse `json:"questionnaires"`
	Total          int64                          `json:"total"`
	Page           int32                          `json:"page"`
	PageSize       int32                          `json:"page_size"`
}

// QuestionnaireDetailResponse 问卷详情响应（collection-server）
type QuestionnaireDetailResponse struct {
	Code      string             `json:"code"`
	Title     string             `json:"title"`
	Status    string             `json:"status"`
	Version   string             `json:"version"`
	Type      string             `json:"type"`
	Questions []QuestionResponse `json:"questions"`
}

// QuestionResponse 问题响应（collection-server）
type QuestionResponse struct {
	Code    string           `json:"code"`
	Type    string           `json:"type"`
	Title   string           `json:"title"`
	Options []OptionResponse `json:"options"`
}

// OptionResponse 选项响应（collection-server）
type OptionResponse struct {
	Code    string `json:"code"`
	Content string `json:"content"`
	Score   int32  `json:"score"`
}

// CreateQuestionnaireRequest 创建问卷请求
type CreateQuestionnaireRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
	Type        string `json:"type"`
}

// CreateScaleRequest 创建量表请求
type CreateScaleRequest struct {
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	Category             string   `json:"category"`
	Stages               []string `json:"stages"`
	ApplicableAges       []string `json:"applicable_ages"`
	Reporters            []string `json:"reporters"`
	Tags                 []string `json:"tags"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
}

// QuestionDTO 问题 DTO（匹配 viewmodel.QuestionDTO）
type QuestionDTO struct {
	Code        string      `json:"code"`
	Type        string      `json:"question_type"` // API 期望的字段名
	Stem        string      `json:"stem"`
	Tips        string      `json:"tips,omitempty"` // API 期望的字段名
	Options     []OptionDTO `json:"options,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	// 以下字段可选，暂时不设置
	// ValidationRules []ValidationRuleDTO `json:"validation_rules,omitempty"`
	// CalculationRule *CalculationRuleDTO `json:"calculation_rule,omitempty"`
	// ShowController  *ShowControllerDTO  `json:"show_controller,omitempty"`
}

// OptionDTO 选项 DTO（匹配 viewmodel.OptionDTO）
type OptionDTO struct {
	Code    string  `json:"code"`    // API 期望的字段名
	Content string  `json:"content"` // API 期望的字段名
	Score   float64 `json:"score"`   // API 期望是 float64
}

// BatchUpdateQuestionsRequest 批量更新问题请求
type BatchUpdateQuestionsRequest struct {
	Questions []QuestionDTO `json:"questions"`
}

// FactorDTO 因子 DTO
type FactorDTO struct {
	Code            string             `json:"code"`
	Title           string             `json:"title"`
	FactorType      string             `json:"factor_type"`
	IsTotalScore    bool               `json:"is_total_score"`
	QuestionCodes   []string           `json:"question_codes"`
	ScoringStrategy string             `json:"scoring_strategy"`
	ScoringParams   *ScoringParamsDTO  `json:"scoring_params,omitempty"`
	InterpretRules  []InterpretRuleDTO `json:"interpret_rules"`
}

// ScoringParamsDTO 计分参数 DTO
type ScoringParamsDTO struct {
	CntOptionContents []string `json:"cnt_option_contents,omitempty"`
}

// InterpretRuleDTO 解读规则 DTO
type InterpretRuleDTO struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	RiskLevel  string  `json:"risk_level"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion"`
}

// BatchUpdateFactorsRequest 批量更新因子请求
type BatchUpdateFactorsRequest struct {
	Factors []FactorDTO `json:"factors"`
}

// EnrollTesteeRequest 计划入组请求（apiserver）。
type EnrollTesteeRequest struct {
	PlanID    string `json:"plan_id"`
	TesteeID  string `json:"testee_id"`
	StartDate string `json:"start_date"`
}

// SubmitAnswerSheetRequest 提交答卷请求（collection-server）
type SubmitAnswerSheetRequest struct {
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	Title                string   `json:"title"`
	TesteeID             uint64   `json:"testee_id"`
	TaskID               string   `json:"task_id,omitempty"`
	TaskCompletedAt      string   `json:"task_completed_at,omitempty"`
	Answers              []Answer `json:"answers"`
}

// AdminSubmitAnswerSheetRequest 管理员提交答卷请求（apiserver）
type AdminSubmitAnswerSheetRequest struct {
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	Title                string   `json:"title"`
	TesteeID             uint64   `json:"testee_id"`
	TaskID               string   `json:"task_id,omitempty"`
	TaskCompletedAt      string   `json:"task_completed_at,omitempty"`
	WriterID             uint64   `json:"writer_id,omitempty"`
	FillerID             uint64   `json:"filler_id,omitempty"`
	Answers              []Answer `json:"answers"`
}

// Answer 提交答案
// 注意：Value 使用 interface{} 类型，以支持不同类型的问题答案：
// - Radio: string (选项 code)
// - Checkbox: []string (选项 code 数组)
// - Text/Textarea: string
// - Number: number (float64)
// 注意：Score 字段在 admin-submit 接口中会被忽略，但保留以兼容其他接口
type Answer struct {
	QuestionCode string      `json:"question_code"`
	QuestionType string      `json:"question_type"`
	Score        uint32      `json:"score,omitempty"` // 使用 omitempty，避免发送不必要的字段
	Value        interface{} `json:"value"`
}

// SubmitAnswerSheetResponse 提交答卷响应（collection-server）
type SubmitAnswerSheetResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// doRequest 执行 HTTP 请求
func (c *APIClient) doRequest(ctx context.Context, method, path string, body interface{}) (*Response, error) {
	return c.doRequestWithRetryAndTimeout(ctx, method, path, body, true, c.httpClient.Timeout)
}

func (c *APIClient) doRequestWithRetry(ctx context.Context, method, path string, body interface{}, allowRefresh bool) (*Response, error) {
	return c.doRequestWithRetryAndTimeout(ctx, method, path, body, allowRefresh, c.httpClient.Timeout)
}

func (c *APIClient) doRequestWithRetryAndTimeout(ctx context.Context, method, path string, body interface{}, allowRefresh bool, timeout time.Duration) (*Response, error) {
	return c.doRequestWithRetryTimeoutAndLimit(ctx, method, path, body, allowRefresh, timeout, c.retryMax)
}

func (c *APIClient) doRequestWithRetryTimeoutAndLimit(
	ctx context.Context,
	method, path string,
	body interface{},
	allowRefresh bool,
	timeout time.Duration,
	retryMax int,
) (*Response, error) {
	url := c.baseURL + path
	httpClient := *c.httpClient
	if timeout > 0 {
		httpClient.Timeout = timeout
	}
	if retryMax < 0 {
		retryMax = 0
	}
	for attempt := 0; attempt <= retryMax; attempt++ {
		if err := c.ensureFreshToken(ctx); err != nil {
			return nil, err
		}

		var reqBody io.Reader
		if body != nil {
			jsonData, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			reqBody = bytes.NewBuffer(jsonData)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if token := c.getToken(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if shouldRetryNetErr(err) && attempt < retryMax {
				delay := backoffDelay(attempt, c.retryMinDelay, c.retryMaxDelay)
				c.logger.Warnw("seeddata request failed, retrying",
					"url", url,
					"attempt", attempt+1,
					"delay_ms", delay.Milliseconds(),
					"error", err.Error(),
				)
				if err := sleepWithContext(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("do request: %w", err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		if resp.StatusCode == http.StatusOK {
			var apiResp Response
			if err := json.Unmarshal(respBody, &apiResp); err != nil {
				bodyStr := string(respBody)
				if len(bodyStr) > 200 {
					bodyStr = bodyStr[:200] + "..."
				}
				return nil, fmt.Errorf("unmarshal response: %w, url=%s, body=%s", err, url, bodyStr)
			}
			if apiResp.Code != 0 {
				return nil, fmt.Errorf("api error: code=%d, message=%s, http_status=%d", apiResp.Code, apiResp.Message, resp.StatusCode)
			}
			return &apiResp, nil
		}

		if resp.StatusCode == http.StatusUnauthorized {
			var apiResp Response
			if err := json.Unmarshal(respBody, &apiResp); err == nil {
				if allowRefresh && c.refresher != nil {
					if err := c.refreshToken(ctx); err == nil {
						return c.doRequestWithRetryTimeoutAndLimit(ctx, method, path, body, false, timeout, retryMax)
					}
				}
				return nil, fmt.Errorf("authentication failed (401): please check your API token. message=%s", apiResp.Message)
			}
			if allowRefresh && c.refresher != nil {
				if err := c.refreshToken(ctx); err == nil {
					return c.doRequestWithRetryTimeoutAndLimit(ctx, method, path, body, false, timeout, retryMax)
				}
			}
			return nil, fmt.Errorf("authentication failed (401): please check your API token. url=%s", url)
		}

		if isRetryableStatus(resp.StatusCode) && attempt < retryMax {
			delay, ok := parseRetryAfter(resp.Header.Get("Retry-After"))
			if !ok {
				delay = backoffDelay(attempt, c.retryMinDelay, c.retryMaxDelay)
			}
			if delay > c.retryMaxDelay {
				delay = c.retryMaxDelay
			}
			c.logger.Warnw("seeddata request throttled, retrying",
				"url", url,
				"status", resp.StatusCode,
				"attempt", attempt+1,
				"delay_ms", delay.Milliseconds(),
			)
			if err := sleepWithContext(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}

		var apiResp Response
		if err := json.Unmarshal(respBody, &apiResp); err == nil {
			bodyStr := string(respBody)
			if resp.StatusCode == http.StatusInternalServerError {
				c.logger.Warnw("API returned 500 error",
					"url", url,
					"code", apiResp.Code,
					"message", apiResp.Message,
					"full_response", bodyStr,
				)
			}
			if len(bodyStr) > 500 {
				bodyStr = bodyStr[:500] + "..."
			}
			return nil, fmt.Errorf("api error: http_status=%d, code=%d, message=%s, body=%s", resp.StatusCode, apiResp.Code, apiResp.Message, bodyStr)
		}

		bodyStr := string(respBody)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		return nil, fmt.Errorf("http error: status=%d, url=%s, body=%s", resp.StatusCode, url, bodyStr)
	}

	return nil, fmt.Errorf("request failed after retries: url=%s", url)
}

func defaultRetryConfig() (int, time.Duration, time.Duration) {
	return 3, 200 * time.Millisecond, 5 * time.Second
}

func shouldRetryNetErr(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	return false
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func parseRetryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(value); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(value); err == nil {
		delay := time.Until(t)
		if delay < 0 {
			return 0, false
		}
		return delay, true
	}
	return 0, false
}

func backoffDelay(attempt int, minDelay, maxDelay time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := minDelay << attempt
	if delay > maxDelay {
		delay = maxDelay
	}
	jitter := time.Duration(time.Now().UnixNano()%int64(delay/2+1)) - delay/4
	return delay + jitter
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *APIClient) refreshToken(ctx context.Context) error {
	current := strings.TrimSpace(c.getLocalToken())
	if current == "" {
		current = c.getToken()
	}
	if c.provider != nil {
		token, err := c.provider.Refresh(ctx, current)
		if err != nil {
			return err
		}
		c.SetToken(token)
		return nil
	}
	if c.refresher == nil {
		return fmt.Errorf("token refresher not configured")
	}
	token, err := c.refresher(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("token refresher returned empty token")
	}
	c.SetToken(token)
	return nil
}

func (c *APIClient) ensureFreshToken(ctx context.Context) error {
	if c.provider == nil {
		return nil
	}

	remainingTTL := c.provider.RemainingTTL(time.Now())
	refreshed, err := c.provider.RefreshIfNeeded(ctx, seedTokenRefreshSkew)
	if err != nil {
		return fmt.Errorf("refresh api token before request: %w", err)
	}
	if refreshed {
		c.SetToken(c.provider.Token())
		c.logger.Infow("Seeddata proactively refreshed API token",
			"base_url", c.baseURL,
			"previous_remaining_seconds", int64(remainingTTL/time.Second),
			"expires_at", c.provider.ExpiresAt(),
		)
	}
	return nil
}

// CreateQuestionnaire 创建问卷
func (c *APIClient) CreateQuestionnaire(ctx context.Context, req CreateQuestionnaireRequest) (*QuestionnaireResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/questionnaires", req)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var qResp QuestionnaireResponse
	if err := json.Unmarshal(dataBytes, &qResp); err != nil {
		return nil, fmt.Errorf("unmarshal questionnaire response: %w", err)
	}

	return &qResp, nil
}

// UpdateQuestionnaireBasicInfo 更新问卷基本信息
func (c *APIClient) UpdateQuestionnaireBasicInfo(ctx context.Context, code string, req CreateQuestionnaireRequest) (*QuestionnaireResponse, error) {
	resp, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/questionnaires/%s/basic-info", code), req)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var qResp QuestionnaireResponse
	if err := json.Unmarshal(dataBytes, &qResp); err != nil {
		return nil, fmt.Errorf("unmarshal questionnaire response: %w", err)
	}

	return &qResp, nil
}

// BatchUpdateQuestions 批量更新问题
func (c *APIClient) BatchUpdateQuestions(ctx context.Context, code string, req BatchUpdateQuestionsRequest) error {
	_, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/questionnaires/%s/questions/batch", code), req)
	return err
}

// SaveDraftQuestionnaire 保存草稿
func (c *APIClient) SaveDraftQuestionnaire(ctx context.Context, code string) (*QuestionnaireResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/questionnaires/%s/draft", code), nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var qResp QuestionnaireResponse
	if err := json.Unmarshal(dataBytes, &qResp); err != nil {
		return nil, fmt.Errorf("unmarshal questionnaire response: %w", err)
	}

	return &qResp, nil
}

// UnpublishQuestionnaire 下架问卷（将已发布的问卷变为草稿状态）
func (c *APIClient) UnpublishQuestionnaire(ctx context.Context, code string) (*QuestionnaireResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/questionnaires/%s/unpublish", code), nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var qResp QuestionnaireResponse
	if err := json.Unmarshal(dataBytes, &qResp); err != nil {
		return nil, fmt.Errorf("unmarshal questionnaire response: %w", err)
	}

	return &qResp, nil
}

// PublishQuestionnaire 发布问卷
func (c *APIClient) PublishQuestionnaire(ctx context.Context, code string) (*QuestionnaireResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/questionnaires/%s/publish", code), nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var qResp QuestionnaireResponse
	if err := json.Unmarshal(dataBytes, &qResp); err != nil {
		return nil, fmt.Errorf("unmarshal questionnaire response: %w", err)
	}

	return &qResp, nil
}

// GetQuestionnaire 获取问卷详情
func (c *APIClient) GetQuestionnaire(ctx context.Context, code string) (*QuestionnaireResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/questionnaires/%s", code), nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var qResp QuestionnaireResponse
	if err := json.Unmarshal(dataBytes, &qResp); err != nil {
		return nil, fmt.Errorf("unmarshal questionnaire response: %w", err)
	}

	return &qResp, nil
}

// CreateScale 创建量表
func (c *APIClient) CreateScale(ctx context.Context, req CreateScaleRequest) (*ScaleResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/scales", req)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var sResp ScaleResponse
	if err := json.Unmarshal(dataBytes, &sResp); err != nil {
		return nil, fmt.Errorf("unmarshal scale response: %w", err)
	}

	return &sResp, nil
}

// UpdateScaleBasicInfo 更新量表基本信息
func (c *APIClient) UpdateScaleBasicInfo(ctx context.Context, code string, req CreateScaleRequest) (*ScaleResponse, error) {
	updateReq := map[string]interface{}{
		"title":           req.Title,
		"description":     req.Description,
		"category":        req.Category,
		"stages":          req.Stages,
		"applicable_ages": req.ApplicableAges,
		"reporters":       req.Reporters,
		"tags":            req.Tags,
	}

	resp, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/scales/%s/basic-info", code), updateReq)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var sResp ScaleResponse
	if err := json.Unmarshal(dataBytes, &sResp); err != nil {
		return nil, fmt.Errorf("unmarshal scale response: %w", err)
	}

	return &sResp, nil
}

// UpdateScaleQuestionnaire 更新量表关联问卷
func (c *APIClient) UpdateScaleQuestionnaire(ctx context.Context, code string, questionnaireCode, questionnaireVersion string) (*ScaleResponse, error) {
	updateReq := map[string]interface{}{
		"questionnaire_code":    questionnaireCode,
		"questionnaire_version": questionnaireVersion,
	}

	resp, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/scales/%s/questionnaire", code), updateReq)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var sResp ScaleResponse
	if err := json.Unmarshal(dataBytes, &sResp); err != nil {
		return nil, fmt.Errorf("unmarshal scale response: %w", err)
	}

	return &sResp, nil
}

// BatchUpdateFactors 批量更新因子
func (c *APIClient) BatchUpdateFactors(ctx context.Context, code string, req BatchUpdateFactorsRequest) error {
	_, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/scales/%s/factors/batch", code), req)
	return err
}

// PublishScale 发布量表
func (c *APIClient) PublishScale(ctx context.Context, code string) (*ScaleResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/scales/%s/publish", code), nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var sResp ScaleResponse
	if err := json.Unmarshal(dataBytes, &sResp); err != nil {
		return nil, fmt.Errorf("unmarshal scale response: %w", err)
	}

	return &sResp, nil
}

// GetScale 获取量表详情
func (c *APIClient) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	cacheKey := normalizeSeedCacheKey(code)
	if cacheKey != "" {
		c.scaleCacheMu.RLock()
		cached := c.scaleCache[cacheKey]
		c.scaleCacheMu.RUnlock()
		if cached != nil {
			cloned := *cached
			return &cloned, nil
		}
	}

	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/scales/%s", code), nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var sResp ScaleResponse
	if err := json.Unmarshal(dataBytes, &sResp); err != nil {
		return nil, fmt.Errorf("unmarshal scale response: %w", err)
	}

	if cacheKey != "" {
		cloned := sResp
		c.scaleCacheMu.Lock()
		c.scaleCache[cacheKey] = &cloned
		c.scaleCacheMu.Unlock()
	}

	return &sResp, nil
}

func decodeResponseData(resp *Response, out interface{}) error {
	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return fmt.Errorf("marshal response data: %w", err)
	}
	if err := json.Unmarshal(dataBytes, out); err != nil {
		return fmt.Errorf("unmarshal response data: %w", err)
	}
	return nil
}

// GetPlan 获取计划详情（apiserver）。
func (c *APIClient) GetPlan(ctx context.Context, planID string) (*PlanResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/plans/%s", planID), nil)
	if err != nil {
		return nil, err
	}

	var planResp PlanResponse
	if err := decodeResponseData(resp, &planResp); err != nil {
		return nil, fmt.Errorf("unmarshal plan response: %w", err)
	}
	return &planResp, nil
}

// EnrollTestee 将受试者加入计划（apiserver）。
func (c *APIClient) EnrollTestee(ctx context.Context, req EnrollTesteeRequest) (*EnrollmentResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/plans/enroll", req)
	if err != nil {
		return nil, err
	}

	var enrollResp EnrollmentResponse
	if err := decodeResponseData(resp, &enrollResp); err != nil {
		return nil, fmt.Errorf("unmarshal enrollment response: %w", err)
	}
	return &enrollResp, nil
}

// SchedulePendingTasks 调度待开放任务（apiserver internal API）。
func (c *APIClient) SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error) {
	path := "/internal/v1/plans/tasks/schedule"
	query := url.Values{}
	if strings.TrimSpace(req.Before) != "" {
		query.Set("before", strings.TrimSpace(req.Before))
	}
	if strings.TrimSpace(req.Source) != "" {
		query.Set("source", strings.TrimSpace(req.Source))
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	body := struct {
		PlanID    string   `json:"plan_id,omitempty"`
		TesteeIDs []string `json:"testee_ids,omitempty"`
	}{
		PlanID:    strings.TrimSpace(req.PlanID),
		TesteeIDs: append([]string(nil), req.TesteeIDs...),
	}
	if body.PlanID == "" && len(body.TesteeIDs) == 0 {
		resp, err := c.doRequestWithRetryAndTimeout(ctx, "POST", path, nil, true, planScheduleRequestTimeout)
		if err != nil {
			return nil, err
		}

		var taskList TaskListResponse
		if err := decodeResponseData(resp, &taskList); err != nil {
			return nil, fmt.Errorf("unmarshal schedule response: %w", err)
		}
		return &taskList, nil
	}

	resp, err := c.doRequestWithRetryAndTimeout(ctx, "POST", path, body, true, planScheduleRequestTimeout)
	if err != nil {
		return nil, err
	}

	var taskList TaskListResponse
	if err := decodeResponseData(resp, &taskList); err != nil {
		return nil, fmt.Errorf("unmarshal schedule response: %w", err)
	}
	return &taskList, nil
}

// ListTasksByTesteeAndPlan 查询受试者在指定计划下的任务。
func (c *APIClient) ListTasksByTesteeAndPlan(ctx context.Context, testeeID, planID string) (*TaskListResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/testees/%s/plans/%s/tasks", testeeID, planID), nil)
	if err != nil {
		return nil, err
	}

	var taskList TaskListResponse
	if err := decodeResponseData(resp, &taskList); err != nil {
		return nil, fmt.Errorf("unmarshal plan task list response: %w", err)
	}
	return &taskList, nil
}

// GetTask 获取任务详情（apiserver）。
func (c *APIClient) GetTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/plans/tasks/%s", taskID), nil)
	if err != nil {
		return nil, err
	}

	var taskResp TaskResponse
	if err := decodeResponseData(resp, &taskResp); err != nil {
		return nil, fmt.Errorf("unmarshal task response: %w", err)
	}
	return &taskResp, nil
}

// ExpireTask 将计划任务标记为过期（apiserver internal API）。
func (c *APIClient) ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/internal/v1/plans/tasks/%s/expire", taskID), nil)
	if err != nil {
		return nil, err
	}

	var taskResp TaskResponse
	if err := decodeResponseData(resp, &taskResp); err != nil {
		return nil, fmt.Errorf("unmarshal expire task response: %w", err)
	}
	return &taskResp, nil
}

// ListTestees 获取受试者列表（collection-server）
func (c *APIClient) ListTestees(ctx context.Context, offset, limit int) (*ListTesteesResponse, error) {
	path := fmt.Sprintf("/api/v1/testees?offset=%d&limit=%d", offset, limit)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var listResp ListTesteesResponse
	if err := json.Unmarshal(dataBytes, &listResp); err != nil {
		return nil, fmt.Errorf("unmarshal testees response: %w", err)
	}

	return &listResp, nil
}

// ListQuestionnaires 获取问卷列表（collection-server）
func (c *APIClient) ListQuestionnaires(ctx context.Context, page, pageSize int, status string) (*ListQuestionnairesResponse, error) {
	path := fmt.Sprintf("/api/v1/questionnaires?page=%d&page_size=%d&status=%s", page, pageSize, status)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var listResp ListQuestionnairesResponse
	if err := json.Unmarshal(dataBytes, &listResp); err != nil {
		return nil, fmt.Errorf("unmarshal questionnaires response: %w", err)
	}

	return &listResp, nil
}

// ListScales 获取量表列表（collection-server）
func (c *APIClient) ListScales(ctx context.Context, page, pageSize int, status, category string) (*CollectionListScalesResponse, error) {
	path := fmt.Sprintf("/api/v1/scales?page=%d&page_size=%d&status=%s&category=%s", page, pageSize, status, category)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var listResp CollectionListScalesResponse
	if err := json.Unmarshal(dataBytes, &listResp); err != nil {
		return nil, fmt.Errorf("unmarshal scales response: %w", err)
	}

	return &listResp, nil
}

// ListTesteesByOrg 获取受试者列表（apiserver）
func (c *APIClient) ListTesteesByOrg(ctx context.Context, orgID int64, page, pageSize int) (*ApiserverTesteeListResponse, error) {
	path := fmt.Sprintf("/api/v1/testees?org_id=%d&page=%d&page_size=%d", orgID, page, pageSize)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list testees: org_id=%d page=%d page_size=%d: %w", orgID, page, pageSize, err)
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var listResp ApiserverTesteeListResponse
	if err := json.Unmarshal(dataBytes, &listResp); err != nil {
		return nil, fmt.Errorf("unmarshal testees response: %w", err)
	}

	return &listResp, nil
}

// GetTesteeByID 获取受试者详情（apiserver）。
func (c *APIClient) GetTesteeByID(ctx context.Context, testeeID string) (*ApiserverTesteeResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/testees/%s", testeeID), nil)
	if err != nil {
		return nil, fmt.Errorf("get testee: id=%s: %w", testeeID, err)
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var testeeResp ApiserverTesteeResponse
	if err := json.Unmarshal(dataBytes, &testeeResp); err != nil {
		return nil, fmt.Errorf("unmarshal testee response: %w", err)
	}

	return &testeeResp, nil
}

// GetQuestionnaireDetail 获取问卷详情（collection-server）
func (c *APIClient) GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error) {
	cacheKey := normalizeSeedCacheKey(code)
	if cacheKey != "" {
		c.questionnaireCacheMu.RLock()
		cached := c.questionnaireCache[cacheKey]
		c.questionnaireCacheMu.RUnlock()
		if cached != nil {
			cloned := *cached
			return &cloned, nil
		}
	}

	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/questionnaires/%s", code), nil)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var detailResp QuestionnaireDetailResponse
	if err := json.Unmarshal(dataBytes, &detailResp); err != nil {
		return nil, fmt.Errorf("unmarshal questionnaire response: %w", err)
	}

	if cacheKey != "" {
		cloned := detailResp
		c.questionnaireCacheMu.Lock()
		c.questionnaireCache[cacheKey] = &cloned
		c.questionnaireCacheMu.Unlock()
	}

	return &detailResp, nil
}

// SubmitAnswerSheet 提交答卷（collection-server）
func (c *APIClient) SubmitAnswerSheet(ctx context.Context, req SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/answersheets", req)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var submitResp SubmitAnswerSheetResponse
	if err := json.Unmarshal(dataBytes, &submitResp); err != nil {
		return nil, fmt.Errorf("unmarshal submit response: %w", err)
	}

	return &submitResp, nil
}

// SubmitAnswerSheetAdmin 管理员提交答卷（apiserver）
func (c *APIClient) SubmitAnswerSheetAdmin(ctx context.Context, req AdminSubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/answersheets/admin-submit", req)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var submitResp SubmitAnswerSheetResponse
	if err := json.Unmarshal(dataBytes, &submitResp); err != nil {
		return nil, fmt.Errorf("unmarshal submit response: %w", err)
	}

	return &submitResp, nil
}

func (c *APIClient) SubmitAnswerSheetAdminWithPolicy(
	ctx context.Context,
	req AdminSubmitAnswerSheetRequest,
	timeout time.Duration,
	retryMax int,
) (*SubmitAnswerSheetResponse, error) {
	resp, err := c.doRequestWithRetryTimeoutAndLimit(ctx, "POST", "/api/v1/answersheets/admin-submit", req, true, timeout, retryMax)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var submitResp SubmitAnswerSheetResponse
	if err := json.Unmarshal(dataBytes, &submitResp); err != nil {
		return nil, fmt.Errorf("unmarshal submit response: %w", err)
	}

	return &submitResp, nil
}

// ============= Assessment 相关类型和接口 =============

// CreateAssessmentRequest 创建测评请求（apiserver）
type CreateAssessmentRequest struct {
	TesteeID             uint64  `json:"testee_id"`
	QuestionnaireCode    string  `json:"questionnaire_code"`
	QuestionnaireVersion string  `json:"questionnaire_version"`
	AnswerSheetID        uint64  `json:"answer_sheet_id"`
	MedicalScaleID       *uint64 `json:"medical_scale_id,omitempty"`
	MedicalScaleCode     *string `json:"medical_scale_code,omitempty"`
	MedicalScaleName     *string `json:"medical_scale_name,omitempty"`
	OriginType           string  `json:"origin_type"`
	OriginID             *string `json:"origin_id,omitempty"`
}

// SubmitAssessmentRequest 提交测评请求（apiserver）
type SubmitAssessmentRequest struct {
	AssessmentID uint64 `json:"assessment_id"`
}

// AssessmentResponse 测评响应（apiserver）
type AssessmentResponse struct {
	ID                string   `json:"id"`
	TesteeID          string   `json:"testee_id"`
	QuestionnaireCode string   `json:"questionnaire_code"`
	Status            string   `json:"status"`
	TotalScore        *float64 `json:"total_score,omitempty"`
	RiskLevel         *string  `json:"risk_level,omitempty"`
}

// CreateAssessment 创建测评（apiserver）
func (c *APIClient) CreateAssessment(ctx context.Context, req CreateAssessmentRequest) (*AssessmentResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/evaluations/assessments", req)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var assessmentResp AssessmentResponse
	if err := json.Unmarshal(dataBytes, &assessmentResp); err != nil {
		return nil, fmt.Errorf("unmarshal assessment response: %w", err)
	}

	return &assessmentResp, nil
}

// SubmitAssessment 提交测评（apiserver）
func (c *APIClient) SubmitAssessment(ctx context.Context, req SubmitAssessmentRequest) (*AssessmentResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/evaluations/assessments/submit", req)
	if err != nil {
		return nil, err
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var assessmentResp AssessmentResponse
	if err := json.Unmarshal(dataBytes, &assessmentResp); err != nil {
		return nil, fmt.Errorf("unmarshal assessment response: %w", err)
	}

	return &assessmentResp, nil
}
