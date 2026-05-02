package assessment

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type assessmentCreateRequestAssembler struct{}

func (assessmentCreateRequestAssembler) Assemble(dto CreateAssessmentDTO) (domainAssessment.CreateAssessmentRequest, error) {
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

	if dto.MedicalScaleID != nil {
		scaleCode := ""
		if dto.MedicalScaleCode != nil {
			scaleCode = *dto.MedicalScaleCode
		}
		scaleName := ""
		if dto.MedicalScaleName != nil {
			scaleName = *dto.MedicalScaleName
		}
		scaleRef := domainAssessment.NewMedicalScaleRef(
			meta.FromUint64(*dto.MedicalScaleID),
			meta.NewCode(scaleCode),
			scaleName,
		)
		req.MedicalScaleRef = &scaleRef
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
