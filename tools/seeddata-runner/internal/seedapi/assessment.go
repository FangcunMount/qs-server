package seedapi

import (
	"context"
	"encoding/json"
	"fmt"
)

// AssessmentListResponse 测评列表响应（apiserver）。
type AssessmentListResponse struct {
	Items      []*AssessmentResponse `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalPages int                   `json:"total_pages"`
}

// AssessmentResponse 测评响应（apiserver）。
type AssessmentResponse struct {
	ID                string   `json:"id"`
	TesteeID          string   `json:"testee_id"`
	QuestionnaireCode string   `json:"questionnaire_code"`
	Status            string   `json:"status"`
	TotalScore        *float64 `json:"total_score,omitempty"`
	RiskLevel         *string  `json:"risk_level,omitempty"`
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
