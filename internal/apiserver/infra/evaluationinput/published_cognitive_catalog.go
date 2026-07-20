package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

// PublishedCognitiveCatalog loads cognitive payloads from v2 published-model snapshots.
type PublishedCognitiveCatalog struct {
	reader rulesetport.PublishedModelReader
	norms  rulesetport.NormRepository
}

func NewPublishedCognitiveCatalog(reader rulesetport.PublishedModelReader, norms ...rulesetport.NormRepository) PublishedCognitiveCatalog {
	var normRepo rulesetport.NormRepository
	if len(norms) > 0 {
		normRepo = norms[0]
	}
	return PublishedCognitiveCatalog{reader: reader, norms: normRepo}
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
	return c.decodePublished(ctx, snapshot)
}

func (c PublishedCognitiveCatalog) FindCognitiveByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*taskperfsnapshot.Snapshot, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published cognitive reader is not configured")
	}
	snapshot, err := c.reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return c.decodePublished(ctx, snapshot)
}

func (c PublishedCognitiveCatalog) decodePublished(ctx context.Context, model *rulesetport.PublishedModel) (*taskperfsnapshot.Snapshot, error) {
	snapshot, err := decodePublishedCognitiveModel(model)
	if err != nil || snapshot == nil || snapshot.SPM == nil || len(snapshot.SPM.ItemSets) == 0 || model == nil || model.DefinitionV2 == nil {
		return snapshot, err
	}
	var table *norm.Norm
	for _, ref := range model.DefinitionV2.Calibration.NormRefs {
		if ref.FactorCode != snapshot.SPM.TotalFactorCode || ref.NormTableVersion == "" {
			continue
		}
		if c.norms == nil {
			return nil, fmt.Errorf("cognitive norm repository is not configured")
		}
		table, err = c.norms.FindNorm(ctx, ref.NormTableVersion)
		if err != nil {
			return nil, err
		}
		break
	}
	snapshot.SPM.NormTables = taskperfsnapshot.NormTablesFromCatalog(table)
	return snapshot, nil
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
	payload.PublishedRuntime = rulesetport.RuntimeMetaFromPublished(model)
	return payload, nil
}
