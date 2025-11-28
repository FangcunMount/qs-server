package assessment

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// ==================== AssessmentCreator 领域服务 ====================

// AssessmentCreator 测评创建服务接口（领域服务）
// 负责从答卷提交创建完整的测评记录
//
// 设计说明：
// - 这是一个领域服务，因为测评创建需要协调多个聚合根和领域概念
// - 封装了"跨聚合验证 → 测评创建 → 状态迁移"的完整流程
//
// 职责：
// 1. 跨聚合验证（受试者、问卷、答卷、量表）
// 2. 创建测评聚合根
// 3. 提交测评（状态迁移）
// 4. 返回待持久化的测评对象
type AssessmentCreator interface {
	// Create 创建测评
	// 这是最常用的入口，对应"用户提交答卷"场景
	Create(ctx context.Context, req CreateAssessmentRequest) (*Assessment, error)
}

// ==================== 创建请求 ====================

// CreateAssessmentRequest 创建测评请求
type CreateAssessmentRequest struct {
	// 必填字段
	OrgID            int64
	TesteeID         testee.ID
	QuestionnaireRef QuestionnaireRef
	AnswerSheetRef   AnswerSheetRef

	// 来源信息
	Origin Origin

	// 可选字段
	MedicalScaleRef *MedicalScaleRef

	// 是否自动提交（默认 true）
	// 设为 false 可以创建 pending 状态的测评，便于测试或特殊场景
	AutoSubmit bool
}

// NewCreateAssessmentRequest 创建请求构造函数
func NewCreateAssessmentRequest(
	orgID int64,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	origin Origin,
) CreateAssessmentRequest {
	return CreateAssessmentRequest{
		OrgID:            orgID,
		TesteeID:         testeeID,
		QuestionnaireRef: questionnaireRef,
		AnswerSheetRef:   answerSheetRef,
		Origin:           origin,
		AutoSubmit:       true, // 默认自动提交
	}
}

// WithMedicalScale 设置关联量表
func (r CreateAssessmentRequest) WithMedicalScale(ref MedicalScaleRef) CreateAssessmentRequest {
	r.MedicalScaleRef = &ref
	return r
}

// WithoutAutoSubmit 不自动提交
func (r CreateAssessmentRequest) WithoutAutoSubmit() CreateAssessmentRequest {
	r.AutoSubmit = false
	return r
}

// ==================== 依赖接口定义 ====================

// TesteeValidator 受试者验证器接口
// 用于验证受试者是否存在且有效
type TesteeValidator interface {
	// Exists 检查受试者是否存在
	Exists(ctx context.Context, testeeID testee.ID) (bool, error)
}

// QuestionnaireValidator 问卷验证器接口
// 用于验证问卷是否存在且有效
type QuestionnaireValidator interface {
	// Exists 检查问卷是否存在
	Exists(ctx context.Context, ref QuestionnaireRef) (bool, error)

	// IsPublished 检查问卷是否已发布
	IsPublished(ctx context.Context, ref QuestionnaireRef) (bool, error)
}

// AnswerSheetValidator 答卷验证器接口
// 用于验证答卷是否存在且属于指定问卷
type AnswerSheetValidator interface {
	// Exists 检查答卷是否存在
	Exists(ctx context.Context, ref AnswerSheetRef) (bool, error)

	// BelongsToQuestionnaire 检查答卷是否属于指定问卷
	BelongsToQuestionnaire(ctx context.Context, answerSheetRef AnswerSheetRef, questionnaireRef QuestionnaireRef) (bool, error)
}

// ScaleValidator 量表验证器接口
// 用于验证量表是否存在且与问卷关联
type ScaleValidator interface {
	// Exists 检查量表是否存在
	Exists(ctx context.Context, ref MedicalScaleRef) (bool, error)

	// IsLinkedToQuestionnaire 检查量表是否与问卷关联
	IsLinkedToQuestionnaire(ctx context.Context, scaleRef MedicalScaleRef, questionnaireRef QuestionnaireRef) (bool, error)
}

// ==================== DefaultAssessmentCreator 默认实现 ====================

// DefaultAssessmentCreator 默认测评创建服务
// 包含完整的跨聚合验证逻辑
type DefaultAssessmentCreator struct {
	testeeValidator        TesteeValidator
	questionnaireValidator QuestionnaireValidator
	answerSheetValidator   AnswerSheetValidator
	scaleValidator         ScaleValidator
}

// AssessmentCreatorOption 创建器配置选项
type AssessmentCreatorOption func(*DefaultAssessmentCreator)

// WithTesteeValidator 设置受试者验证器
func WithTesteeValidator(v TesteeValidator) AssessmentCreatorOption {
	return func(c *DefaultAssessmentCreator) {
		c.testeeValidator = v
	}
}

// WithQuestionnaireValidator 设置问卷验证器
func WithQuestionnaireValidator(v QuestionnaireValidator) AssessmentCreatorOption {
	return func(c *DefaultAssessmentCreator) {
		c.questionnaireValidator = v
	}
}

// WithAnswerSheetValidator 设置答卷验证器
func WithAnswerSheetValidator(v AnswerSheetValidator) AssessmentCreatorOption {
	return func(c *DefaultAssessmentCreator) {
		c.answerSheetValidator = v
	}
}

// WithScaleValidator 设置量表验证器
func WithScaleValidator(v ScaleValidator) AssessmentCreatorOption {
	return func(c *DefaultAssessmentCreator) {
		c.scaleValidator = v
	}
}

