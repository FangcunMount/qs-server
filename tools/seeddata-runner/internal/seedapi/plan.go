package seedapi

import (
	"context"
	"fmt"
)

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

func (c *APIClient) EnrollTesteeInPlan(ctx context.Context, req EnrollTesteeRequest) (*EnrollmentResponse, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/v1/plans/enroll", req)
	if err != nil {
		return nil, err
	}

	var enrollmentResp EnrollmentResponse
	if err := decodeResponseData(resp, &enrollmentResp); err != nil {
		return nil, fmt.Errorf("decode enroll testee response: %w", err)
	}
	return &enrollmentResp, nil
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
