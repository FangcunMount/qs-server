package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

// PublishedCognitiveCatalog loads cognitive payloads from v2 published-model snapshots.
type PublishedCognitiveCatalog struct {
	reader rulesetport.PublishedModelReader
}

func NewPublishedCognitiveCatalog(reader rulesetport.PublishedModelReader) PublishedCognitiveCatalog {
	return PublishedCognitiveCatalog{reader: reader}
}

func (c PublishedCognitiveCatalog) GetCognitiveByRef(ctx context.Context, ref port.ModelRef) (*taskperfsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published cognitive reader is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	snapshot, err := c.reader.GetPublishedModelByRef(ctx, cognitiveLookupRef(ref))
	if err != nil {
		return nil, err
	}
	return decodePublishedCognitiveModel(snapshot)
}

func (c PublishedCognitiveCatalog) FindCognitiveByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*taskperfsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published cognitive reader is not configured")
	}
	snapshot, err := c.reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return decodePublishedCognitiveModel(snapshot)
}

func cognitiveLookupRef(ref port.ModelRef) rulesetport.Ref {
	algorithm := domain.Algorithm(ref.Algorithm)
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	return rulesetport.Ref{
		Kind:      domain.KindCognitive,
		Algorithm: algorithm,
		Code:      ref.Code,
		Version:   ref.Version,
		Title:     ref.Title,
	}
}

func decodePublishedCognitiveModel(model *rulesetport.PublishedModel) (*taskperfsnapshot.Snapshot, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.Kind != domain.KindCognitive {
		return nil, fmt.Errorf("published model kind = %q, want cognitive", model.Kind)
	}
	if !domain.IsCognitivePayloadFormat(model.PayloadFormat) {
		return nil, fmt.Errorf("unsupported cognitive payload format: %s", model.PayloadFormat)
	}
	if model.DefinitionV2 == nil {
		return nil, fmt.Errorf("cognitive definition_v2 is required for runtime: %s", model.Code)
	}
	payload, err := taskperfsnapshot.SnapshotFromDefinition(taskperfsnapshot.DefinitionEnvelope{
		Code: model.Code, Version: model.Version, Title: model.Title, QuestionnaireCode: model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion, Status: model.Status,
	}, model.DefinitionV2)
	if err != nil {
		return nil, err
	}
	if !payload.IsPublished() {
		return nil, fmt.Errorf("cognitive model is not published: %s", payload.Code)
	}
	return payload, nil
}