// NewDefaultAssessmentCreator 创建默认测评创建服务
func NewDefaultAssessmentCreator(opts ...AssessmentCreatorOption) *DefaultAssessmentCreator {
	c := &DefaultAssessmentCreator{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Create 创建测评
func (c *DefaultAssessmentCreator) Create(
	ctx context.Context,
	req CreateAssessmentRequest,
) (*Assessment, error) {
	// 1. 执行跨聚合验证
	if err := c.validate(ctx, req); err != nil {
		return nil, err
	}

	// 2. 验证来源参数
	if err := c.validateOrigin(req.Origin); err != nil {
		return nil, err
	}

	// 3. 创建测评
	opts := make([]AssessmentOption, 0)
	if req.MedicalScaleRef != nil {
		opts = append(opts, WithMedicalScale(*req.MedicalScaleRef))
	}

	assessment, err := NewAssessment(
		req.OrgID,
		req.TesteeID,
		req.QuestionnaireRef,
		req.AnswerSheetRef,
		req.Origin,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	// 4. 自动提交（如果启用）
	if req.AutoSubmit {
		if err := assessment.Submit(); err != nil {
			return nil, err
		}
	}

	return assessment, nil
}

// validate 执行跨聚合验证
func (c *DefaultAssessmentCreator) validate(
	ctx context.Context,
	req CreateAssessmentRequest,
) error {
	// 1. 验证受试者
	if c.testeeValidator != nil {
		exists, err := c.testeeValidator.Exists(ctx, req.TesteeID)
		if err != nil {
			return fmt.Errorf("failed to validate testee: %w", err)
		}
		if !exists {
			return ErrTesteeNotFound
		}
	}

	// 2. 验证问卷
	if c.questionnaireValidator != nil {
		exists, err := c.questionnaireValidator.Exists(ctx, req.QuestionnaireRef)
		if err != nil {
			return fmt.Errorf("failed to validate questionnaire: %w", err)
		}
		if !exists {
			return ErrQuestionnaireNotFound
		}

		published, err := c.questionnaireValidator.IsPublished(ctx, req.QuestionnaireRef)
		if err != nil {
			return fmt.Errorf("failed to check questionnaire status: %w", err)
		}
		if !published {
			return ErrQuestionnaireNotPublished
		}
	}

	// 3. 验证答卷
	if c.answerSheetValidator != nil {
		exists, err := c.answerSheetValidator.Exists(ctx, req.AnswerSheetRef)
		if err != nil {
			return fmt.Errorf("failed to validate answer sheet: %w", err)
		}
		if !exists {
			return ErrAnswerSheetNotFound
		}

		belongs, err := c.answerSheetValidator.BelongsToQuestionnaire(ctx, req.AnswerSheetRef, req.QuestionnaireRef)
		if err != nil {
			return fmt.Errorf("failed to check answer sheet ownership: %w", err)
		}
		if !belongs {
			return ErrAnswerSheetMismatch
		}
	}

	// 4. 验证量表（如果指定了量表）
	if req.MedicalScaleRef != nil && c.scaleValidator != nil {
		exists, err := c.scaleValidator.Exists(ctx, *req.MedicalScaleRef)
		if err != nil {
			return fmt.Errorf("failed to validate medical scale: %w", err)
		}
		if !exists {
			return ErrScaleNotFound
		}

		linked, err := c.scaleValidator.IsLinkedToQuestionnaire(ctx, *req.MedicalScaleRef, req.QuestionnaireRef)
		if err != nil {
			return fmt.Errorf("failed to check scale-questionnaire link: %w", err)
		}
		if !linked {
			return ErrScaleNotLinked
		}
	}

	return nil
}

// validateOrigin 验证来源参数
func (c *DefaultAssessmentCreator) validateOrigin(origin Origin) error {
	switch origin.Type() {
	case OriginAdhoc:
		// adhoc 不需要额外参数
		return nil
	case OriginPlan:
		if origin.ID() == nil || *origin.ID() == "" {
			return ErrInvalidArgument
		}
		return nil
	case OriginScreening:
		if origin.ID() == nil || *origin.ID() == "" {
			return ErrInvalidArgument
		}
		return nil
	default:
		return ErrInvalidArgument
	}
}

// ==================== SimpleAssessmentCreator 简单实现 ====================

// SimpleAssessmentCreator 简单测评创建服务
// 不进行跨聚合验证，适用于测试环境或已在应用层完成验证的场景
type SimpleAssessmentCreator struct{}

// NewSimpleAssessmentCreator 创建简单测评创建服务
func NewSimpleAssessmentCreator() *SimpleAssessmentCreator {
	return &SimpleAssessmentCreator{}
}

// Create 创建测评（简单实现，不做跨聚合验证）
func (c *SimpleAssessmentCreator) Create(
	_ context.Context,
	req CreateAssessmentRequest,
) (*Assessment, error) {
	// 1. 构造选项
	opts := make([]AssessmentOption, 0)
	if req.MedicalScaleRef != nil {
		opts = append(opts, WithMedicalScale(*req.MedicalScaleRef))
	}

	// 2. 创建测评
	assessment, err := NewAssessment(
		req.OrgID,
		req.TesteeID,
		req.QuestionnaireRef,
		req.AnswerSheetRef,
		req.Origin,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	// 3. 自动提交（如果启用）
	if req.AutoSubmit {
		if err := assessment.Submit(); err != nil {
			return nil, err
		}
	}

	return assessment, nil
}

// ==================== 确保实现接口 ====================

var (
	_ AssessmentCreator = (*DefaultAssessmentCreator)(nil)
	_ AssessmentCreator = (*SimpleAssessmentCreator)(nil)
)
