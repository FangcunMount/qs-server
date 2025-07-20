package validation

import (
	"context"
	"fmt"
	"strconv"

	questionnairepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/validation"
	grpcclient "github.com/yshujie/questionnaire-scale/internal/collection-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Service 校验服务接口
type Service interface {
	// ValidateAnswersheet 校验答卷
	ValidateAnswersheet(ctx context.Context, answersheet *AnswersheetValidationRequest) error
	// ValidateQuestionnaireCode 校验问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// service 校验服务实现
type service struct {
	questionnaireClient grpcclient.QuestionnaireClient
	validator           *validation.Validator
}

// NewService 创建新的校验服务
func NewService(questionnaireClient grpcclient.QuestionnaireClient) Service {
	return &service{
		questionnaireClient: questionnaireClient,
		validator:           validation.NewValidator(),
	}
}

// AnswersheetValidationRequest 答卷校验请求
type AnswersheetValidationRequest struct {
	QuestionnaireCode string                 `json:"questionnaire_code"`
	Answers           []AnswerValidationItem `json:"answers"`
	TesteeInfo        TesteeInfo             `json:"testee_info"`
}

// AnswerValidationItem 答案校验项
type AnswerValidationItem struct {
	QuestionID   string      `json:"question_id"`
	QuestionType string      `json:"question_type"`
	Value        interface{} `json:"value"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Gender string `json:"gender"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
}

// ValidateAnswersheet 校验答卷
func (s *service) ValidateAnswersheet(ctx context.Context, req *AnswersheetValidationRequest) error {
	log.L(ctx).Infof("Validating answersheet for questionnaire: %s", req.QuestionnaireCode)

	// 1. 校验问卷代码
	if err := s.ValidateQuestionnaireCode(ctx, req.QuestionnaireCode); err != nil {
		return fmt.Errorf("invalid questionnaire code: %w", err)
	}

	// 2. 获取问卷详情
	questionnaire, err := s.questionnaireClient.GetQuestionnaire(ctx, req.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("failed to get questionnaire: %w", err)
	}

	// 3. 根据问卷配置校验答案
	if err := s.validateAnswersWithQuestionnaire(ctx, req.Answers, questionnaire.Questionnaire); err != nil {
		return fmt.Errorf("invalid answers: %w", err)
	}

	log.L(ctx).Info("Answersheet validation passed")
	return nil
}

// ValidateQuestionnaireCode 校验问卷代码
func (s *service) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	if code == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}

	// 校验代码长度
	if len(code) < 3 || len(code) > 50 {
		return fmt.Errorf("questionnaire code length must be between 3 and 50 characters")
	}

	return nil
}

// validateAnswersWithQuestionnaire 根据问卷配置校验答案
func (s *service) validateAnswersWithQuestionnaire(ctx context.Context, answers []AnswerValidationItem, questionnaire *questionnairepb.Questionnaire) error {
	if len(answers) == 0 {
		return fmt.Errorf("answers cannot be empty")
	}

	// 创建问题映射，方便查找
	questionMap := make(map[string]*questionnairepb.Question)
	for _, q := range questionnaire.Questions {
		questionMap[q.Code] = q
	}

	// 校验每个答案
	for i, answer := range answers {
		// 查找对应的问题
		question, exists := questionMap[answer.QuestionID]
		if !exists {
			return fmt.Errorf("question not found: %s", answer.QuestionID)
		}

		// 根据问题配置生成验证规则并校验
		if err := s.validateAnswerWithQuestion(ctx, answer, question); err != nil {
			return fmt.Errorf("invalid answer at index %d (question %s): %w", i, answer.QuestionID, err)
		}
	}

	return nil
}

// validateAnswerWithQuestion 根据问题配置校验单个答案
func (s *service) validateAnswerWithQuestion(ctx context.Context, answer AnswerValidationItem, question *questionnairepb.Question) error {
	// 生成验证规则
	rules := s.generateValidationRules(question)

	// 使用验证器校验答案
	errors := s.validator.ValidateMultiple(answer.Value, rules)
	if len(errors) > 0 {
		// 返回第一个错误
		return fmt.Errorf("validation failed: %s", errors[0].Error())
	}

	return nil
}

// generateValidationRules 根据问题配置生成验证规则
func (s *service) generateValidationRules(question *questionnairepb.Question) []*validation.ValidationRule {
	var rules []*validation.ValidationRule

	// 处理问卷中配置的验证规则
	for _, protoRule := range question.ValidationRules {
		rule := s.convertProtoValidationRule(protoRule, question)
		if rule != nil {
			rules = append(rules, rule)
		}
	}

	return rules
}

// convertProtoValidationRule 转换 protobuf 验证规则为领域验证规则
func (s *service) convertProtoValidationRule(protoRule *questionnairepb.ValidationRule, question *questionnairepb.Question) *validation.ValidationRule {
	switch protoRule.RuleType {
	case "required":
		return validation.Required("此题为必答题")

	case "min_length":
		if length, err := strconv.Atoi(protoRule.TargetValue); err == nil {
			return validation.MinLength(length, fmt.Sprintf("答案长度不能少于%d个字符", length))
		}

	case "max_length":
		if length, err := strconv.Atoi(protoRule.TargetValue); err == nil {
			return validation.MaxLength(length, fmt.Sprintf("答案长度不能超过%d个字符", length))
		}

	case "min_value":
		if value, err := strconv.ParseFloat(protoRule.TargetValue, 64); err == nil {
			return validation.MinValue(value, fmt.Sprintf("答案不能小于%v", value))
		}

	case "max_value":
		if value, err := strconv.ParseFloat(protoRule.TargetValue, 64); err == nil {
			return validation.MaxValue(value, fmt.Sprintf("答案不能大于%v", value))
		}

	case "min_selections":
		if count, err := strconv.Atoi(protoRule.TargetValue); err == nil {
			// 对于多选题，验证最少选择数量
			return validation.MinValue(float64(count), fmt.Sprintf("至少需要选择%d个选项", count))
		}

	case "max_selections":
		if count, err := strconv.Atoi(protoRule.TargetValue); err == nil {
			// 对于多选题，验证最多选择数量
			return validation.MaxValue(float64(count), fmt.Sprintf("最多只能选择%d个选项", count))
		}
	}

	return nil
}
