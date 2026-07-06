package evaluationinput

import (
	"context"
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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
	for _, v2Ref := range typologyLookupRefs(ref, algorithm) {
		payload, err := c.decodePublishedModelRef(ctx, v2Ref)
		if err == nil {
			if algorithm != "" {
				return assertTypologyAlgorithm(payload, algorithm)
			}
			return payload, nil
		}
		if !domain.IsNotFound(err) {
			return nil, err
		}
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
		payload, err := modeltypology.DecodeFromSnapshot(domain.LegacyFromPublished(snapshot))
		if err != nil {
			return nil, err
		}
		return ensurePublishedTypologyPayload(payload)
	}
	var payload modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		return nil, err
	}
	return ensurePublishedTypologyPayload(&payload)
}

func ensurePublishedTypologyPayload(payload *modeltypology.Payload) (*modeltypology.Payload, error) {
	if payload == nil {
		return nil, domain.ErrNotFound
	}
	if !payload.IsPublished() {
		return nil, fmt.Errorf("typology model is not published: %s", payload.Code)
	}
	return payload, nil
}
