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
