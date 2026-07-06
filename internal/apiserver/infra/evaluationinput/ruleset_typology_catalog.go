package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// RuleSetTypologyCatalog loads unified typology payloads from the ruleset catalog.
type RuleSetTypologyCatalog struct {
	reader rulesetport.PublishedReader
}

func NewRuleSetTypologyCatalog(reader rulesetport.PublishedReader) RuleSetTypologyCatalog {
	return RuleSetTypologyCatalog{reader: reader}
}

func (c RuleSetTypologyCatalog) GetTypologyModelByRef(ctx context.Context, ref port.ModelRef) (*modeltypology.Payload, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	algorithm := resolveTypologyAlgorithm(ref)

	for _, v2Ref := range typologyLookupRefs(ref, algorithm) {
		payload, err := c.decodePublishedRef(ctx, v2Ref)
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

	if algorithm == "" {
		return nil, domain.ErrNotFound
	}

	if legacyKind := legacyKindForAlgorithm(algorithm); legacyKind != "" {
		legacyRef := rulesetport.Ref{
			Kind:    legacyKind,
			Code:    ref.Code,
			Version: ref.Version,
		}
		payload, err := c.decodePublishedRef(ctx, legacyRef)
		if err != nil {
			return nil, err
		}
		return assertTypologyAlgorithm(payload, algorithm)
	}
	return nil, domain.ErrNotFound
}

func (c RuleSetTypologyCatalog) FindTypologyModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*modeltypology.Payload, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("ruleset catalog is not configured")
	}
	snapshot, err := c.reader.FindPublishedByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return modeltypology.DecodeFromSnapshot(snapshot)
}

func (c RuleSetTypologyCatalog) decodePublishedRef(ctx context.Context, ref rulesetport.Ref) (*modeltypology.Payload, error) {
	snapshot, err := c.reader.GetPublishedByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return modeltypology.DecodeFromSnapshot(snapshot)
}

func resolveTypologyAlgorithm(ref port.ModelRef) domain.Algorithm {
	if ref.Algorithm != "" {
		return domain.Algorithm(ref.Algorithm)
	}
	if _, _, algorithm, ok := domain.LegacyKindMapping(domain.Kind(ref.Kind)); ok {
		return algorithm
	}
	return ""
}

func typologyLookupRefs(ref port.ModelRef, algorithm domain.Algorithm) []rulesetport.Ref {
	if algorithm != "" {
		return []rulesetport.Ref{{
			Kind:      domain.KindPersonality,
			SubKind:   domain.SubKindTypology,
			Algorithm: algorithm,
			Code:      ref.Code,
			Version:   ref.Version,
		}}
	}
	refs := make([]rulesetport.Ref, 0, 3)
	if ref.SubKind != "" {
		refs = append(refs, rulesetport.Ref{
			Kind:    domain.KindPersonality,
			SubKind: domain.SubKind(ref.SubKind),
			Code:    ref.Code,
			Version: ref.Version,
		})
	}
	refs = append(refs,
		rulesetport.Ref{
			Kind:    domain.KindPersonality,
			Code:    ref.Code,
			Version: ref.Version,
		},
		rulesetport.Ref{
			Kind:    domain.KindPersonality,
			SubKind: domain.SubKindTypology,
			Code:    ref.Code,
			Version: ref.Version,
		},
	)
	return refs
}

func legacyKindForAlgorithm(algorithm domain.Algorithm) domain.Kind {
	switch algorithm {
	case domain.AlgorithmMBTI:
		return domain.KindMBTIMigration
	case domain.AlgorithmSBTI:
		return domain.KindSBTIMigration
	default:
		return ""
	}
}

func assertTypologyAlgorithm(payload *modeltypology.Payload, algorithm domain.Algorithm) (*modeltypology.Payload, error) {
	if payload == nil {
		return nil, fmt.Errorf("typology payload is nil")
	}
	if payload.Algorithm != algorithm {
		return nil, fmt.Errorf("typology algorithm %s does not match ref %s", payload.Algorithm, algorithm)
	}
	return payload, nil
}
