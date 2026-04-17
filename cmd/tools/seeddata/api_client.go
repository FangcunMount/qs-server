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

// PlanTaskWindowResponse 任务窗口响应。
type PlanTaskWindowResponse struct {
	Tasks    []TaskResponse `json:"tasks"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
	HasMore  bool           `json:"has_more"`
}

// ListTasksRequest 任务分页查询请求。
type ListTasksRequest struct {
	PlanID   string
	TesteeID string
	Status   string
	Page     int
	PageSize int
}

// ListPlanTaskWindowRequest 查询任务窗口请求。
type ListPlanTaskWindowRequest struct {
	PlanID        string   `json:"plan_id"`
	Status        string   `json:"status,omitempty"`
	TesteeIDs     []string `json:"testee_ids,omitempty"`
	PlannedBefore string   `json:"planned_before,omitempty"`
	Page          int      `json:"page,omitempty"`
	PageSize      int      `json:"page_size,omitempty"`
}

// EnrollmentResponse 加入计划响应。
type EnrollmentResponse struct {
	PlanID string         `json:"plan_id"`
	Tasks  []TaskResponse `json:"tasks"`
}

// SchedulePendingTasksRequest 调度待开放任务请求。
type SchedulePendingTasksRequest struct {
	Before    string   `json:"before,omitempty"`
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
	ID        string     `json:"id"`
	OrgID     string     `json:"org_id,omitempty"`
	ProfileID *string    `json:"profile_id,omitempty"`
	Name      string     `json:"name"`
	Gender    string     `json:"gender,omitempty"`
	Birthday  *time.Time `json:"birthday,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (r *ApiserverTesteeResponse) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID        string  `json:"id"`
		OrgID     string  `json:"org_id,omitempty"`
		ProfileID *string `json:"profile_id,omitempty"`
		Name      string  `json:"name"`
		Gender    string  `json:"gender,omitempty"`
		Birthday  *string `json:"birthday,omitempty"`
		CreatedAt string  `json:"created_at"`
		UpdatedAt string  `json:"updated_at"`
	}

	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	birthday, err := parseFlexibleSeedNullableTime(raw.Birthday)
	if err != nil {
		return fmt.Errorf("parse birthday: %w", err)
	}
	createdAt, err := parseFlexibleSeedOptionalTimeValue(raw.CreatedAt)
	if err != nil {
		return fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseFlexibleSeedOptionalTimeValue(raw.UpdatedAt)
	if err != nil {
		return fmt.Errorf("parse updated_at: %w", err)
	}

	r.ID = raw.ID
	r.OrgID = raw.OrgID
	r.ProfileID = raw.ProfileID
	r.Name = raw.Name
	r.Gender = raw.Gender
	r.Birthday = birthday
	r.CreatedAt = createdAt
	r.UpdatedAt = updatedAt
	return nil
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

func (r *TesteeResponse) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at,omitempty"`
		UpdatedAt string `json:"updated_at,omitempty"`
	}

	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var createdAt time.Time
	var updatedAt time.Time
	var err error
	if strings.TrimSpace(raw.CreatedAt) != "" {
		createdAt, err = parseFlexibleSeedRequiredTime(raw.CreatedAt)
		if err != nil {
			return fmt.Errorf("parse created_at: %w", err)
		}
	}
	if strings.TrimSpace(raw.UpdatedAt) != "" {
		updatedAt, err = parseFlexibleSeedRequiredTime(raw.UpdatedAt)
		if err != nil {
			return fmt.Errorf("parse updated_at: %w", err)
		}
	}

	r.ID = raw.ID
	r.Name = raw.Name
	r.CreatedAt = createdAt
	r.UpdatedAt = updatedAt
	return nil
}

// StaffResponse 员工响应（apiserver）。
type StaffResponse struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	UserID    string    `json:"user_id"`
	Roles     []string  `json:"roles"`
	Name      string    `json:"name"`
	Email     string    `json:"email,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *StaffResponse) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID        string   `json:"id"`
		OrgID     string   `json:"org_id"`
		UserID    string   `json:"user_id"`
		Roles     []string `json:"roles"`
		Name      string   `json:"name"`
		Email     string   `json:"email,omitempty"`
		Phone     string   `json:"phone,omitempty"`
		IsActive  bool     `json:"is_active"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}

	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	createdAt, err := parseFlexibleSeedOptionalTimeValue(raw.CreatedAt)
	if err != nil {
		return fmt.Errorf("parse created_at: %w", err)
	}
	updatedAt, err := parseFlexibleSeedOptionalTimeValue(raw.UpdatedAt)
	if err != nil {
		return fmt.Errorf("parse updated_at: %w", err)
	}

	r.ID = raw.ID
	r.OrgID = raw.OrgID
	r.UserID = raw.UserID
	r.Roles = raw.Roles
	r.Name = raw.Name
	r.Email = raw.Email
	r.Phone = raw.Phone
	r.IsActive = raw.IsActive
	r.CreatedAt = createdAt
	r.UpdatedAt = updatedAt
	return nil
}

