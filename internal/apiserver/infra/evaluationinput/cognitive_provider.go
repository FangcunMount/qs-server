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
	catalog             port.CognitiveModelCatalog
	publishedModels     rulesetport.PublishedModelReader
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
	normSubjectReader   port.NormSubjectReader
}

func NewCognitiveModelInputProvider(
	catalog port.CognitiveModelCatalog,
	publishedModels rulesetport.PublishedModelReader,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
	normSubjectReader port.NormSubjectReader,
) CognitiveModelInputProvider {
	return CognitiveModelInputProvider{
		catalog:             catalog,
		publishedModels:     publishedModels,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
		normSubjectReader:   normSubjectReader,
	}
}

func (CognitiveModelInputProvider) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityCognitiveDefault
}

func (CognitiveModelInputProvider) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathCognitiveDescriptor
}

func (p CognitiveModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	if p.catalog == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("cognitive catalog is not configured"), "解释模型不存在", "加载解释模型失败")
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
