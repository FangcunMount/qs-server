package seedapi

import (
	"context"
	"encoding/json"
	"fmt"
)

// GetScale 获取量表详情。
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

// GetQuestionnaireDetail 获取问卷详情（collection-server）。
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
