package dailysim

import (
	"context"
	"time"

	toolanswersheet "github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/answersheet"
)

type adminAnswerSheetSubmitPolicy = toolanswersheet.SubmitPolicy

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
	internalReq := toToolSubmitRequest(req)
	return toolanswersheet.SubmitWithRetry(ctx, internalReq, policy, func(
		ctx context.Context,
		submitReq toolanswersheet.SubmitRequest,
		timeout time.Duration,
		retryMax int,
	) error {
		adminReq := buildAdminSubmitAnswerSheetRequest(fromToolSubmitRequest(submitReq))
		if timeout > 0 || retryMax != 0 {
			_, err := client.SubmitAnswerSheetAdminWithPolicy(ctx, adminReq, timeout, retryMax)
			return err
		}
		_, err := client.SubmitAnswerSheetAdmin(ctx, adminReq)
		return err
	})
}
