package evaluationinput

import (
	"context"
	"fmt"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// ConfiguredTypologyModelInputProvider resolves configured personality-typology payloads.
type ConfiguredTypologyModelInputProvider struct {
	catalog             port.TypologyModelCatalog
	publishedModels     rulesetport.PublishedModelReader
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
}

func NewConfiguredTypologyModelInputProvider(
	catalog port.TypologyModelCatalog,
	publishedModels rulesetport.PublishedModelReader,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) ConfiguredTypologyModelInputProvider {
	return ConfiguredTypologyModelInputProvider{
		catalog:             catalog,
		publishedModels:     publishedModels,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
	}
}

func (ConfiguredTypologyModelInputProvider) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityPersonalityTypology
}

func (ConfiguredTypologyModelInputProvider) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathTypologyDescriptor
}

func (p ConfiguredTypologyModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	const algorithm = modelcatalog.AlgorithmPersonalityTypology
	if p.catalog == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("typology model catalog is not configured"), "人格模型不存在", "加载解释模型失败")
	}
	payload, err := p.catalog.GetTypologyModelByRef(ctx, ref.ModelRef)
	if err != nil {
		if modelcatalog.IsNotFound(err) {
			return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "人格模型不存在", "加载解释模型失败")
		}
		return nil, port.NewDependencyResolveError(port.DependencyCategoryModelCatalog, err, "加载解释模型依赖失败", "加载解释模型失败")
	}
	if payload == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("typology model payload is nil"), "人格模型不存在", "加载解释模型失败")
	}
	if payload.Algorithm != algorithm {
		err := fmt.Errorf("typology algorithm %s does not match provider %s", payload.Algorithm, algorithm)
		return nil, port.NewResolveError(port.FailureKindUnsupportedModel, err, "不支持的解释模型", "加载解释模型失败")
	}
	if !payload.IsPublished() {
		err := fmt.Errorf("typology model is not published: %s", payload.Code)
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "人格模型不可用", "加载解释模型失败")
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
	snapshot := &port.InputSnapshot{
		Model:         port.NewTypologyModelSnapshot(payload),
		ModelPayload:  port.TypologyModelPayload{Payload: payload},
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
	}
	if err := attachTypologyCanonical(ctx, p.publishedModels, ref, algorithm, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}
