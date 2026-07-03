package acl

import (
	"context"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
)

// AnswerSheetBFFReader 将 infra gRPC 输出转换为 answersheet application DTO。
type AnswerSheetBFFReader struct {
	inner grpcbridge.AnswerSheetWriter
}

// NewAnswerSheetBFFReader 构造答卷读 ACL 适配器。
func NewAnswerSheetBFFReader(inner grpcbridge.AnswerSheetWriter) *AnswerSheetBFFReader {
	return &AnswerSheetBFFReader{inner: inner}
}

// AnswerSheetBFFWriter 将 answersheet application DTO 转换为下游 gRPC DTO。
type AnswerSheetBFFWriter struct {
	inner grpcbridge.AnswerSheetWriter
}

// NewAnswerSheetBFFWriter 构造答卷写 ACL 适配器。
func NewAnswerSheetBFFWriter(inner grpcbridge.AnswerSheetWriter) *AnswerSheetBFFWriter {
	return &AnswerSheetBFFWriter{inner: inner}
}

func (w *AnswerSheetBFFWriter) SaveAnswerSheet(ctx context.Context, input *answersheet.SaveAnswerSheetInput) (*answersheet.SaveAnswerSheetOutput, error) {
	if w == nil || w.inner == nil {
		return nil, nil
	}
	result, err := w.inner.SaveAnswerSheet(ctx, toGRPCSaveAnswerSheetInput(input))
	if err != nil || result == nil {
		return nil, err
	}
	return &answersheet.SaveAnswerSheetOutput{
		ID:      result.ID,
		Message: result.Message,
	}, nil
}

func toGRPCSaveAnswerSheetInput(input *answersheet.SaveAnswerSheetInput) *grpcbridge.SaveAnswerSheetInput {
	if input == nil {
		return nil
	}
	answers := make([]grpcbridge.AnswerInput, len(input.Answers))
	for i, answer := range input.Answers {
		answers[i] = grpcbridge.AnswerInput{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Score:        answer.Score,
			Value:        answer.Value,
		}
	}
	return &grpcbridge.SaveAnswerSheetInput{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
		IdempotencyKey:       input.IdempotencyKey,
		Title:                input.Title,
		WriterID:             input.WriterID,
		TesteeID:             input.TesteeID,
		TaskID:               input.TaskID,
		OrgID:                input.OrgID,
		Answers:              answers,
	}
}

func (r *AnswerSheetBFFReader) GetAnswerSheet(ctx context.Context, id uint64) (*answersheet.AnswerSheetResponse, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	result, err := r.inner.GetAnswerSheet(ctx, id)
	if err != nil || result == nil {
		return nil, err
	}
	answers := make([]answersheet.Answer, len(result.Answers))
	for i, a := range result.Answers {
		answers[i] = answersheet.Answer{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Score:        a.Score,
			Value:        a.Value,
		}
	}
	return &answersheet.AnswerSheetResponse{
		ID:                   strconv.FormatUint(result.ID, 10),
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		Title:                result.Title,
		Score:                result.Score,
		WriterID:             strconv.FormatUint(result.WriterID, 10),
		WriterName:           result.WriterName,
		TesteeID:             strconv.FormatUint(result.TesteeID, 10),
		TesteeName:           result.TesteeName,
		Answers:              answers,
		CreatedAt:            result.CreatedAt,
		UpdatedAt:            result.UpdatedAt,
	}, nil
}

// TesteeActorLookup 将 ActorReader 适配为 answersheet.ActorLookup。
type TesteeActorLookup struct {
	inner grpcbridge.ActorReader
}

// NewTesteeActorLookup 构造受试者查询 ACL 适配器。
func NewTesteeActorLookup(inner grpcbridge.ActorReader) *TesteeActorLookup {
	return &TesteeActorLookup{inner: inner}
}

func (a *TesteeActorLookup) GetTestee(ctx context.Context, testeeID uint64) (*answersheet.ActorTestee, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	out, err := a.inner.GetTestee(ctx, testeeID)
	if err != nil || out == nil {
		return nil, err
	}
	return &answersheet.ActorTestee{
		OrgID:        out.OrgID,
		IAMProfileID: out.IAMProfileID,
		Name:         out.Name,
	}, nil
}

func (a *TesteeActorLookup) TesteeExists(ctx context.Context, orgID, iamProfileID uint64) (bool, uint64, error) {
	if a == nil || a.inner == nil {
		return false, 0, nil
	}
	return a.inner.TesteeExists(ctx, orgID, iamProfileID)
}
