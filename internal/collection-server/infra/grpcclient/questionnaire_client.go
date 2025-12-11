package grpcclient

import (
	"context"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
)

// ==================== Output Types ====================

// QuestionnaireOutput 问卷输出
type QuestionnaireOutput struct {
	Code        string
	Title       string
	Description string
	ImgURL      string
	Status      string
	Version     string
	Questions   []QuestionOutput
	CreatedAt   string
	UpdatedAt   string
}

// QuestionOutput 问题输出
type QuestionOutput struct {
	Code            string
	Type            string
	Title           string
	Tips            string
	Placeholder     string
	Options         []OptionOutput
	ValidationRules []ValidationRuleOutput
	CalculationRule *CalculationRuleOutput
}

// OptionOutput 选项输出
type OptionOutput struct {
	Code    string
	Content string
	Score   int32
}

// ValidationRuleOutput 验证规则输出
type ValidationRuleOutput struct {
	RuleType    string
	TargetValue string
}

// CalculationRuleOutput 计算规则输出
type CalculationRuleOutput struct {
	FormulaType string
}

// ListQuestionnairesOutput 问卷列表输出
type ListQuestionnairesOutput struct {
	Questionnaires []QuestionnaireSummaryOutput
	Total          int64
	Page           int32
	PageSize       int32
}

// QuestionnaireSummaryOutput 问卷摘要输出（不含问题详情）
type QuestionnaireSummaryOutput struct {
	Code          string
	Title         string
	Description   string
	ImgURL        string
	Status        string
	Version       string
	QuestionCount int32
	CreatedAt     string
	UpdatedAt     string
}

// ==================== Client ====================

// QuestionnaireClient 问卷服务 gRPC 客户端封装
type QuestionnaireClient struct {
	client     *Client
	grpcClient pb.QuestionnaireServiceClient
}

// NewQuestionnaireClient 创建问卷服务客户端
func NewQuestionnaireClient(client *Client) *QuestionnaireClient {
	return &QuestionnaireClient{
		client:     client,
		grpcClient: pb.NewQuestionnaireServiceClient(client.Conn()),
	}
}

// GetQuestionnaire 获取问卷详情
func (c *QuestionnaireClient) GetQuestionnaire(ctx context.Context, code string) (*QuestionnaireOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.GetQuestionnaireRequest{Code: code}
	resp, err := c.grpcClient.GetQuestionnaire(ctx, req)
	if err != nil {
		return nil, err
	}

	q := resp.GetQuestionnaire()
	if q == nil {
		return nil, nil
	}

	return c.convertQuestionnaire(q), nil
}

// ListQuestionnaires 获取问卷列表（摘要）
func (c *QuestionnaireClient) ListQuestionnaires(ctx context.Context, page, pageSize int32, status, title string) (*ListQuestionnairesOutput, error) {
	ctx, cancel := c.client.ContextWithTimeout(ctx)
	defer cancel()

	req := &pb.ListQuestionnairesRequest{
		Page:     page,
		PageSize: pageSize,
		Status:   status,
		Title:    title,
	}

	resp, err := c.grpcClient.ListQuestionnaires(ctx, req)
	if err != nil {
		return nil, err
	}

	// 使用轻量级摘要类型，不含问题详情
	questionnaires := make([]QuestionnaireSummaryOutput, len(resp.GetQuestionnaires()))
	for i, q := range resp.GetQuestionnaires() {
		questionnaires[i] = QuestionnaireSummaryOutput{
			Code:          q.GetCode(),
			Title:         q.GetTitle(),
			Description:   q.GetDescription(),
			ImgURL:        q.GetImgUrl(),
			Status:        q.GetStatus(),
			Version:       q.GetVersion(),
			QuestionCount: q.GetQuestionCount(),
			CreatedAt:     q.GetCreatedAt(),
			UpdatedAt:     q.GetUpdatedAt(),
		}
	}

	return &ListQuestionnairesOutput{
		Questionnaires: questionnaires,
		Total:          resp.GetTotal(),
		Page:           page,
		PageSize:       pageSize,
	}, nil
}

// convertQuestionnaire 转换 protobuf 问卷到输出类型
func (c *QuestionnaireClient) convertQuestionnaire(q *pb.Questionnaire) *QuestionnaireOutput {
	questions := make([]QuestionOutput, len(q.GetQuestions()))
	for i, question := range q.GetQuestions() {
		questions[i] = c.convertQuestion(question)
	}

	return &QuestionnaireOutput{
		Code:        q.GetCode(),
		Title:       q.GetTitle(),
		Description: q.GetDescription(),
		ImgURL:      q.GetImgUrl(),
		Status:      q.GetStatus(),
		Version:     q.GetVersion(),
		Questions:   questions,
		CreatedAt:   q.GetCreatedAt(),
		UpdatedAt:   q.GetUpdatedAt(),
	}
}

// convertQuestion 转换 protobuf 问题到输出类型
func (c *QuestionnaireClient) convertQuestion(q *pb.Question) QuestionOutput {
	options := make([]OptionOutput, len(q.GetOptions()))
	for i, opt := range q.GetOptions() {
		options[i] = OptionOutput{
			Code:    opt.GetCode(),
			Content: opt.GetContent(),
			Score:   opt.GetScore(),
		}
	}

	validationRules := make([]ValidationRuleOutput, len(q.GetValidationRules()))
	for i, rule := range q.GetValidationRules() {
		validationRules[i] = ValidationRuleOutput{
			RuleType:    rule.GetRuleType(),
			TargetValue: rule.GetTargetValue(),
		}
	}

	var calcRule *CalculationRuleOutput
	if q.GetCalculationRule() != nil {
		calcRule = &CalculationRuleOutput{
			FormulaType: q.GetCalculationRule().GetFormulaType(),
		}
	}

	return QuestionOutput{
		Code:            q.GetCode(),
		Type:            q.GetType(),
		Title:           q.GetTitle(),
		Tips:            q.GetTips(),
		Placeholder:     q.GetPlaceholder(),
		Options:         options,
		ValidationRules: validationRules,
		CalculationRule: calcRule,
	}
}
