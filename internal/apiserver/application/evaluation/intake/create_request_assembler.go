package intake

import (
	"strings"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type assessmentCreateRequestAssembler struct{}

func (assessmentCreateRequestAssembler) Assemble(dto CreateCommand) (domainAssessment.CreateAssessmentRequest, error) {
	if dto.TesteeID == 0 {
		return domainAssessment.CreateAssessmentRequest{}, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	if dto.QuestionnaireCode == "" {
		return domainAssessment.CreateAssessmentRequest{}, evalerrors.InvalidArgument("问卷编码不能为空")
	}
	if dto.AnswerSheetID == 0 {
		return domainAssessment.CreateAssessmentRequest{}, evalerrors.InvalidArgument("答卷ID不能为空")
	}

	orgID, err := safeconv.Uint64ToInt64(dto.OrgID)
	if err != nil {
		return domainAssessment.CreateAssessmentRequest{}, evalerrors.InvalidArgument("机构ID超出 int64 范围")
	}

	req := domainAssessment.CreateAssessmentRequest{
		OrgID:    orgID,
		TesteeID: meta.FromUint64(dto.TesteeID),
		QuestionnaireRef: domainAssessment.NewQuestionnaireRefByCode(
			meta.NewCode(dto.QuestionnaireCode),
			dto.QuestionnaireVersion,
		),
		AnswerSheetRef: domainAssessment.NewAnswerSheetRef(
			meta.FromUint64(dto.AnswerSheetID),
		),
	}

	if dto.ModelCode != nil {
		kind := domainAssessment.EvaluationModelKind(strings.TrimSpace(valueOrEmpty(dto.ModelKind)))
		if kind == "" {
			kind = domainAssessment.EvaluationModelKindScale
		}
		if !kind.IsValid() {
			return domainAssessment.CreateAssessmentRequest{}, evalerrors.InvalidArgument("不支持的解释模型类型: %s", kind)
		}
		subKind := modelcatalog.SubKind(strings.TrimSpace(valueOrEmpty(dto.ModelSubKind)))
		algorithm := modelcatalog.Algorithm(strings.TrimSpace(valueOrEmpty(dto.ModelAlgorithm)))
		modelRef := domainAssessment.NewEvaluationModelRefWithIdentity(
			kind,
			subKind,
			algorithm,
			meta.ID(0),
			meta.NewCode(strings.TrimSpace(*dto.ModelCode)),
			strings.TrimSpace(valueOrEmpty(dto.ModelVersion)),
			strings.TrimSpace(valueOrEmpty(dto.ModelTitle)),
		)
		req.ModelRef = &modelRef
	}

	switch dto.OriginType {
	case "", "adhoc":
		req.Origin = domainAssessment.NewAdhocOrigin()
	case "plan":
		if dto.OriginID != nil {
			req.Origin = domainAssessment.NewPlanOrigin(*dto.OriginID)
		}
	default:
		return domainAssessment.CreateAssessmentRequest{}, evalerrors.InvalidArgument("不支持的来源类型: %s", dto.OriginType)
	}

	return req, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
