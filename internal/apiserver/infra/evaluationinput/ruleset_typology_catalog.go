package evaluationinput

import (
	"context"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
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
	if payload, err := c.decodePublishedRef(ctx, v2Ref); err == nil {
		return assertTypologyAlgorithm(payload, algorithm)
	} else if !domain.IsNotFound(err) {
		return nil, err
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
