package binding

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Policy 拥有身份特定的问卷约束
type Policy interface {
	// Supports 支持
	Supports(domain.Identity) bool
	// Validate 验证
	Validate(context.Context, *domain.AssessmentModel, domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error)
	// BeforePublish 发布前
	BeforePublish(context.Context, *domain.AssessmentModel) error
}

// Policies 策略
type Policies struct {
	policies []Policy
}

// NewPolicies 创建策略
func NewPolicies(policies ...Policy) Policies {
	result := Policies{policies: make([]Policy, 0, len(policies))}
	for _, policy := range policies {
		if policy != nil {
			result.policies = append(result.policies, policy)
		}
	}
	return result
}

// Validate 验证
func (r Policies) Validate(ctx context.Context, model *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	policy := r.resolve(model)
	if policy == nil {
		if model == nil {
			return binding, nil
		}
		return domain.QuestionnaireBinding{}, fmt.Errorf("questionnaire binding policy is not registered for kind %s", model.Kind)
	}
	return policy.Validate(ctx, model, binding)
}

// BeforePublish 发布前
func (r Policies) BeforePublish(ctx context.Context, model *domain.AssessmentModel) error {
	policy := r.resolve(model)
	if policy == nil {
		if model == nil {
			return nil
		}
		return fmt.Errorf("questionnaire binding policy is not registered for kind %s", model.Kind)
	}
	return policy.BeforePublish(ctx, model)
}

// resolve 解析策略
func (r Policies) resolve(model *domain.AssessmentModel) Policy {
	if model == nil {
		return nil
	}
	identity := domain.Identity{Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm}
	for _, policy := range r.policies {
		if policy.Supports(identity) {
			return policy
		}
	}
	return nil
}

// PolicyFunc 策略函数
type PolicyFunc struct {
	Match             func(domain.Identity) bool
	ValidateFunc      func(context.Context, *domain.AssessmentModel, domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error)
	BeforePublishFunc func(context.Context, *domain.AssessmentModel) error
}

// Supports 支持
func (f PolicyFunc) Supports(identity domain.Identity) bool {
	return f.Match != nil && f.Match(identity)
}

// Validate 验证
func (f PolicyFunc) Validate(ctx context.Context, model *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	if f.ValidateFunc == nil {
		return binding, nil
	}
	return f.ValidateFunc(ctx, model, binding)
}

// BeforePublish 发布前
func (f PolicyFunc) BeforePublish(ctx context.Context, model *domain.AssessmentModel) error {
	if f.BeforePublishFunc == nil {
		return nil
	}
	return f.BeforePublishFunc(ctx, model)
}
