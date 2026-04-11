package main

import (
	"context"
	"time"
)

type adminAnswerSheetSubmitPolicy struct {
	Timeout      time.Duration
	HTTPRetryMax int
	MaxAttempts  int
	RetryBackoff time.Duration
	Retryable    func(error) bool
}

type adminAnswerSheetSubmitClient interface {
	SubmitAnswerSheetAdmin(context.Context, AdminSubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error)
	SubmitAnswerSheetAdminWithPolicy(context.Context, AdminSubmitAnswerSheetRequest, time.Duration, int) (*SubmitAnswerSheetResponse, error)
}

func buildAdminSubmitAnswerSheetRequest(req SubmitAnswerSheetRequest) AdminSubmitAnswerSheetRequest {
	return AdminSubmitAnswerSheetRequest{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		TesteeID:             req.TesteeID,
		TaskID:               req.TaskID,
		Answers:              req.Answers,
	}
}

func submitAdminAnswerSheet(
	ctx context.Context,
	client adminAnswerSheetSubmitClient,
	req SubmitAnswerSheetRequest,
	policy adminAnswerSheetSubmitPolicy,
) (int, error) {
	maxAttempts := policy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error
	attempts := 0
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return attempts, ctx.Err()
		}

		attempts++
		if err := submitAdminAnswerSheetOnce(ctx, client, req, policy); err == nil {
			return attempts, nil
		} else {
			lastErr = err
		}

		if attempt == maxAttempts-1 {
			break
		}
		if policy.Retryable != nil && !policy.Retryable(lastErr) {
			break
		}
		if policy.RetryBackoff <= 0 {
			continue
		}
		if err := sleepWithContext(ctx, policy.RetryBackoff*time.Duration(attempt+1)); err != nil {
			return attempts, err
		}
	}

	return attempts, lastErr
}

func submitAdminAnswerSheetOnce(
	ctx context.Context,
	client adminAnswerSheetSubmitClient,
	req SubmitAnswerSheetRequest,
	policy adminAnswerSheetSubmitPolicy,
) error {
	adminReq := buildAdminSubmitAnswerSheetRequest(req)
	if policy.Timeout > 0 || policy.HTTPRetryMax != 0 {
		_, err := client.SubmitAnswerSheetAdminWithPolicy(ctx, adminReq, policy.Timeout, policy.HTTPRetryMax)
		return err
	}
	_, err := client.SubmitAnswerSheetAdmin(ctx, adminReq)
	return err
}