// StaffListResponse 员工列表响应（apiserver）。
type StaffListResponse struct {
	Items      []*StaffResponse `json:"items"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

// CreateStaffRequest 创建员工请求（apiserver）。
type CreateStaffRequest struct {
	OrgID    int64    `json:"org_id"`
	UserID   *uint64  `json:"user_id,omitempty"`
	Roles    []string `json:"roles"`
	Name     string   `json:"name"`
	Email    string   `json:"email,omitempty"`
	Phone    string   `json:"phone,omitempty"`
	Password string   `json:"password,omitempty"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// UpdateStaffRequest 更新员工请求（apiserver）。
type UpdateStaffRequest struct {
	Roles    []string `json:"roles,omitempty"`
	Name     *string  `json:"name,omitempty"`
	Email    *string  `json:"email,omitempty"`
	Phone    *string  `json:"phone,omitempty"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// ClinicianResponse 临床医师响应（apiserver）。
type ClinicianResponse struct {
	ID                   string  `json:"id"`
	OrgID                string  `json:"org_id"`
	OperatorID           *string `json:"operator_id,omitempty"`
	Name                 string  `json:"name"`
	Department           string  `json:"department,omitempty"`
	Title                string  `json:"title,omitempty"`
	ClinicianType        string  `json:"clinician_type"`
	EmployeeCode         string  `json:"employee_code,omitempty"`
	IsActive             bool    `json:"is_active"`
	AssignedTesteeCount  int64   `json:"assigned_testee_count"`
	AssessmentEntryCount int64   `json:"assessment_entry_count"`
}

// ClinicianListResponse 临床医师列表响应（apiserver）。
type ClinicianListResponse struct {
	Items      []*ClinicianResponse `json:"items"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
	TotalPages int                  `json:"total_pages"`
}

// AssessmentEntryResponse 测评入口响应（apiserver）。
type AssessmentEntryResponse struct {
	ID            string     `json:"id"`
	OrgID         string     `json:"org_id"`
	ClinicianID   string     `json:"clinician_id"`
	Token         string     `json:"token"`
	TargetType    string     `json:"target_type"`
	TargetCode    string     `json:"target_code"`
	TargetVersion string     `json:"target_version,omitempty"`
	IsActive      bool       `json:"is_active"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	QRCodeURL     string     `json:"qrcode_url,omitempty"`
}

func (r *AssessmentEntryResponse) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID            string  `json:"id"`
		OrgID         string  `json:"org_id"`
		ClinicianID   string  `json:"clinician_id"`
		Token         string  `json:"token"`
		TargetType    string  `json:"target_type"`
		TargetCode    string  `json:"target_code"`
		TargetVersion string  `json:"target_version,omitempty"`
		IsActive      bool    `json:"is_active"`
		ExpiresAt     *string `json:"expires_at,omitempty"`
		QRCodeURL     string  `json:"qrcode_url,omitempty"`
	}

	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	expiresAt, err := parseFlexibleSeedNullableTime(raw.ExpiresAt)
	if err != nil {
		return fmt.Errorf("parse expires_at: %w", err)
	}

	r.ID = raw.ID
	r.OrgID = raw.OrgID
	r.ClinicianID = raw.ClinicianID
	r.Token = raw.Token
	r.TargetType = raw.TargetType
	r.TargetCode = raw.TargetCode
	r.TargetVersion = raw.TargetVersion
	r.IsActive = raw.IsActive
	r.ExpiresAt = expiresAt
	r.QRCodeURL = raw.QRCodeURL
	return nil
}

// AssessmentEntryListResponse 测评入口列表响应（apiserver）。
type AssessmentEntryListResponse struct {
	Items      []*AssessmentEntryResponse `json:"items"`
	Total      int64                      `json:"total"`
	Page       int                        `json:"page"`
	PageSize   int                        `json:"page_size"`
	TotalPages int                        `json:"total_pages"`
}

// RelationResponse 从业者关系响应（apiserver）。
type RelationResponse struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	ClinicianID  string     `json:"clinician_id"`
	TesteeID     string     `json:"testee_id"`
	RelationType string     `json:"relation_type"`
	SourceType   string     `json:"source_type"`
	SourceID     *string    `json:"source_id,omitempty"`
	IsActive     bool       `json:"is_active"`
	BoundAt      time.Time  `json:"bound_at"`
	UnboundAt    *time.Time `json:"unbound_at,omitempty"`
}

func (r *RelationResponse) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID           string  `json:"id"`
		OrgID        string  `json:"org_id"`
		ClinicianID  string  `json:"clinician_id"`
		TesteeID     string  `json:"testee_id"`
		RelationType string  `json:"relation_type"`
		SourceType   string  `json:"source_type"`
		SourceID     *string `json:"source_id,omitempty"`
		IsActive     bool    `json:"is_active"`
		BoundAt      string  `json:"bound_at"`
		UnboundAt    *string `json:"unbound_at,omitempty"`
	}

	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	boundAt, err := parseFlexibleSeedRequiredTime(raw.BoundAt)
	if err != nil {
		return fmt.Errorf("parse bound_at: %w", err)
	}
	unboundAt, err := parseFlexibleSeedNullableTime(raw.UnboundAt)
	if err != nil {
		return fmt.Errorf("parse unbound_at: %w", err)
	}

	r.ID = raw.ID
	r.OrgID = raw.OrgID
	r.ClinicianID = raw.ClinicianID
	r.TesteeID = raw.TesteeID
	r.RelationType = raw.RelationType
	r.SourceType = raw.SourceType
	r.SourceID = raw.SourceID
	r.IsActive = raw.IsActive
	r.BoundAt = boundAt
	r.UnboundAt = unboundAt
	return nil
}

// TesteeClinicianRelationResponse 受试者-从业者关系响应（apiserver）。
type TesteeClinicianRelationResponse struct {
	Clinician *ClinicianResponse `json:"clinician"`
	Relation  *RelationResponse  `json:"relation"`
}

// TesteeClinicianRelationListResponse 受试者-从业者关系列表响应（apiserver）。
type TesteeClinicianRelationListResponse struct {
	Items []*TesteeClinicianRelationResponse `json:"items"`
}

// ClinicianRelationResponse 从业者关系详情响应（apiserver）。
type ClinicianRelationResponse struct {
	Testee   *ApiserverTesteeResponse `json:"testee"`
	Relation *RelationResponse        `json:"relation"`
}

// ClinicianRelationListResponse 从业者关系列表响应（apiserver）。
type ClinicianRelationListResponse struct {
	Items      []*ClinicianRelationResponse `json:"items"`
	Total      int64                        `json:"total"`
	Page       int                          `json:"page"`
	PageSize   int                          `json:"page_size"`
	TotalPages int                          `json:"total_pages"`
}

// CreateClinicianRequest 创建临床医师请求（apiserver）。
type CreateClinicianRequest struct {
	OrgID         int64   `json:"org_id"`
	OperatorID    *uint64 `json:"operator_id,omitempty"`
	Name          string  `json:"name"`
	Department    string  `json:"department,omitempty"`
	Title         string  `json:"title,omitempty"`
	ClinicianType string  `json:"clinician_type"`
	EmployeeCode  string  `json:"employee_code,omitempty"`
	IsActive      bool    `json:"is_active"`
}

// UpdateClinicianRequest 更新临床医师请求（apiserver）。
type UpdateClinicianRequest struct {
	Name          string `json:"name"`
	Department    string `json:"department,omitempty"`
	Title         string `json:"title,omitempty"`
	ClinicianType string `json:"clinician_type"`
	EmployeeCode  string `json:"employee_code,omitempty"`
}

// BindClinicianOperatorRequest 绑定临床医师与员工请求（apiserver）。
type BindClinicianOperatorRequest struct {
	OperatorID uint64 `json:"operator_id"`
}

// CreateAssessmentEntryRequest 创建测评入口请求（apiserver）。
type CreateAssessmentEntryRequest struct {
	TargetType    string     `json:"target_type"`
	TargetCode    string     `json:"target_code"`
	TargetVersion string     `json:"target_version,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

// IntakeAssessmentEntryRequest 公开入口 intake 请求（apiserver）。
type IntakeAssessmentEntryRequest struct {
	ProfileID *uint64    `json:"profile_id,omitempty"`
	Name      string     `json:"name"`
	Gender    string     `json:"gender,omitempty"`
	Birthday  *time.Time `json:"birthday,omitempty"`
}

// AssessmentEntryResolvedResponse 公开入口解析响应（apiserver）。
type AssessmentEntryResolvedResponse struct {
	Entry     *AssessmentEntryResponse `json:"entry"`
	Clinician *ClinicianResponse       `json:"clinician"`
}

// AssessmentEntryIntakeResponse 公开入口 intake 响应（apiserver）。
type AssessmentEntryIntakeResponse struct {
	Entry      *AssessmentEntryResponse `json:"entry"`
	Clinician  *ClinicianResponse       `json:"clinician"`
	Testee     *ApiserverTesteeResponse `json:"testee"`
	Relation   *RelationResponse        `json:"relation,omitempty"`
	Assignment *RelationResponse        `json:"assignment,omitempty"`
}

// IAMChildResponse IAM 儿童响应。
type IAMChildResponse struct {
	ID        string     `json:"id"`
	LegalName string     `json:"legalName"`
	Gender    *uint8     `json:"gender,omitempty"`
	DOB       string     `json:"dob,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

func (r *IAMChildResponse) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID        string  `json:"id"`
		LegalName string  `json:"legalName"`
		Gender    *uint8  `json:"gender,omitempty"`
		DOB       string  `json:"dob,omitempty"`
		CreatedAt *string `json:"createdAt,omitempty"`
		UpdatedAt *string `json:"updatedAt,omitempty"`
	}

	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	createdAt, err := parseFlexibleSeedNullableTime(raw.CreatedAt)
	if err != nil {
		return fmt.Errorf("parse createdAt: %w", err)
	}
	updatedAt, err := parseFlexibleSeedNullableTime(raw.UpdatedAt)
	if err != nil {
		return fmt.Errorf("parse updatedAt: %w", err)
	}

	r.ID = raw.ID
	r.LegalName = raw.LegalName
	r.Gender = raw.Gender
	r.DOB = raw.DOB
	r.CreatedAt = createdAt
	r.UpdatedAt = updatedAt
	return nil
}

func parseFlexibleSeedRequiredTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("time is empty")
	}
	return parseFlexibleSeedTime(value)
}

func parseFlexibleSeedOptionalTimeValue(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, nil
	}
	return parseFlexibleSeedTime(value)
}

func parseFlexibleSeedNullableTime(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := parseFlexibleSeedTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// IAMChildPageResponse IAM 当前用户儿童分页响应。
type IAMChildPageResponse struct {
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
	Items  []*IAMChildResponse `json:"items"`
}

// IAMChildRegisterRequest IAM 注册儿童请求。
type IAMChildRegisterRequest struct {
	LegalName string `json:"legalName"`
	Gender    uint8  `json:"gender"`
	DOB       string `json:"dob"`
	Relation  string `json:"relation"`
}

// IAMChildRegisterResponse IAM 注册儿童响应。
type IAMChildRegisterResponse struct {
	Child *IAMChildResponse `json:"child"`
}

// CollectionCreateTesteeRequest 创建 collection 受试者请求。
type CollectionCreateTesteeRequest struct {
	IAMUserID  string   `json:"iam_user_id,omitempty"`
	IAMChildID string   `json:"iam_child_id"`
	Name       string   `json:"name"`
	Gender     int32    `json:"gender"`
	Birthday   string   `json:"birthday,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Source     string   `json:"source,omitempty"`
	IsKeyFocus bool     `json:"is_key_focus,omitempty"`
}

// CollectionTesteeExistsResponse collection testee exists 响应。
type CollectionTesteeExistsResponse struct {
	Exists   bool   `json:"exists"`
	TesteeID string `json:"testee_id"`
}

// CollectionAssessmentDetailResponse collection 侧答卷对应测评详情。
type CollectionAssessmentDetailResponse struct {
	ID                   string `json:"id"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	Status               string `json:"status"`
}

// AssignClinicianTesteeRequest 建立从业者关系请求（apiserver）。
type AssignClinicianTesteeRequest struct {
	OrgID        int64   `json:"org_id"`
	ClinicianID  uint64  `json:"clinician_id"`
	TesteeID     uint64  `json:"testee_id"`
	RelationType string  `json:"relation_type,omitempty"`
	SourceType   string  `json:"source_type,omitempty"`
	SourceID     *uint64 `json:"source_id,omitempty"`
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
	Answers              []Answer `json:"answers"`
}

// AdminSubmitAnswerSheetRequest 管理员提交答卷请求（apiserver）
type AdminSubmitAnswerSheetRequest struct {
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	Title                string   `json:"title"`
	TesteeID             uint64   `json:"testee_id"`
	TaskID               string   `json:"task_id,omitempty"`
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

// AdminAnswerSheetListItem 管理端答卷列表项。
type AdminAnswerSheetListItem struct {
	ID                string `json:"id"`
	QuestionnaireCode string `json:"questionnaire_code"`
	Version           string `json:"questionnaire_version"`
	Title             string `json:"title"`
	WriterID          string `json:"writer_id"`
	TesteeID          string `json:"testee_id"`
}

// AdminAnswerSheetListResponse 管理端答卷列表响应。
type AdminAnswerSheetListResponse struct {
	Total int64                      `json:"total"`
	Items []AdminAnswerSheetListItem `json:"items"`
}

// doRequest 执行 HTTP 请求
func (c *APIClient) doRequest(ctx context.Context, method, path string, body interface{}) (*Response, error) {
	return c.doRequestWithRetryAndTimeout(ctx, method, path, body, true, c.httpClient.Timeout)
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
		return netErr.Timeout()
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

func urlQueryEscape(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}

func isAPIHTTPStatus(err error, status int) bool {
	if err == nil {
		return false
	}
	statusToken := fmt.Sprintf("http_status=%d", status)
	if strings.Contains(err.Error(), statusToken) {
		return true
	}
	return strings.Contains(err.Error(), fmt.Sprintf("status=%d", status))
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
		return nil, fmt.Errorf("decode plan response: %w", err)
	}
	return &planResp, nil
}

// GetTask 获取任务详情（apiserver）。
func (c *APIClient) GetTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/plans/tasks/%s", taskID), nil)
	if err != nil {
		return nil, err
	}

	var taskResp TaskResponse
	if err := decodeResponseData(resp, &taskResp); err != nil {
		return nil, fmt.Errorf("decode task response: %w", err)
	}
	return &taskResp, nil
}

// SchedulePendingTasks 调度待开放任务（apiserver internal）。
func (c *APIClient) SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error) {
	resp, err := c.doRequestWithRetryAndTimeout(ctx, "POST", "/internal/v1/plans/tasks/schedule", req, true, planScheduleRequestTimeout)
	if err != nil {
		return nil, err
	}

	var taskList TaskListResponse
	if err := decodeResponseData(resp, &taskList); err != nil {
		return nil, fmt.Errorf("decode scheduled task list response: %w", err)
	}
	return &taskList, nil
}

// ListPlanTaskWindow 查询任务窗口（apiserver internal）。
func (c *APIClient) ListPlanTaskWindow(ctx context.Context, req ListPlanTaskWindowRequest) (*PlanTaskWindowResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/internal/v1/plans/tasks/window", req)
	if err != nil {
		return nil, err
	}

	var windowResp PlanTaskWindowResponse
	if err := decodeResponseData(resp, &windowResp); err != nil {
		return nil, fmt.Errorf("decode task window response: %w", err)
	}
	return &windowResp, nil
}

// ExpireTask 过期任务（apiserver internal）。
func (c *APIClient) ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/internal/v1/plans/tasks/%s/expire", taskID), nil)
	if err != nil {
		return nil, err
	}

	var taskResp TaskResponse
	if err := decodeResponseData(resp, &taskResp); err != nil {
		return nil, fmt.Errorf("decode expired task response: %w", err)
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

// CreateCollectionTestee 创建 collection 受试者。
func (c *APIClient) CreateCollectionTestee(ctx context.Context, req CollectionCreateTesteeRequest) (*TesteeResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/testees", req)
	if err != nil {
		return nil, err
	}

	var testeeResp TesteeResponse
	if err := decodeResponseData(resp, &testeeResp); err != nil {
		return nil, fmt.Errorf("decode create testee response: %w", err)
	}
	return &testeeResp, nil
}

// TesteeExistsByIAMChildID 检查指定 IAM child 是否已经创建 collection testee。
func (c *APIClient) TesteeExistsByIAMChildID(ctx context.Context, iamChildID string) (*CollectionTesteeExistsResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/testees/exists?iam_child_id=%s", urlQueryEscape(strings.TrimSpace(iamChildID))), nil)
	if err != nil {
		return nil, err
	}

	var existsResp CollectionTesteeExistsResponse
	if err := decodeResponseData(resp, &existsResp); err != nil {
		return nil, fmt.Errorf("decode testee exists response: %w", err)
	}
	return &existsResp, nil
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
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/testees/%s", strings.TrimSpace(testeeID)), nil)
	if err != nil {
		return nil, fmt.Errorf("get testee %s: %w", testeeID, err)
	}

	var item ApiserverTesteeResponse
	if err := decodeResponseData(resp, &item); err != nil {
		return nil, fmt.Errorf("decode testee response: %w", err)
	}
	return &item, nil
}

// ListIAMMyChildren 获取当前 IAM 用户名下 children。
func (c *APIClient) ListIAMMyChildren(ctx context.Context, limit, offset int) (*IAMChildPageResponse, error) {
	path := fmt.Sprintf("/api/v1/identity/me/children?limit=%d&offset=%d", limit, offset)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var listResp IAMChildPageResponse
	if err := decodeResponseData(resp, &listResp); err != nil {
		return nil, fmt.Errorf("decode iam children response: %w", err)
	}
	return &listResp, nil
}

// RegisterIAMChild 注册当前 IAM 用户的 child。
func (c *APIClient) RegisterIAMChild(ctx context.Context, req IAMChildRegisterRequest) (*IAMChildRegisterResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/identity/children/register", req)
	if err != nil {
		return nil, err
	}

	var registerResp IAMChildRegisterResponse
	if err := decodeResponseData(resp, &registerResp); err != nil {
		return nil, fmt.Errorf("decode iam child register response: %w", err)
	}
	return &registerResp, nil
}

// ListStaff 获取员工列表（apiserver）。
func (c *APIClient) ListStaff(ctx context.Context, orgID int64, page, pageSize int) (*StaffListResponse, error) {
	path := fmt.Sprintf("/api/v1/staff?org_id=%d&page=%d&page_size=%d", orgID, page, pageSize)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list staff: org_id=%d page=%d page_size=%d: %w", orgID, page, pageSize, err)
	}

	var listResp StaffListResponse
	if err := decodeResponseData(resp, &listResp); err != nil {
		return nil, fmt.Errorf("decode staff list response: %w", err)
	}
	return &listResp, nil
}

// CreateStaff 创建员工（apiserver）。
func (c *APIClient) CreateStaff(ctx context.Context, req CreateStaffRequest) (*StaffResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/staff", req)
	if err != nil {
		return nil, err
	}

	var staffResp StaffResponse
	if err := decodeResponseData(resp, &staffResp); err != nil {
		return nil, fmt.Errorf("decode staff response: %w", err)
	}
	return &staffResp, nil
}

// UpdateStaff 更新员工（apiserver）。
func (c *APIClient) UpdateStaff(ctx context.Context, staffID string, req UpdateStaffRequest) (*StaffResponse, error) {
	resp, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/staff/%s", staffID), req)
	if err != nil {
		return nil, err
	}

	var staffResp StaffResponse
	if err := decodeResponseData(resp, &staffResp); err != nil {
		return nil, fmt.Errorf("decode updated staff response: %w", err)
	}
	return &staffResp, nil
}

// ListClinicians 获取临床医师列表（apiserver）。
func (c *APIClient) ListClinicians(ctx context.Context, orgID int64, page, pageSize int) (*ClinicianListResponse, error) {
	path := fmt.Sprintf("/api/v1/clinicians?org_id=%d&page=%d&page_size=%d", orgID, page, pageSize)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list clinicians: org_id=%d page=%d page_size=%d: %w", orgID, page, pageSize, err)
	}

	var listResp ClinicianListResponse
	if err := decodeResponseData(resp, &listResp); err != nil {
		return nil, fmt.Errorf("decode clinician list response: %w", err)
	}
	return &listResp, nil
}

// CreateClinician 创建临床医师（apiserver）。
func (c *APIClient) CreateClinician(ctx context.Context, req CreateClinicianRequest) (*ClinicianResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/clinicians", req)
	if err != nil {
		return nil, err
	}

	var clinicianResp ClinicianResponse
	if err := decodeResponseData(resp, &clinicianResp); err != nil {
		return nil, fmt.Errorf("decode clinician response: %w", err)
	}
	return &clinicianResp, nil
}

// UpdateClinician 更新临床医师（apiserver）。
func (c *APIClient) UpdateClinician(ctx context.Context, clinicianID string, req UpdateClinicianRequest) (*ClinicianResponse, error) {
	resp, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/v1/clinicians/%s", clinicianID), req)
	if err != nil {
		return nil, err
	}

	var clinicianResp ClinicianResponse
	if err := decodeResponseData(resp, &clinicianResp); err != nil {
		return nil, fmt.Errorf("decode updated clinician response: %w", err)
	}
	return &clinicianResp, nil
}

// ActivateClinician 激活临床医师（apiserver）。
func (c *APIClient) ActivateClinician(ctx context.Context, clinicianID string) (*ClinicianResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/clinicians/%s/activate", clinicianID), nil)
	if err != nil {
		return nil, err
	}

	var clinicianResp ClinicianResponse
	if err := decodeResponseData(resp, &clinicianResp); err != nil {
		return nil, fmt.Errorf("decode activated clinician response: %w", err)
	}
	return &clinicianResp, nil
}

// DeactivateClinician 停用临床医师（apiserver）。
func (c *APIClient) DeactivateClinician(ctx context.Context, clinicianID string) (*ClinicianResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/clinicians/%s/deactivate", clinicianID), nil)
	if err != nil {
		return nil, err
	}

	var clinicianResp ClinicianResponse
	if err := decodeResponseData(resp, &clinicianResp); err != nil {
		return nil, fmt.Errorf("decode deactivated clinician response: %w", err)
	}
	return &clinicianResp, nil
}

// BindClinicianOperator 绑定临床医师到员工（apiserver）。
func (c *APIClient) BindClinicianOperator(ctx context.Context, clinicianID string, operatorID uint64) (*ClinicianResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/clinicians/%s/bind-operator", clinicianID), BindClinicianOperatorRequest{
		OperatorID: operatorID,
	})
	if err != nil {
		return nil, err
	}

	var clinicianResp ClinicianResponse
	if err := decodeResponseData(resp, &clinicianResp); err != nil {
		return nil, fmt.Errorf("decode bound clinician response: %w", err)
	}
	return &clinicianResp, nil
}

// UnbindClinicianOperator 解绑临床医师与员工（apiserver）。
func (c *APIClient) UnbindClinicianOperator(ctx context.Context, clinicianID string) (*ClinicianResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/clinicians/%s/unbind-operator", clinicianID), nil)
	if err != nil {
		return nil, err
	}

	var clinicianResp ClinicianResponse
	if err := decodeResponseData(resp, &clinicianResp); err != nil {
		return nil, fmt.Errorf("decode unbound clinician response: %w", err)
	}
	return &clinicianResp, nil
}

// ListClinicianAssessmentEntries 获取临床医师测评入口列表（apiserver）。
func (c *APIClient) ListClinicianAssessmentEntries(ctx context.Context, clinicianID string, page, pageSize int) (*AssessmentEntryListResponse, error) {
	path := fmt.Sprintf("/api/v1/clinicians/%s/assessment-entries?page=%d&page_size=%d", clinicianID, page, pageSize)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list clinician assessment entries: clinician_id=%s page=%d page_size=%d: %w", clinicianID, page, pageSize, err)
	}

	var listResp AssessmentEntryListResponse
	if err := decodeResponseData(resp, &listResp); err != nil {
		return nil, fmt.Errorf("decode clinician assessment entry list response: %w", err)
	}
	return &listResp, nil
}

// CreateClinicianAssessmentEntry 创建临床医师测评入口（apiserver）。
func (c *APIClient) CreateClinicianAssessmentEntry(ctx context.Context, clinicianID string, req CreateAssessmentEntryRequest) (*AssessmentEntryResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/clinicians/%s/assessment-entries", clinicianID), req)
	if err != nil {
		return nil, err
	}

	var entryResp AssessmentEntryResponse
	if err := decodeResponseData(resp, &entryResp); err != nil {
		return nil, fmt.Errorf("decode clinician assessment entry response: %w", err)
	}
	return &entryResp, nil
}

// GetAssessmentEntry 获取测评入口详情（apiserver）。
func (c *APIClient) GetAssessmentEntry(ctx context.Context, entryID string) (*AssessmentEntryResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/assessment-entries/%s", strings.TrimSpace(entryID)), nil)
	if err != nil {
		return nil, err
	}

	var entryResp AssessmentEntryResponse
	if err := decodeResponseData(resp, &entryResp); err != nil {
		return nil, fmt.Errorf("decode assessment entry response: %w", err)
	}
	return &entryResp, nil
}

// ReactivateAssessmentEntry 重新激活测评入口（apiserver）。
func (c *APIClient) ReactivateAssessmentEntry(ctx context.Context, entryID string) (*AssessmentEntryResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/assessment-entries/%s/reactivate", strings.TrimSpace(entryID)), nil)
	if err != nil {
		return nil, err
	}

	var entryResp AssessmentEntryResponse
	if err := decodeResponseData(resp, &entryResp); err != nil {
		return nil, fmt.Errorf("decode reactivated assessment entry response: %w", err)
	}
	return &entryResp, nil
}

// ListClinicianRelations 获取临床医师当前有效的 testee 关系（apiserver）。
func (c *APIClient) ListClinicianRelations(ctx context.Context, clinicianID string, page, pageSize int) (*ClinicianRelationListResponse, error) {
	path := fmt.Sprintf("/api/v1/clinicians/%s/relations?page=%d&page_size=%d", clinicianID, page, pageSize)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list clinician relations: clinician_id=%s page=%d page_size=%d: %w", clinicianID, page, pageSize, err)
	}

	var listResp ClinicianRelationListResponse
	if err := decodeResponseData(resp, &listResp); err != nil {
		return nil, fmt.Errorf("decode clinician relation list response: %w", err)
	}
	return &listResp, nil
}

// ResolveAssessmentEntry 公开解析测评入口（apiserver）。
func (c *APIClient) ResolveAssessmentEntry(ctx context.Context, token string) (*AssessmentEntryResolvedResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/public/assessment-entries/%s", strings.TrimSpace(token)), nil)
	if err != nil {
		return nil, fmt.Errorf("resolve assessment entry token=%s: %w", token, err)
	}

	var result AssessmentEntryResolvedResponse
	if err := decodeResponseData(resp, &result); err != nil {
		return nil, fmt.Errorf("decode assessment entry resolve response: %w", err)
	}
	return &result, nil
}

// IntakeAssessmentEntry 公开扫码 intake（apiserver）。
func (c *APIClient) IntakeAssessmentEntry(ctx context.Context, token string, req IntakeAssessmentEntryRequest) (*AssessmentEntryIntakeResponse, error) {
	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/v1/public/assessment-entries/%s/intake", strings.TrimSpace(token)), req)
	if err != nil {
		return nil, fmt.Errorf("intake assessment entry token=%s: %w", token, err)
	}

	var result AssessmentEntryIntakeResponse
	if err := decodeResponseData(resp, &result); err != nil {
		return nil, fmt.Errorf("decode assessment entry intake response: %w", err)
	}
	return &result, nil
}

// GetTesteeClinicians 获取受试者当前有效的从业者关系（apiserver）。
func (c *APIClient) GetTesteeClinicians(ctx context.Context, testeeID string) (*TesteeClinicianRelationListResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/testees/%s/clinicians", testeeID), nil)
	if err != nil {
		return nil, err
	}

	var relationResp TesteeClinicianRelationListResponse
	if err := decodeResponseData(resp, &relationResp); err != nil {
		return nil, fmt.Errorf("decode testee clinician relations response: %w", err)
	}
	return &relationResp, nil
}

// AssignClinicianTesteeWithRelationType 按指定关系类型建立受试者分配（apiserver）。
func (c *APIClient) AssignClinicianTesteeWithRelationType(ctx context.Context, relationType string, req AssignClinicianTesteeRequest) (*RelationResponse, error) {
	path := "/api/v1/clinician-testee-relations/assign"
	switch strings.ToLower(strings.TrimSpace(relationType)) {
	case "primary":
		path = "/api/v1/clinician-testee-relations/assign-primary"
	case "collaborator":
		path = "/api/v1/clinician-testee-relations/assign-collaborator"
	case "attending", "", "assigned":
		path = "/api/v1/clinician-testee-relations/assign-attending"
	}

	resp, err := c.doRequest(ctx, "POST", path, req)
	if err != nil {
		return nil, err
	}

	var relationResp RelationResponse
	if err := decodeResponseData(resp, &relationResp); err != nil {
		return nil, fmt.Errorf("decode clinician-testee relation response: %w", err)
	}
	return &relationResp, nil
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

// ListAdminAnswerSheets 查询管理端答卷列表（apiserver）。
func (c *APIClient) ListAdminAnswerSheets(
	ctx context.Context,
	questionnaireCode string,
	fillerID uint64,
	page, pageSize int,
) (*AdminAnswerSheetListResponse, error) {
	path := fmt.Sprintf(
		"/api/v1/answersheets?page=%d&page_size=%d&questionnaire_code=%s&filler_id=%d",
		page,
		pageSize,
		urlQueryEscape(questionnaireCode),
		fillerID,
	)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var listResp AdminAnswerSheetListResponse
	if err := decodeResponseData(resp, &listResp); err != nil {
		return nil, fmt.Errorf("decode admin answersheet list response: %w", err)
	}
	return &listResp, nil
}

// GetAssessmentByAnswerSheetID 查询答卷对应的测评详情（collection-server）。
// 当测评尚未生成时返回 (nil, nil)。
func (c *APIClient) GetAssessmentByAnswerSheetID(ctx context.Context, answerSheetID string) (*CollectionAssessmentDetailResponse, error) {
	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/v1/answersheets/%s/assessment", strings.TrimSpace(answerSheetID)), nil)
	if err != nil {
		if isAPIHTTPStatus(err, http.StatusNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var detail CollectionAssessmentDetailResponse
	if err := decodeResponseData(resp, &detail); err != nil {
		return nil, fmt.Errorf("decode assessment-by-answersheet response: %w", err)
	}
	return &detail, nil
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

// AssessmentListResponse 测评列表响应（apiserver）。
type AssessmentListResponse struct {
	Items      []*AssessmentResponse `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalPages int                   `json:"total_pages"`
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

// ListAssessmentsByTestee 获取某个受试者的测评列表（apiserver）。
func (c *APIClient) ListAssessmentsByTestee(ctx context.Context, testeeID string, page, pageSize int) (*AssessmentListResponse, error) {
	path := fmt.Sprintf("/api/v1/evaluations/assessments?testee_id=%s&page=%d&page_size=%d", testeeID, page, pageSize)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("list assessments by testee: testee_id=%s page=%d page_size=%d: %w", testeeID, page, pageSize, err)
	}

	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}

	var listResp AssessmentListResponse
	if err := json.Unmarshal(dataBytes, &listResp); err != nil {
		return nil, fmt.Errorf("unmarshal assessment list response: %w", err)
	}
	return &listResp, nil
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
