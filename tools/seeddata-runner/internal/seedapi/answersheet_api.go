package seedapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SubmitAnswerSheet 提交答卷（collection-server）。
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

// SubmitAnswerSheetAdmin 管理员提交答卷（apiserver）。
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
func (c *APIClient) ListAdminAnswerSheets(ctx context.Context, questionnaireCode string, fillerID uint64, page, pageSize int) (*AdminAnswerSheetListResponse, error) {
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

// SubmitAnswerSheetAdminWithPolicy 管理员提交答卷并覆盖超时/重试。
func (c *APIClient) SubmitAnswerSheetAdminWithPolicy(ctx context.Context, req AdminSubmitAnswerSheetRequest, timeout time.Duration, retryMax int) (*SubmitAnswerSheetResponse, error) {
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
