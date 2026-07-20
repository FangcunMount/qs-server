package evaluationinput

import (
	"context"
	"fmt"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type CognitiveModelInputProvider struct {
	algorithm           modelcatalog.Algorithm
	catalog             port.CognitiveModelCatalog
	publishedModels     rulesetport.PublishedModelReader
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
	normSubjectReader   port.NormSubjectReader
}

func NewCognitiveModelInputProvider(
	algorithm modelcatalog.Algorithm,
	catalog port.CognitiveModelCatalog,
	publishedModels rulesetport.PublishedModelReader,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
	normSubjectReader port.NormSubjectReader,
) CognitiveModelInputProvider {
	return CognitiveModelInputProvider{
		algorithm:           algorithm,
		catalog:             catalog,
		publishedModels:     publishedModels,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
		normSubjectReader:   normSubjectReader,
	}
}

func (p CognitiveModelInputProvider) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.CognitiveIdentity(p.algorithm)
}

func (CognitiveModelInputProvider) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathCognitiveDescriptor
}

func (p CognitiveModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	if p.catalog == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("cognitive catalog is not configured"), "解释模型不存在", "加载解释模型失败")
	}
	if ref.ModelRef.Algorithm == "" {
		ref.ModelRef.Algorithm = string(p.algorithm)
	} else if modelcatalog.Algorithm(ref.ModelRef.Algorithm) != p.algorithm {
		err := fmt.Errorf("cognitive algorithm %s does not match provider %s", ref.ModelRef.Algorithm, p.algorithm)
		return nil, port.NewResolveError(port.FailureKindUnsupportedModel, err, "不支持的解释模型", "加载解释模型失败")
	}
	model, err := p.catalog.GetCognitiveByRef(ctx, ref.ModelRef)
	if err != nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "解释模型不存在", "加载解释模型失败")
	}
	answerSheet, err := p.answerSheetReader.GetAnswerSheet(ctx, ref.AnswerSheetID)
	if err != nil {
		return nil, err
	}
	qnr, err := p.questionnaireReader.GetQuestionnaire(ctx, answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}
	normSubject, err := resolveNormSubject(ctx, p.normSubjectReader, ref)
	if err != nil {
		return nil, err
	}
	payload := port.CognitiveModelPayload{Snapshot: model}
	snapshot := &port.InputSnapshot{
		Model:         port.NewCognitiveModelSnapshot(model),
		ModelPayload:  payload,
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
		NormSubject:   normSubject,
	}
	attachCognitiveCanonical(ctx, p.publishedModels, ref, snapshot)
	return snapshot, nil
}
