package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
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

func typologyLookupRefs(ref port.ModelRef, algorithm domain.Algorithm) []rulesetport.Ref {
	if algorithm == "" {
		refs := make([]rulesetport.Ref, 0, 3)
		if ref.SubKind != "" {
			refs = append(refs, rulesetport.Ref{
				Kind:    domain.KindTypology,
				SubKind: domain.SubKind(ref.SubKind),
				Code:    ref.Code,
				Version: ref.Version,
			})
		}
		refs = append(refs,
			rulesetport.Ref{
				Kind:    domain.KindTypology,
				Code:    ref.Code,
				Version: ref.Version,
			},
			rulesetport.Ref{
				Kind:    domain.KindTypology,
				SubKind: domain.SubKindTypology,
				Code:    ref.Code,
				Version: ref.Version,
			},
		)
		return refs
	}
	algorithms := []domain.Algorithm{algorithm}
	seen := make(map[domain.Algorithm]struct{}, len(algorithms))
	refs := make([]rulesetport.Ref, 0, len(algorithms))
	for _, alg := range algorithms {
		if _, ok := seen[alg]; ok {
			continue
		}
		seen[alg] = struct{}{}
		refs = append(refs, rulesetport.Ref{
			Kind:      domain.KindTypology,
			SubKind:   domain.SubKindTypology,
			Algorithm: alg,
			Code:      ref.Code,
			Version:   ref.Version,
		})
	}
	return refs
}

func assertTypologyAlgorithm(payload *modeltypology.Payload, algorithm domain.Algorithm) (*modeltypology.Payload, error) {
	if payload == nil {
		return nil, fmt.Errorf("typology payload is nil")
	}
	if payload.Algorithm == algorithm {
		return payload, nil
	}
	return nil, fmt.Errorf("typology algorithm %s does not match ref %s", payload.Algorithm, algorithm)
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

func decodePublishedTypologyModel(model *rulesetport.PublishedModel) (*modeltypology.Payload, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.PayloadFormat != domain.PayloadFormatPersonalityTypologyV1 {
		return nil, fmt.Errorf("unsupported typology payload format %q", model.PayloadFormat)
	}
	if model.DefinitionV2 == nil {
		return nil, fmt.Errorf("typology definition_v2 is required for runtime: %s", model.Code)
	}
	payload, err := modeltypology.PayloadFromDefinition(modeltypology.DefinitionEnvelope{
		Code: model.Code, Version: model.Version, Title: model.Title, QuestionnaireCode: model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion, Status: model.Status, Algorithm: model.Algorithm,
	}, model.DefinitionV2)
	if err != nil {
		return nil, err
	}
	payload, err = ensurePublishedTypologyPayload(payload)
	if err != nil {
		return nil, err
	}
	payload.PublishedRuntime = rulesetport.RuntimeMetaFromPublished(model)
	return payload, nil
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

func resolveTypologyAlgorithm(ref port.ModelRef) domain.Algorithm {
	if ref.Algorithm != "" {
		return domain.Algorithm(ref.Algorithm)
	}
	return ""
}
