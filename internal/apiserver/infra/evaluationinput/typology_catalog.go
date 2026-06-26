package evaluationinput

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type mbtiTypologyCatalog struct {
	catalog port.MBTIModelCatalog
}

func NewMBTITypologyCatalog(catalog port.MBTIModelCatalog) port.TypologyModelCatalog {
	return mbtiTypologyCatalog{catalog: catalog}
}

func (c mbtiTypologyCatalog) GetTypologyModelByRef(ctx context.Context, ref port.ModelRef) (*modeltypology.Payload, error) {
	if c.catalog == nil {
		return nil, fmt.Errorf("mbti model catalog is not configured")
	}
	model, err := c.catalog.GetMBTIModelByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return modeltypology.FromMBTI(model), nil
}

type sbtiTypologyCatalog struct {
	catalog port.SBTIModelCatalog
}

func NewSBTITypologyCatalog(catalog port.SBTIModelCatalog) port.TypologyModelCatalog {
	return sbtiTypologyCatalog{catalog: catalog}
}

func (c sbtiTypologyCatalog) GetTypologyModelByRef(ctx context.Context, ref port.ModelRef) (*modeltypology.Payload, error) {
	if c.catalog == nil {
		return nil, fmt.Errorf("sbti model catalog is not configured")
	}
	model, err := c.catalog.GetSBTIModelByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return modeltypology.FromSBTI(model), nil
}

type TypologyModelInputProvider struct {
	algorithm           assessmentmodel.Algorithm
	catalog             port.TypologyModelCatalog
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
}

func NewTypologyModelInputProvider(
	algorithm assessmentmodel.Algorithm,
	catalog port.TypologyModelCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) TypologyModelInputProvider {
	return TypologyModelInputProvider{
		algorithm:           algorithm,
		catalog:             catalog,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
	}
}

func (p TypologyModelInputProvider) EvaluatorKey() evaldomain.EvaluatorKey {
	return evaldomain.PersonalityTypologyKey(p.algorithm)
}

func (p TypologyModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	if p.catalog == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("typology model catalog is not configured"), typologyModelNotFoundMessage(p.algorithm), "加载解释模型失败")
	}
	payload, err := p.catalog.GetTypologyModelByRef(ctx, ref.ModelRef)
	if err != nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, typologyModelNotFoundMessage(p.algorithm), "加载解释模型失败")
	}
	if payload == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("typology model payload is nil"), typologyModelNotFoundMessage(p.algorithm), "加载解释模型失败")
	}
	if payload.Algorithm != p.algorithm {
		err := fmt.Errorf("typology algorithm %s does not match provider %s", payload.Algorithm, p.algorithm)
		return nil, port.NewResolveError(port.FailureKindUnsupportedModel, err, "不支持的解释模型", "加载解释模型失败")
	}
	if !payload.IsPublished() {
		err := fmt.Errorf("typology model is not published: %s", payload.Code)
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, typologyModelUnavailableMessage(p.algorithm), "加载解释模型失败")
	}

	answerSheet, err := p.answerSheetReader.GetAnswerSheet(ctx, ref.AnswerSheetID)
	if err != nil {
		return nil, err
	}
	if !payload.MatchesQuestionnaire(answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion) {
		err := fmt.Errorf("answersheet questionnaire %s@%s does not match typology model questionnaire %s@%s",
			answerSheet.QuestionnaireCode,
			answerSheet.QuestionnaireVersion,
			payload.QuestionnaireCode,
			payload.QuestionnaireVersion,
		)
		return nil, port.NewResolveError(port.FailureKindQuestionnaireVersionMismatch, err, "问卷版本不匹配", "加载问卷失败")
	}

	qnr, err := p.questionnaireReader.GetQuestionnaire(ctx, answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}
	return &port.InputSnapshot{
		Model:         port.NewTypologyModelSnapshot(payload),
		ModelPayload:  port.TypologyModelPayload{Payload: payload},
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
	}, nil
}

func typologyModelNotFoundMessage(algorithm assessmentmodel.Algorithm) string {
	switch algorithm {
	case assessmentmodel.AlgorithmSBTI:
		return "SBTI 模型不存在"
	case assessmentmodel.AlgorithmMBTI:
		return "MBTI 模型不存在"
	default:
		return "人格模型不存在"
	}
}

func typologyModelUnavailableMessage(algorithm assessmentmodel.Algorithm) string {
	switch algorithm {
	case assessmentmodel.AlgorithmSBTI:
		return "SBTI 模型不可用"
	case assessmentmodel.AlgorithmMBTI:
		return "MBTI 模型不可用"
	default:
		return "人格模型不可用"
	}
}
