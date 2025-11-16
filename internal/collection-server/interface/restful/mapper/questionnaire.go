package mapper

import (
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/response"
)

// QuestionnaireMapper 问卷映射器，负责不同层级间的数据转换
type QuestionnaireMapper struct{}

// NewQuestionnaireMapper 创建新的问卷映射器
func NewQuestionnaireMapper() *QuestionnaireMapper {
	return &QuestionnaireMapper{}
}

// ToGRPCListRequest 将HTTP列表请求转换为gRPC请求
func (m *QuestionnaireMapper) ToGRPCListRequest(req *request.QuestionnaireListRequest) *questionnaire.ListQuestionnairesRequest {
	return &questionnaire.ListQuestionnairesRequest{
		Page:     int32(req.Page),
		PageSize: int32(req.PageSize),
		Status:   req.Status,
		// 注意：proto中可能没有Keyword字段，这里先保留Status
	}
}

// ToQuestionnaireListResponse 将gRPC问卷列表转换为HTTP响应
func (m *QuestionnaireMapper) ToQuestionnaireListResponse(
	grpcResp *questionnaire.ListQuestionnairesResponse,
	req *request.QuestionnaireListRequest,
) *response.QuestionnaireListResponse {
	// 转换问卷列表
	questionnaires := make([]response.QuestionnaireItem, len(grpcResp.Questionnaires))
	for i, q := range grpcResp.Questionnaires {
		questionnaires[i] = m.ToQuestionnaireItem(q)
	}

	// 计算总页数
	totalPages := int((grpcResp.Total + int64(req.PageSize) - 1) / int64(req.PageSize))

	return &response.QuestionnaireListResponse{
		Total:          grpcResp.Total,
		Page:           req.Page,
		PageSize:       req.PageSize,
		TotalPages:     totalPages,
		Questionnaires: questionnaires,
	}
}

// ToQuestionnaireItem 将gRPC问卷数据转换为HTTP列表项
func (m *QuestionnaireMapper) ToQuestionnaireItem(q *questionnaire.Questionnaire) response.QuestionnaireItem {
	// 解析时间
	createdAt, _ := time.Parse(time.RFC3339, q.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, q.UpdatedAt)

	// proto中没有published_at字段，设为nil
	var publishedAt *time.Time = nil

	// 解析版本号
	version := 1 // 默认版本
	if v, err := strconv.Atoi(q.Version); err == nil {
		version = v
	}

	return response.QuestionnaireItem{
		Code:        q.Code,
		Title:       q.Title,
		Description: q.Description,
		Category:    "", // proto中没有category字段
		Status:      q.Status,
		Version:     version,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		PublishedAt: publishedAt,
		SubmitCount: 0, // proto中没有submit_count字段
		ViewCount:   0, // proto中没有view_count字段
	}
}

// ToQuestionnaireResponse 将gRPC问卷详情转换为HTTP响应
func (m *QuestionnaireMapper) ToQuestionnaireResponse(q *questionnaire.Questionnaire) *response.QuestionnaireResponse {
	// 转换问题列表
	questions := make([]response.Question, len(q.Questions))
	for i, question := range q.Questions {
		questions[i] = m.ToQuestion(question)
	}

	// 解析时间
	createdAt, _ := time.Parse(time.RFC3339, q.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, q.UpdatedAt)

	// proto中没有published_at字段，设为nil
	var publishedAt *time.Time = nil

	// 解析版本号
	version := 1 // 默认版本
	if v, err := strconv.Atoi(q.Version); err == nil {
		version = v
	}

	return &response.QuestionnaireResponse{
		Code:        q.Code,
		Title:       q.Title,
		Description: q.Description,
		Category:    "", // proto中没有category字段
		Status:      q.Status,
		Version:     version,
		Questions:   questions,
		Settings:    m.ToSettings(q),
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		PublishedAt: publishedAt,
		SubmitCount: 0, // proto中没有submit_count字段
		ViewCount:   0, // proto中没有view_count字段
	}
}

// ToQuestion 将gRPC问题转换为HTTP问题
func (m *QuestionnaireMapper) ToQuestion(q *questionnaire.Question) response.Question {
	// 转换选项
	options := make([]response.QuestionOption, len(q.Options))
	for i, option := range q.Options {
		options[i] = response.QuestionOption{
			Code:   option.Code,
			Text:   option.Content,
			Value:  option.Code, // 使用code作为value
			Order:  i + 1,       // proto中没有order字段，使用索引
			Points: int(option.Score),
		}
	}

	// 转换验证规则
	validationRules := make([]response.ValidationRule, len(q.ValidationRules))
	for i, rule := range q.ValidationRules {
		validationRules[i] = response.ValidationRule{
			RuleType:    rule.RuleType,
			RuleValue:   rule.TargetValue,
			ErrorMsg:    "", // proto中没有message字段
			TargetValue: rule.TargetValue,
		}
	}

	return response.Question{
		Code:             q.Code,
		Type:             q.Type,
		Title:            q.Title,
		Description:      q.Tips, // 使用Tips作为description
		Required:         false,  // proto中没有required字段，默认false
		Options:          options,
		ValidationRules:  validationRules,
		DisplayOrder:     0,                            // proto中没有order字段
		Group:            "",                           // proto中没有group字段
		ConditionalLogic: make(map[string]interface{}), // 暂时为空
	}
}

// ToSettings 转换问卷设置
func (m *QuestionnaireMapper) ToSettings(q *questionnaire.Questionnaire) response.Settings {
	var timeLimit *int = nil // 默认无时间限制

	return response.Settings{
		AllowMultipleSubmissions: false, // 默认不允许多次提交
		RequireLogin:             false, // 默认不需要登录
		ShowProgressBar:          true,  // 默认显示进度条
		RandomizeQuestions:       false, // 默认不随机化问题
		TimeLimit:                timeLimit,
		SubmissionMessage:        "问卷提交成功！",                    // 默认提交消息
		RedirectURL:              "",                           // 默认无重定向
		CustomCSS:                "",                           // 默认无自定义CSS
		ExtraSettings:            make(map[string]interface{}), // 空的额外设置
	}
}

// ValidateAndGetCode 验证并提取问卷代码
func (m *QuestionnaireMapper) ValidateAndGetCode(codeStr string) (string, error) {
	if codeStr == "" {
		return "", fmt.Errorf("questionnaire code cannot be empty")
	}
	return codeStr, nil
}

// ToGetQuestionnaireRequest 将HTTP请求转换为应用服务请求
func (m *QuestionnaireMapper) ToGetQuestionnaireRequest(code string) *questionnaireapp.GetQuestionnaireRequest {
	return &questionnaireapp.GetQuestionnaireRequest{
		Code: code,
	}
}

// ToValidateCodeRequest 转换为验证代码请求
func (m *QuestionnaireMapper) ToValidateCodeRequest(req *request.QuestionnaireValidateCodeRequest) string {
	return req.Code
}
