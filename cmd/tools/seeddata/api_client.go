package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
)

// APIClient HTTP API 客户端
type APIClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     log.Logger
}

// NewAPIClient 创建 API 客户端
func NewAPIClient(baseURL, token string, logger log.Logger) *APIClient {
	// 确保 baseURL 不以斜杠结尾
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &APIClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
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
	Code    string `json:"code"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Version string `json:"version"`
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

// doRequest 执行 HTTP 请求
func (c *APIClient) doRequest(ctx context.Context, method, path string, body interface{}) (*Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// 如果状态码不是 200，先尝试解析 JSON（可能是 API 的错误响应）
	if resp.StatusCode != http.StatusOK {
		var apiResp Response
		if err := json.Unmarshal(respBody, &apiResp); err == nil {
			// 成功解析为 JSON，返回 API 错误信息
			// 特殊处理 401 错误
			if resp.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("authentication failed (401): please check your API token. message=%s", apiResp.Message)
			}
			return nil, fmt.Errorf("api error: http_status=%d, code=%d, message=%s", resp.StatusCode, apiResp.Code, apiResp.Message)
		}
		// 无法解析为 JSON（可能是 HTML 错误页面），返回原始响应
		bodyStr := string(respBody)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		// 特殊处理 401 错误
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("authentication failed (401): please check your API token. url=%s", url)
		}
		return nil, fmt.Errorf("http error: status=%d, url=%s, body=%s", resp.StatusCode, url, bodyStr)
	}

	// 状态码是 200，尝试解析 JSON
	var apiResp Response
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		bodyStr := string(respBody)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		return nil, fmt.Errorf("unmarshal response: %w, url=%s, body=%s", err, url, bodyStr)
	}

	// 检查 API 业务错误码
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("api error: code=%d, message=%s, http_status=%d", apiResp.Code, apiResp.Message, resp.StatusCode)
	}

	return &apiResp, nil
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

	return &sResp, nil
}
