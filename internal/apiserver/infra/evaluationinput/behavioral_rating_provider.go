package evaluationinput

import (
	"context"
	"fmt"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type BehavioralRatingModelInputProvider struct {
	algorithm           modelcatalog.Algorithm
	catalog             port.BehavioralRatingModelCatalog
	publishedModels     rulesetport.PublishedModelReader
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
	normSubjectReader   port.NormSubjectReader
}

func NewBehavioralRatingModelInputProvider(
	algorithm modelcatalog.Algorithm,
	catalog port.BehavioralRatingModelCatalog,
	publishedModels rulesetport.PublishedModelReader,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
	normSubjectReader port.NormSubjectReader,
) BehavioralRatingModelInputProvider {
	return BehavioralRatingModelInputProvider{
		algorithm:           algorithm,
		catalog:             catalog,
		publishedModels:     publishedModels,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
		normSubjectReader:   normSubjectReader,
	}
}

func (p BehavioralRatingModelInputProvider) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.BehavioralRatingIdentity(p.algorithm)
}

func (BehavioralRatingModelInputProvider) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathBehavioralRatingDescriptor
}

func (p BehavioralRatingModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	if p.catalog == nil {
		return nil, port.NewResolveError(port.FailureKindModelNotFound, fmt.Errorf("behavioral_rating catalog is not configured"), "解释模型不存在", "加载解释模型失败")
	}
	if ref.ModelRef.Algorithm == "" {
		ref.ModelRef.Algorithm = string(p.algorithm)
	} else if modelcatalog.Algorithm(ref.ModelRef.Algorithm) != p.algorithm {
		err := fmt.Errorf("behavioral_rating algorithm %s does not match provider %s", ref.ModelRef.Algorithm, p.algorithm)
		return nil, port.NewResolveError(port.FailureKindUnsupportedModel, err, "不支持的解释模型", "加载解释模型失败")
	}
	model, err := p.catalog.GetBehavioralRatingByRef(ctx, ref.ModelRef)
	if err != nil {
		if modelcatalog.IsNotFound(err) {
			return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "解释模型不存在", "加载解释模型失败")
		}
		return nil, port.NewDependencyResolveError(port.DependencyCategoryModelCatalog, err, "加载解释模型依赖失败", "加载解释模型失败")
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
	payload := port.BehavioralRatingModelPayload{Snapshot: model}
	snapshot := &port.InputSnapshot{
		Model:         port.NewBehavioralRatingModelSnapshot(model, p.algorithm),
		ModelPayload:  payload,
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
		NormSubject:   normSubject,
	}
	attachBehavioralCanonical(ctx, p.publishedModels, ref, snapshot)
	return snapshot, nil
}
