package evaluationinput

import (
	"context"
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// PublishedTypologyCatalog loads personality typology payloads from v2 published-model snapshots.
type PublishedTypologyCatalog struct {
	reader   rulesetport.PublishedModelReader
	fallback RuleSetTypologyCatalog
}

func NewPublishedTypologyCatalog(
	reader rulesetport.PublishedModelReader,
	legacy rulesetport.PublishedReader,
) PublishedTypologyCatalog {
	return PublishedTypologyCatalog{
		reader:   reader,
		fallback: NewRuleSetTypologyCatalog(legacy),
	}
}

func (c PublishedTypologyCatalog) GetTypologyModelByRef(ctx context.Context, ref port.ModelRef) (*modeltypology.Payload, error) {
	if c.reader == nil {
		return c.fallback.GetTypologyModelByRef(ctx, ref)
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	algorithm := resolveTypologyAlgorithm(ref)
	if algorithm == "" {
		return nil, fmt.Errorf("typology algorithm is required")
	}
	v2Ref := rulesetport.Ref{
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: algorithm,
		Code:      ref.Code,
		Version:   ref.Version,
	}
	if payload, err := c.decodePublishedModelRef(ctx, v2Ref); err == nil {
		return assertTypologyAlgorithm(payload, algorithm)
	} else if !domain.IsNotFound(err) {
		return nil, err
	}
	return c.fallback.GetTypologyModelByRef(ctx, ref)
}

func (c PublishedTypologyCatalog) FindTypologyModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*modeltypology.Payload, error) {
	if c.reader == nil {
		return c.fallback.FindTypologyModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	}
	snapshot, err := c.reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err == nil {
		return decodePublishedTypologyModel(snapshot)
	}
	if !domain.IsNotFound(err) {
		return nil, err
	}
	return c.fallback.FindTypologyModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (c PublishedTypologyCatalog) decodePublishedModelRef(ctx context.Context, ref rulesetport.Ref) (*modeltypology.Payload, error) {
	snapshot, err := c.reader.GetPublishedModelByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return decodePublishedTypologyModel(snapshot)
}

func decodePublishedTypologyModel(snapshot *domain.PublishedModelSnapshot) (*modeltypology.Payload, error) {
	if snapshot == nil {
		return nil, domain.ErrNotFound
	}
	if snapshot.PayloadFormat != domain.PayloadFormatPersonalityTypologyV1 {
		return modeltypology.DecodeFromSnapshot(domain.LegacyFromPublished(snapshot))
	}
	var payload modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}
