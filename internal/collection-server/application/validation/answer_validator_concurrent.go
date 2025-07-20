package validation

import (
	"context"
	"fmt"
	"sync"

	questionnairepb "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/validation"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// AnswerValidatorConcurrent 并发答案验证器
type AnswerValidatorConcurrent struct {
	validator      *validation.Validator
	ruleFactory    ValidationRuleFactory
	maxConcurrency int
	workerPool     chan struct{}
}

// NewAnswerValidatorConcurrent 创建并发答案验证器
func NewAnswerValidatorConcurrent(ruleFactory ValidationRuleFactory, maxConcurrency int) *AnswerValidatorConcurrent {
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 默认并发数
	}

	return &AnswerValidatorConcurrent{
		validator:      validation.NewValidator(),
		ruleFactory:    ruleFactory,
		maxConcurrency: maxConcurrency,
		workerPool:     make(chan struct{}, maxConcurrency),
	}
}

// ValidateAnswers 并发验证答案列表
func (v *AnswerValidatorConcurrent) ValidateAnswers(ctx context.Context, answers []AnswerValidationItem, questionnaire *questionnairepb.Questionnaire) error {
	if len(answers) == 0 {
		return fmt.Errorf("answers cannot be empty")
	}

	log.L(ctx).Infof("Starting concurrent validation of %d answers with max concurrency: %d", len(answers), v.maxConcurrency)

	// 创建问题映射，方便查找
	questionMap := make(map[string]*questionnairepb.Question)
	for _, q := range questionnaire.Questions {
		questionMap[q.Code] = q
	}

	// 使用 WaitGroup 等待所有验证完成
	var wg sync.WaitGroup
	var validationErrors []ValidationError

	// 创建错误通道
	errorChan := make(chan ValidationError, len(answers))

	// 启动验证工作协程
	for i, answer := range answers {
		wg.Add(1)
		go func(index int, answerItem AnswerValidationItem) {
			defer wg.Done()

			// 获取工作协程槽位
			v.workerPool <- struct{}{}
			defer func() { <-v.workerPool }()

			// 验证单个答案
			if err := v.validateSingleAnswerConcurrent(ctx, answerItem, questionMap); err != nil {
				validationError := ValidationError{
					Index:  index,
					Error:  err,
					Answer: answerItem,
				}
				errorChan <- validationError
			}
		}(i, answer)
	}

	// 等待所有验证完成
	wg.Wait()
	close(errorChan)

	// 收集所有错误
	for err := range errorChan {
		validationErrors = append(validationErrors, err)
	}

	// 如果有错误，返回第一个错误
	if len(validationErrors) > 0 {
		firstError := validationErrors[0]
		return fmt.Errorf("invalid answer at index %d (question %s): %w",
			firstError.Index, firstError.Answer.QuestionID, firstError.Error)
	}

	log.L(ctx).Info("All answers validated successfully")
	return nil
}

// validateSingleAnswerConcurrent 并发验证单个答案
func (v *AnswerValidatorConcurrent) validateSingleAnswerConcurrent(ctx context.Context, answer AnswerValidationItem, questionMap map[string]*questionnairepb.Question) error {
	// 查找对应的问题
	question, exists := questionMap[answer.QuestionID]
	if !exists {
		return fmt.Errorf("question not found: %s", answer.QuestionID)
	}

	// 使用工厂生成验证规则
	rules := v.ruleFactory.CreateValidationRules(question)

	// 使用验证器校验答案
	errors := v.validator.ValidateMultiple(answer.Value, rules)
	if len(errors) > 0 {
		// 返回第一个错误
		return fmt.Errorf("validation failed: %s", errors[0].Error())
	}

	return nil
}

// ValidationError 验证错误信息
type ValidationError struct {
	Index  int
	Error  error
	Answer AnswerValidationItem
}

// GetMaxConcurrency 获取最大并发数
func (v *AnswerValidatorConcurrent) GetMaxConcurrency() int {
	return v.maxConcurrency
}

// SetMaxConcurrency 设置最大并发数
func (v *AnswerValidatorConcurrent) SetMaxConcurrency(maxConcurrency int) {
	if maxConcurrency > 0 {
		v.maxConcurrency = maxConcurrency
		// 重新创建工作协程池
		v.workerPool = make(chan struct{}, maxConcurrency)
	}
}
