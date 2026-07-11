package binding

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Policy owns identity-specific questionnaire constraints.
// It is intentionally separate from the catalogue command flow so no command
// service needs a family switch.
type Policy interface {
	Supports(domain.Identity) bool
	Validate(context.Context, *domain.AssessmentModel, domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error)
	BeforePublish(context.Context, *domain.AssessmentModel) error
}

type Policies struct {
	policies []Policy
}

func NewPolicies(policies ...Policy) Policies {
	result := Policies{policies: make([]Policy, 0, len(policies))}
	for _, policy := range policies {
		if policy != nil {
			result.policies = append(result.policies, policy)
		}
	}
	return result
}

func (r Policies) Validate(ctx context.Context, model *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	policy := r.resolve(model)
	if policy == nil {
		return binding, nil
	}
	return policy.Validate(ctx, model, binding)
}

func (r Policies) BeforePublish(ctx context.Context, model *domain.AssessmentModel) error {
	policy := r.resolve(model)
	if policy == nil {
		return nil
	}
	return policy.BeforePublish(ctx, model)
}

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

// PolicyFunc avoids family command services for simple
// composition-root policies.
type PolicyFunc struct {
	Match             func(domain.Identity) bool
	ValidateFunc      func(context.Context, *domain.AssessmentModel, domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error)
	BeforePublishFunc func(context.Context, *domain.AssessmentModel) error
}

func (f PolicyFunc) Supports(identity domain.Identity) bool {
	return f.Match != nil && f.Match(identity)
}

func (f PolicyFunc) Validate(ctx context.Context, model *domain.AssessmentModel, binding domain.QuestionnaireBinding) (domain.QuestionnaireBinding, error) {
	if f.ValidateFunc == nil {
		return binding, nil
	}
	return f.ValidateFunc(ctx, model, binding)
}

func (f PolicyFunc) BeforePublish(ctx context.Context, model *domain.AssessmentModel) error {
	if f.BeforePublishFunc == nil {
		return nil
	}
	return f.BeforePublishFunc(ctx, model)
}
