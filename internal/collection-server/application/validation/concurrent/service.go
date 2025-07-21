package concurrent

import (
	"context"
	"fmt"
	"sync"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Service 并发验证服务接口
type Service interface {
	// ValidateAnswersheet 验证答卷
	ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error

	// ValidateQuestionnaireCode 验证问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// ValidationRequest 验证请求
type ValidationRequest struct {
	QuestionnaireCode string                  `json:"questionnaire_code" validate:"required"`
	Title             string                  `json:"title" validate:"required"`
	TesteeInfo        *TesteeInfo             `json:"testee_info" validate:"required"`
	Answers           []*AnswerValidationItem `json:"answers" validate:"required"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// AnswerValidationItem 答案验证项
type AnswerValidationItem struct {
	QuestionCode string      `json:"question_code" validate:"required"`
	QuestionType string      `json:"question_type" validate:"required"`
	Value        interface{} `json:"value" validate:"required"`
}

// service 并发验证服务实现
type service struct {
	questionnaireService questionnaire.Service
	answersheetValidator *answersheet.Validator
	maxConcurrency       int
}

// NewService 创建并发验证服务
func NewService(questionnaireService questionnaire.Service, maxConcurrency int) Service {
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 默认并发数
	}

	return &service{
		questionnaireService: questionnaireService,
		answersheetValidator: answersheet.NewValidator(),
		maxConcurrency:       maxConcurrency,
	}
}

// ValidateAnswersheet 验证答卷
func (s *service) ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error {
	if req == nil {
		return fmt.Errorf("validation request cannot be nil")
	}

	log.L(ctx).Infof("Validating answersheet for questionnaire: %s (concurrent, max concurrency: %d)", req.QuestionnaireCode, s.maxConcurrency)

	// 1. 验证问卷代码
	if err := s.ValidateQuestionnaireCode(ctx, req.QuestionnaireCode); err != nil {
		return fmt.Errorf("questionnaire code validation failed: %w", err)
	}

	// 2. 获取问卷信息用于验证
	questionnaireInfo, err := s.questionnaireService.GetQuestionnaireForValidation(ctx, req.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("failed to get questionnaire for validation: %w", err)
	}

	// 3. 转换为领域实体
	answersheetEntity := s.convertToAnswersheet(req)

	// 4. 并发验证答案
	if err := s.validateAnswersConcurrently(ctx, answersheetEntity, questionnaireInfo); err != nil {
		return fmt.Errorf("concurrent answersheet validation failed: %w", err)
	}

	log.L(ctx).Info("Concurrent validation completed successfully")
	return nil
}

// ValidateQuestionnaireCode 验证问卷代码
func (s *service) ValidateQuestionnaireCode(ctx context.Context, code string) error {
	return s.questionnaireService.ValidateQuestionnaireCode(ctx, code)
}

// validateAnswersConcurrently 并发验证答案
func (s *service) validateAnswersConcurrently(ctx context.Context, answersheetEntity *answersheet.SubmitRequest, questionnaireInfo answersheet.QuestionnaireInfo) error {
	// 创建问题映射
	questionMap := make(map[string]answersheet.QuestionInfo)
	for _, q := range questionnaireInfo.GetQuestions() {
		questionMap[q.GetCode()] = q
	}

	// 创建错误通道
	errorChan := make(chan error, len(answersheetEntity.Answers))

	// 创建信号量控制并发数
	semaphore := make(chan struct{}, s.maxConcurrency)

	var wg sync.WaitGroup

	// 并发验证每个答案
	for _, answer := range answersheetEntity.Answers {
		wg.Add(1)
		go func(answer *answersheet.Answer) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 查找对应的问题
			question, exists := questionMap[answer.QuestionCode]
			if !exists {
				errorChan <- fmt.Errorf("question not found: %s", answer.QuestionCode)
				return
			}

			// 验证答案
			if err := s.answersheetValidator.ValidateAnswer(ctx, answer, question); err != nil {
				errorChan <- fmt.Errorf("invalid answer for question %s: %w", answer.QuestionCode, err)
				return
			}
		}(answer)
	}

	// 等待所有验证完成
	wg.Wait()
	close(errorChan)

	// 收集错误
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	// 如果有错误，返回第一个错误
	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// convertToAnswersheet 将请求转换为答卷实体
func (s *service) convertToAnswersheet(req *ValidationRequest) *answersheet.SubmitRequest {
	answers := make([]*answersheet.Answer, 0, len(req.Answers))
	for _, answer := range req.Answers {
		answers = append(answers, &answersheet.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        answer.Value,
		})
	}

	return &answersheet.SubmitRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		TesteeInfo: &answersheet.TesteeInfo{
			Name:  req.TesteeInfo.Name,
			Email: req.TesteeInfo.Email,
			Phone: req.TesteeInfo.Phone,
		},
		Answers: answers,
	}
}
