package modelcatalog

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// AssessmentBinding 建测评时从问卷解析出的不可变测评模型绑定。
type AssessmentBinding struct {
	Ref Ref
}

// AssessmentBindingResolver 统一解析问卷 -> 测评模型绑定。
type AssessmentBindingResolver interface {
	QuestionnaireResolver
	ResolveAssessmentBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) (AssessmentBinding, bool, error)
}

func (b AssessmentBinding) ModelKind() domain.Kind {
	return b.Ref.Kind
}
