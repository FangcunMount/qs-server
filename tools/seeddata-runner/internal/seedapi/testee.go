package seedapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

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

// ListTesteesByOrg 获取受试者列表（apiserver）。
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
