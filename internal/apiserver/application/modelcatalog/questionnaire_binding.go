package modelcatalog

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// QuestionnaireBindingPolicy owns identity-specific questionnaire constraints.
// It is intentionally separate from the catalogue command flow so no command
// service needs a family switch.
type QuestionnaireBindingPolicy interface {
	Supports(domain.Identity) bool
	Validate(context.Context, *domain.AssessmentModel, domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error)
	BeforePublish(context.Context, *domain.AssessmentModel) error
}

type QuestionnaireBindingPolicies struct {
	policies []QuestionnaireBindingPolicy
}

func NewQuestionnaireBindingPolicies(policies ...QuestionnaireBindingPolicy) QuestionnaireBindingPolicies {
	result := QuestionnaireBindingPolicies{policies: make([]QuestionnaireBindingPolicy, 0, len(policies))}
	for _, policy := range policies {
		if policy != nil {
			result.policies = append(result.policies, policy)
		}
	}
	return result
}

func (r QuestionnaireBindingPolicies) Validate(ctx context.Context, model *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	policy := r.resolve(model)
	if policy == nil {
		return binding, nil
	}
	return policy.Validate(ctx, model, binding)
}

func (r QuestionnaireBindingPolicies) BeforePublish(ctx context.Context, model *domain.AssessmentModel) error {
	policy := r.resolve(model)
	if policy == nil {
		return nil
	}
	return policy.BeforePublish(ctx, model)
}

func (r QuestionnaireBindingPolicies) resolve(model *domain.AssessmentModel) QuestionnaireBindingPolicy {
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

// QuestionnaireBindingPolicyFunc avoids family command services for simple
// composition-root policies.
type QuestionnaireBindingPolicyFunc struct {
	Match             func(domain.Identity) bool
	ValidateFunc      func(context.Context, *domain.AssessmentModel, domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error)
	BeforePublishFunc func(context.Context, *domain.AssessmentModel) error
}

func (f QuestionnaireBindingPolicyFunc) Supports(identity domain.Identity) bool {
	return f.Match != nil && f.Match(identity)
}

func (f QuestionnaireBindingPolicyFunc) Validate(ctx context.Context, model *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	if f.ValidateFunc == nil {
		return binding, nil
	}
	return f.ValidateFunc(ctx, model, binding)
}

func (f QuestionnaireBindingPolicyFunc) BeforePublish(ctx context.Context, model *domain.AssessmentModel) error {
	if f.BeforePublishFunc == nil {
		return nil
	}
	return f.BeforePublishFunc(ctx, model)
}
