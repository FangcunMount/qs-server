package seedapi

import (
	"context"
	"fmt"
	"strings"
)

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
