package concurrent

import (
	"context"
	"fmt"
	"sync"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/domain/answersheet"
)

// Validator 并发验证器 - 专注于并发验证逻辑
type Validator struct {
	answersheetValidator *answersheet.Validator
	maxConcurrency       int
}

// NewValidator 创建并发验证器
func NewValidator(maxConcurrency int) *Validator {
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 默认并发数
	}

	return &Validator{
		answersheetValidator: answersheet.NewValidator(),
		maxConcurrency:       maxConcurrency,
	}
}

// ValidateAnswersConcurrently 并发验证答案列表
func (v *Validator) ValidateAnswersConcurrently(ctx context.Context, answers []*answersheet.Answer, questionnaireInfo answersheet.QuestionnaireInfo) error {
	log.L(ctx).Infof("Starting concurrent validation of %d answers (max concurrency: %d)", len(answers), v.maxConcurrency)

	// 创建问题映射
	questionMap := make(map[string]answersheet.QuestionInfo)
	for _, q := range questionnaireInfo.GetQuestions() {
		questionMap[q.GetCode()] = q
	}

	// 创建错误通道
	errorChan := make(chan error, len(answers))

	// 创建信号量控制并发数
	semaphore := make(chan struct{}, v.maxConcurrency)

	var wg sync.WaitGroup

	// 并发验证每个答案
	for _, answer := range answers {
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
			if err := v.answersheetValidator.ValidateAnswer(ctx, answer, question); err != nil {
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
		log.L(ctx).Errorf("Concurrent validation failed with %d errors, returning first error: %v", len(errors), errors[0])
		return errors[0]
	}

	log.L(ctx).Info("Concurrent validation completed successfully")
	return nil
}

// ValidateAnswersWithValidation 验证答案列表并使用额外的测试者信息验证
func (v *Validator) ValidateAnswersWithValidation(ctx context.Context, req *answersheet.SubmitRequest, questionnaireInfo answersheet.QuestionnaireInfo) error {
	// 首先验证测试者信息
	if err := v.answersheetValidator.ValidateTesteeInfo(req.TesteeInfo); err != nil {
		return fmt.Errorf("testee info validation failed: %w", err)
	}

	// 并发验证答案
	return v.ValidateAnswersConcurrently(ctx, req.Answers, questionnaireInfo)
}

// GetMaxConcurrency 获取最大并发数
func (v *Validator) GetMaxConcurrency() int {
	return v.maxConcurrency
}

// SetMaxConcurrency 设置最大并发数
func (v *Validator) SetMaxConcurrency(maxConcurrency int) {
	if maxConcurrency > 0 {
		v.maxConcurrency = maxConcurrency
	}
}

// GetConcurrencyStats 获取并发统计信息
func (v *Validator) GetConcurrencyStats() map[string]interface{} {
	return map[string]interface{}{
		"max_concurrency":  v.maxConcurrency,
		"validator_type":   "concurrent",
		"domain_validator": "answersheet",
	}
}
