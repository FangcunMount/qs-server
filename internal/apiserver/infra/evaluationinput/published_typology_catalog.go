package evaluationinput

import (
	"context"
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedTypologyCatalog loads personality typology payloads from v2 published-model snapshots.
type PublishedTypologyCatalog struct {
	reader rulesetport.PublishedModelReader
}

func NewPublishedTypologyCatalog(reader rulesetport.PublishedModelReader) PublishedTypologyCatalog {
	return PublishedTypologyCatalog{reader: reader}
}

func (c PublishedTypologyCatalog) GetTypologyModelByRef(ctx context.Context, ref port.ModelRef) (*modeltypology.Payload, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published typology catalog is not configured")
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
	return nil, domain.ErrNotFound
}

func (c PublishedTypologyCatalog) FindTypologyModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*modeltypology.Payload, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("published typology catalog is not configured")
	}
	snapshot, err := c.reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return decodePublishedTypologyModel(snapshot)
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
		payload, err := legacy.DecodeTypologyFromSnapshot(domain.LegacyFromPublished(snapshot))
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
