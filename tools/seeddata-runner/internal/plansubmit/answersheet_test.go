package plansubmit

import (
	"context"
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
