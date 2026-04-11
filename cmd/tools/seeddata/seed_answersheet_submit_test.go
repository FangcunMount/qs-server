package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type adminAnswerSheetSubmitClientStub struct {
	adminCalls      int
	withPolicyCalls int
	lastAdminReq    AdminSubmitAnswerSheetRequest
	lastPolicyReq   AdminSubmitAnswerSheetRequest
	lastTimeout     time.Duration
	lastRetryMax    int
	adminErrs       []error
	policyErrs      []error
}

func (s *adminAnswerSheetSubmitClientStub) SubmitAnswerSheetAdmin(ctx context.Context, req AdminSubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
	s.adminCalls++
	s.lastAdminReq = req
	if len(s.adminErrs) == 0 {
		return &SubmitAnswerSheetResponse{}, nil
	}
	err := s.adminErrs[0]
	s.adminErrs = s.adminErrs[1:]
	return nil, err
}

func (s *adminAnswerSheetSubmitClientStub) SubmitAnswerSheetAdminWithPolicy(ctx context.Context, req AdminSubmitAnswerSheetRequest, timeout time.Duration, retryMax int) (*SubmitAnswerSheetResponse, error) {
	s.withPolicyCalls++
	s.lastPolicyReq = req
	s.lastTimeout = timeout
	s.lastRetryMax = retryMax
	if len(s.policyErrs) == 0 {
		return &SubmitAnswerSheetResponse{}, nil
	}
	err := s.policyErrs[0]
	s.policyErrs = s.policyErrs[1:]
	return nil, err
}

func TestBuildAdminSubmitAnswerSheetRequest(t *testing.T) {
	req := SubmitAnswerSheetRequest{
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		Title:                "测试问卷",
		TesteeID:             1001,
		TaskID:               "2001",
		Answers: []Answer{
			{QuestionCode: "Q1", QuestionType: questionTypeRadio, Value: "A"},
		},
	}

	adminReq := buildAdminSubmitAnswerSheetRequest(req)
	if adminReq.QuestionnaireCode != req.QuestionnaireCode ||
		adminReq.QuestionnaireVersion != req.QuestionnaireVersion ||
		adminReq.Title != req.Title ||
		adminReq.TesteeID != req.TesteeID ||
		adminReq.TaskID != req.TaskID ||
		len(adminReq.Answers) != 1 {
		t.Fatalf("unexpected admin submit request: %+v", adminReq)
	}
}

func TestSubmitAdminAnswerSheetUsesPlainAdminAPIWithoutPolicy(t *testing.T) {
	client := &adminAnswerSheetSubmitClientStub{}
	_, err := submitAdminAnswerSheet(context.Background(), client, SubmitAnswerSheetRequest{}, adminAnswerSheetSubmitPolicy{
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("unexpected submit error: %v", err)
	}
	if client.adminCalls != 1 || client.withPolicyCalls != 0 {
		t.Fatalf("unexpected submit call distribution: admin=%d policy=%d", client.adminCalls, client.withPolicyCalls)
	}
}

func TestSubmitAdminAnswerSheetRetriesOnlyWhenPolicyAllows(t *testing.T) {
	retryableErr := errors.New("http_status=504")
	client := &adminAnswerSheetSubmitClientStub{
		policyErrs: []error{retryableErr, nil},
	}

	attempts, err := submitAdminAnswerSheet(context.Background(), client, SubmitAnswerSheetRequest{}, adminAnswerSheetSubmitPolicy{
		Timeout:      15 * time.Second,
		HTTPRetryMax: 2,
		MaxAttempts:  3,
		RetryBackoff: 0,
		Retryable: func(err error) bool {
			return err != nil && err.Error() == retryableErr.Error()
		},
	})
	if err != nil {
		t.Fatalf("unexpected submit error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if client.adminCalls != 0 || client.withPolicyCalls != 2 {
		t.Fatalf("unexpected submit call distribution: admin=%d policy=%d", client.adminCalls, client.withPolicyCalls)
	}
	if client.lastTimeout != 15*time.Second || client.lastRetryMax != 2 {
		t.Fatalf("unexpected policy call settings: timeout=%s retry_max=%d", client.lastTimeout, client.lastRetryMax)
	}
}

func TestSubmitAdminAnswerSheetStopsOnNonRetryableError(t *testing.T) {
	client := &adminAnswerSheetSubmitClientStub{
		policyErrs: []error{errors.New("http_status=400")},
	}

	attempts, err := submitAdminAnswerSheet(context.Background(), client, SubmitAnswerSheetRequest{}, adminAnswerSheetSubmitPolicy{
		Timeout:      15 * time.Second,
		HTTPRetryMax: 0,
		MaxAttempts:  3,
		RetryBackoff: 0,
		Retryable: func(err error) bool {
			return false
		},
	})
	if err == nil {
		t.Fatal("expected submit error")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
	if client.withPolicyCalls != 1 {
		t.Fatalf("expected 1 policy submit call, got %d", client.withPolicyCalls)
	}
}
