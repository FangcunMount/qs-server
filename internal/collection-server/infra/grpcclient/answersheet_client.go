package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
)

// ==================== Input/Output Types ====================

// SaveAnswerSheetInput 保存答卷输入
type SaveAnswerSheetInput struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
	Title                string
	WriterID             uint64
	TesteeID             uint64
	OrgID                uint64
	Answers              []AnswerInput
}

// AnswerInput 答案输入
type AnswerInput struct {
	QuestionCode string
	QuestionType string
	Score        uint32
	Value        string
}

// SaveAnswerSheetOutput 保存答卷输出
type SaveAnswerSheetOutput struct {
	ID      uint64
	Message string
}

// AnswerSheetOutput 答卷输出
type AnswerSheetOutput struct {
	ID                   uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	Title                string
	Score                float64
	WriterID             uint64
	WriterName           string
	TesteeID             uint64
	TesteeName           string
	Answers              []AnswerOutput
	CreatedAt            string
	UpdatedAt            string
}

// AnswerOutput 答案输出
type AnswerOutput struct {
	QuestionCode string
	QuestionType string
	Score        uint32
	Value        string
}

// ==================== Client ====================

// AnswerSheetClient 答卷服务 gRPC 客户端封装
type AnswerSheetClient struct {
	client     *Client
	grpcClient pb.AnswerSheetServiceClient
}

// NewAnswerSheetClient 创建答卷服务客户端
func NewAnswerSheetClient(client *Client) *AnswerSheetClient {
	return &AnswerSheetClient{
		client:     client,
		grpcClient: pb.NewAnswerSheetServiceClient(client.Conn()),
	}
}

// SaveAnswerSheet 保存答卷
func (c *AnswerSheetClient) SaveAnswerSheet(ctx context.Context, input *SaveAnswerSheetInput) (*SaveAnswerSheetOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	// 转换为 protobuf 请求
	answers := make([]*pb.Answer, len(input.Answers))
	for i, a := range input.Answers {
		answers[i] = &pb.Answer{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Score:        a.Score,
			Value:        a.Value,
		}
	}

	req := &pb.SaveAnswerSheetRequest{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
		Title:                input.Title,
		WriterId:             input.WriterID,
		TesteeId:             input.TesteeID,
		OrgId:                input.OrgID,
		Answers:              answers,
	}

	resp, err := c.grpcClient.SaveAnswerSheet(ctx, req)
	if err != nil {
		return nil, err
	}

	return &SaveAnswerSheetOutput{
		ID:      resp.GetId(),
		Message: resp.GetMessage(),
	}, nil
}

// GetAnswerSheet 获取答卷详情
func (c *AnswerSheetClient) GetAnswerSheet(ctx context.Context, id uint64) (*AnswerSheetOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetAnswerSheetRequest{Id: id}
	resp, err := c.grpcClient.GetAnswerSheet(ctx, req)
	if err != nil {
		return nil, err
	}

	sheet := resp.GetAnswerSheet()
	if sheet == nil {
		return nil, nil
	}

	// 转换 answers
	answers := make([]AnswerOutput, len(sheet.GetAnswers()))
	for i, a := range sheet.GetAnswers() {
		answers[i] = AnswerOutput{
			QuestionCode: a.GetQuestionCode(),
			QuestionType: a.GetQuestionType(),
			Score:        a.GetScore(),
			Value:        a.GetValue(),
		}
	}

	return &AnswerSheetOutput{
		ID:                   sheet.GetId(),
		QuestionnaireCode:    sheet.GetQuestionnaireCode(),
		QuestionnaireVersion: sheet.GetQuestionnaireVersion(),
		Title:                sheet.GetTitle(),
		Score:                sheet.GetScore(),
		WriterID:             sheet.GetWriterId(),
		WriterName:           sheet.GetWriterName(),
		TesteeID:             sheet.GetTesteeId(),
		TesteeName:           sheet.GetTesteeName(),
		Answers:              answers,
		CreatedAt:            sheet.GetCreatedAt(),
		UpdatedAt:            sheet.GetUpdatedAt(),
	}, nil
}

// ListAnswerSheets 获取答卷列表 (保留原始 protobuf 接口以供复杂查询使用)
func (c *AnswerSheetClient) ListAnswerSheets(ctx context.Context, req *pb.ListAnswerSheetsRequest) (*pb.ListAnswerSheetsResponse, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	return c.grpcClient.ListAnswerSheets(ctx, req)
}
