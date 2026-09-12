package answersheet

import (
	"bytes"
	"context"
	"encoding/json"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math"
	"strconv"
)

// StartAnsweringRequest intentionally excludes company, user, store and clock.
type StartAnsweringRequest struct {
	RequestKey           string     `json:"request_key" binding:"required"`
	TesteeID             string     `json:"testee_id" binding:"required"`
	QuestionnaireCode    string     `json:"questionnaire_code" binding:"required"`
	QuestionnaireVersion string     `json:"questionnaire_version" binding:"required"`
	ModelCode            string     `json:"model_code,omitempty"`
	ModelVersion         string     `json:"model_version,omitempty"`
	OriginRef            *OriginRef `json:"origin_ref,omitempty"`
}

func (r *StartAnsweringRequest) UnmarshalJSON(data []byte) error {
	type plain StartAnsweringRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*plain)(r))
}

type StartAnsweringInput struct {
	Request                 StartAnsweringRequest
	UserID, TesteeID, OrgID uint64
}
type StartAnsweringOutput struct {
	ID        string `json:"id"`
	StartedAt string `json:"started_at"`
	Created   bool   `json:"-"`
}
type AnsweringStartWriter interface {
	StartAnswering(context.Context, *StartAnsweringInput) (*StartAnsweringOutput, error)
}

func (s *SubmissionService) StartAnswering(ctx context.Context, userID uint64, req *StartAnsweringRequest) (*StartAnsweringOutput, error) {
	if req == nil || !safeIdempotencyKey.MatchString(req.RequestKey) || req.QuestionnaireCode == "" || req.QuestionnaireVersion == "" {
		return nil, status.Error(codes.InvalidArgument, "exact content and safe request key required")
	}
	id, err := strconv.ParseUint(req.TesteeID, 10, 64)
	if err != nil || id == 0 || id > math.MaxInt64 {
		return nil, status.Error(codes.InvalidArgument, "invalid testee_id")
	}
	if err := s.validateWriter(ctx, userID); err != nil {
		return nil, err
	}
	writer, ok := s.answerSheetWriter.(AnsweringStartWriter)
	if !ok {
		return nil, status.Error(codes.Unavailable, "answering start unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, s.acceptTimeout)
	defer cancel()
	subject, canonical, err := s.profileAccess.Resolve(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	result, err := writer.StartAnswering(ctx, &StartAnsweringInput{Request: *req, UserID: userID, TesteeID: canonical, OrgID: subject.OrgID})
	if err != nil {
		return nil, err
	}
	if result == nil || result.ID == "" {
		return nil, status.Error(codes.Unavailable, "answering start returned no record")
	}
	return result, nil
}
