package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	personalityseed "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/seed"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestRuleSetTypologyCatalogLookupBigFiveWithoutAlgorithm(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:      "BIG5_IPIP_50",
		Version:   "1.0.0",
		Algorithm: domain.AlgorithmBigFive,
		Status:    "published",
	}
	payloadBytes, format, err := codec.EncodeTypology(payload)
	if err != nil {
		t.Fatalf("EncodeTypology: %v", err)
	}
	reader := stubRuleReader{snapshot: &domain.RuleSetSnapshot{
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind:    domain.KindPersonality,
			Code:    payload.Code,
			Version: payload.Version,
		},
		Payload: payloadBytes,
	}}
	catalog := NewRuleSetTypologyCatalog(reader)
	got, err := catalog.GetTypologyModelByRef(t.Context(), port.ModelRef{
		Kind:    port.EvaluationModelKindPersonality,
		Code:    payload.Code,
		Version: payload.Version,
	})
	if err != nil {
		t.Fatalf("GetTypologyModelByRef: %v", err)
	}
	if got.Algorithm != domain.AlgorithmBigFive {
		t.Fatalf("Algorithm = %s, want bigfive", got.Algorithm)
	}
}

func TestRuleSetTypologyCatalogLookupWithoutAlgorithm(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:      "ENNEAGRAM_45",
		Version:   "1.0.0",
		Algorithm: domain.AlgorithmPersonalityTypology,
		Status:    "published",
	}
	payloadBytes, format, err := codec.EncodeTypology(payload)
	if err != nil {
		t.Fatalf("EncodeTypology: %v", err)
	}
	reader := stubRuleReader{snapshot: &domain.RuleSetSnapshot{
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind:    domain.KindPersonality,
			Code:    payload.Code,
			Version: payload.Version,
		},
		Payload: payloadBytes,
	}}
	catalog := NewRuleSetTypologyCatalog(reader)
	got, err := catalog.GetTypologyModelByRef(t.Context(), port.ModelRef{
		Kind:    port.EvaluationModelKindPersonality,
		Code:    payload.Code,
		Version: payload.Version,
	})
	if err != nil {
		t.Fatalf("GetTypologyModelByRef: %v", err)
	}
	if got.Algorithm != domain.AlgorithmPersonalityTypology {
		t.Fatalf("Algorithm = %s, want personality_typology", got.Algorithm)
	}
}

func TestRuleSetTypologyCatalogDecodesV2BigFivePayload(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:      "BIGFIVE_V1",
		Version:   "1.0.0",
		Algorithm: domain.AlgorithmBigFive,
		Status:    "published",
	}
	payloadBytes, format, err := codec.EncodeTypology(payload)
	if err != nil {
		t.Fatalf("EncodeTypology: %v", err)
	}
	reader := stubRuleReader{snapshot: &domain.RuleSetSnapshot{
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind:    domain.KindPersonality,
			Code:    payload.Code,
			Version: payload.Version,
		},
		Payload: payloadBytes,
	}}
	catalog := NewRuleSetTypologyCatalog(reader)
	got, err := catalog.GetTypologyModelByRef(t.Context(), port.ModelRef{
		Kind:      port.EvaluationModelKindPersonality,
		SubKind:   string(domain.SubKindTypology),
		Algorithm: string(domain.AlgorithmBigFive),
		Code:      payload.Code,
		Version:   payload.Version,
	})
	if err != nil {
		t.Fatalf("GetTypologyModelByRef: %v", err)
	}
	if got.Algorithm != domain.AlgorithmBigFive {
		t.Fatalf("Algorithm = %s", got.Algorithm)
	}
}

func TestPublishedTypologyCatalogDecodesPublishedModelSnapshot(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:      "PUBLISHED_MBTI",
		Version:   "v4",
		Algorithm: domain.AlgorithmMBTI,
		Status:    "published",
	}
	payloadBytes, format, err := codec.EncodeTypology(payload)
	if err != nil {
		t.Fatalf("EncodeTypology: %v", err)
	}
	reader := stubPublishedModelReader{snapshot: &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: format,
		Model: domain.ModelDefinition{
			Kind:      domain.KindPersonality,
			SubKind:   domain.SubKindTypology,
			Algorithm: domain.AlgorithmMBTI,
			Code:      payload.Code,
			Version:   payload.Version,
			Status:    "published",
		},
		Payload: payloadBytes,
	}}
	catalog := NewPublishedTypologyCatalog(reader, nil)
	got, err := catalog.GetTypologyModelByRef(t.Context(), port.ModelRef{
		Kind:      port.EvaluationModelKindPersonality,
		SubKind:   string(domain.SubKindTypology),
		Algorithm: string(domain.AlgorithmMBTI),
		Code:      payload.Code,
		Version:   payload.Version,
	})
	if err != nil {
		t.Fatalf("GetTypologyModelByRef: %v", err)
	}
	if got.Code != payload.Code || got.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("payload = %#v", got)
	}
}

func TestPublishedTypologyCatalogFallsBackToLegacyReader(t *testing.T) {
	model := &modeltypology.MBTILegacyModel{
		Code:                 personalityseed.MBTIModelCode,
		Version:              personalityseed.MBTIModelVersion,
		QuestionnaireCode:    personalityseed.MBTIQuestionnaireCode,
		QuestionnaireVersion: personalityseed.MBTIModelVersion,
		Status:               "published",
		DimensionOrder:       []string{"EI"},
		Dimensions: map[string]modeltypology.MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"},
		},
		TypeProfiles: []modeltypology.MBTILegacyTypeProfile{
			{TypeCode: "INTJ", TypeName: "建筑师"},
		},
	}
	payload, format, err := codec.EncodeMBTI(model)
	if err != nil {
		t.Fatalf("EncodeMBTI: %v", err)
	}
	legacy := stubRuleReader{snapshot: &domain.RuleSetSnapshot{
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind: domain.KindPersonality, Code: model.Code, Version: model.Version,
		},
		Payload: payload,
	}}
	catalog := NewPublishedTypologyCatalog(stubPublishedModelReader{err: domain.ErrNotFound}, legacy)
	got, err := catalog.GetTypologyModelByRef(t.Context(), port.ModelRef{
		Kind:      port.EvaluationModelKindPersonality,
		Algorithm: string(domain.AlgorithmMBTI),
		Code:      model.Code,
		Version:   model.Version,
	})
	if err != nil {
		t.Fatalf("GetTypologyModelByRef: %v", err)
	}
	if got.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("Algorithm = %s", got.Algorithm)
	}
}

func TestRuleSetTypologyCatalogMBTILookupViaV2Ref(t *testing.T) {
	model := &modeltypology.MBTILegacyModel{
		Code:                 personalityseed.MBTIModelCode,
		Version:              personalityseed.MBTIModelVersion,
		QuestionnaireCode:    personalityseed.MBTIQuestionnaireCode,
		QuestionnaireVersion: personalityseed.MBTIModelVersion,
		Status:               "published",
		DimensionOrder:       []string{"EI"},
		Dimensions: map[string]modeltypology.MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"},
		},
		TypeProfiles: []modeltypology.MBTILegacyTypeProfile{
			{TypeCode: "INTJ", TypeName: "建筑师"},
		},
	}
	payload, format, err := codec.EncodeMBTI(model)
	if err != nil {
		t.Fatalf("EncodeMBTI: %v", err)
	}
	reader := stubRuleReader{snapshot: &domain.RuleSetSnapshot{
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind: domain.KindPersonality, Code: model.Code, Version: model.Version,
		},
		Payload: payload,
	}}
	catalog := NewRuleSetTypologyCatalog(reader)
	got, err := catalog.GetTypologyModelByRef(t.Context(), port.ModelRef{
		Kind:      "mbti",
		Algorithm: string(domain.AlgorithmMBTI),
		Code:      model.Code,
		Version:   model.Version,
	})
	if err != nil {
		t.Fatalf("GetTypologyModelByRef: %v", err)
	}
	if got.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("Algorithm = %s", got.Algorithm)
	}
}

func TestRuleSetTypologyCatalogV2BigFiveUsesPersonalityRef(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:      "BF",
		Version:   "1.0.0",
		Algorithm: domain.AlgorithmBigFive,
		Status:    "published",
	}
	payloadBytes, format, err := codec.EncodeTypology(payload)
	if err != nil {
		t.Fatalf("EncodeTypology: %v", err)
	}
	var captured rulesetport.RuleSetRef
	reader := &captureRuleReader{
		snapshot: &domain.RuleSetSnapshot{
			PayloadFormat: format,
			Payload:       payloadBytes,
		},
		captured: &captured,
	}
	catalog := NewRuleSetTypologyCatalog(reader)
	_, err = catalog.GetTypologyModelByRef(t.Context(), port.ModelRef{
		Algorithm: string(domain.AlgorithmBigFive),
		Code:      "BF",
		Version:   "1.0.0",
	})
	if err != nil {
		t.Fatalf("GetTypologyModelByRef: %v", err)
	}
	if captured.Kind != domain.KindPersonality || captured.Algorithm != domain.AlgorithmBigFive {
		t.Fatalf("captured ref = %#v", captured)
	}
}

type captureRuleReader struct {
	snapshot *domain.RuleSetSnapshot
	captured *rulesetport.RuleSetRef
}

func (c *captureRuleReader) GetPublishedByRef(_ context.Context, ref rulesetport.RuleSetRef) (*domain.RuleSetSnapshot, error) {
	if c.captured != nil {
		*c.captured = ref
	}
	return c.snapshot, nil
}

func (c *captureRuleReader) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.RuleSetSnapshot, error) {
	return nil, domain.ErrNotFound
}

func TestRuleSetTypologyCatalogFindByQuestionnaireDecodesV2Payload(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:                 "MBTI-16P",
		Version:              "1.0.0",
		Algorithm:            domain.AlgorithmMBTI,
		Status:               "published",
		QuestionnaireCode:    "Q-MBTI",
		QuestionnaireVersion: "1.0.0",
	}
	payloadBytes, format, err := codec.EncodeTypology(payload)
	if err != nil {
		t.Fatalf("EncodeTypology: %v", err)
	}
	reader := stubRuleReader{snapshot: &domain.RuleSetSnapshot{
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind:    domain.KindPersonality,
			Code:    payload.Code,
			Version: payload.Version,
		},
		Payload: payloadBytes,
	}}
	catalog := NewRuleSetTypologyCatalog(reader)
	got, err := catalog.FindTypologyModelByQuestionnaire(t.Context(), "Q-MBTI", "1.0.0")
	if err != nil {
		t.Fatalf("FindTypologyModelByQuestionnaire: %v", err)
	}
	if got.Algorithm != domain.AlgorithmMBTI || got.Code != payload.Code {
		t.Fatalf("payload = %#v", got)
	}
}

func TestRuleSetTypologyCatalogFindByQuestionnaireTypologyMBTI(t *testing.T) {
	model := &modeltypology.MBTILegacyModel{
		Code:                 personalityseed.MBTIModelCode,
		Version:              personalityseed.MBTIModelVersion,
		QuestionnaireCode:    personalityseed.MBTIQuestionnaireCode,
		QuestionnaireVersion: personalityseed.MBTIModelVersion,
		Status:               "published",
		DimensionOrder:       []string{"EI"},
		Dimensions: map[string]modeltypology.MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E"},
		},
		TypeProfiles: []modeltypology.MBTILegacyTypeProfile{
			{TypeCode: "INTJ", TypeName: "建筑师"},
		},
	}
	payloadBytes, format, err := codec.EncodeMBTI(model)
	if err != nil {
		t.Fatalf("EncodeMBTI: %v", err)
	}
	reader := stubRuleReader{snapshot: &domain.RuleSetSnapshot{
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind: domain.KindPersonality, Code: model.Code, Version: model.Version,
		},
		Payload: payloadBytes,
	}}
	catalog := NewRuleSetTypologyCatalog(reader)
	got, err := catalog.FindTypologyModelByQuestionnaire(t.Context(), model.QuestionnaireCode, model.QuestionnaireVersion)
	if err != nil {
		t.Fatalf("FindTypologyModelByQuestionnaire: %v", err)
	}
	if got.Algorithm != domain.AlgorithmMBTI {
		t.Fatalf("Algorithm = %s", got.Algorithm)
	}
}

func TestRepositoryResolverFindTypologyModelByQuestionnaire(t *testing.T) {
	payload := modeltypology.FromMBTI(&modeltypology.MBTILegacyModel{
		Code:                 "MBTI_TEST",
		Version:              "1.0.0",
		QuestionnaireCode:    "Q-MBTI",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
	})
	resolver, err := newResolver(nil, fakeTypologyCatalog{payload: payload})
	if err != nil {
		t.Fatalf("newResolver: %v", err)
	}
	got, err := resolver.FindTypologyModelByQuestionnaire(context.Background(), "Q-MBTI", "1.0.0")
	if err != nil {
		t.Fatalf("FindTypologyModelByQuestionnaire: %v", err)
	}
	if got.Code != "MBTI_TEST" {
		t.Fatalf("payload code = %s", got.Code)
	}
}

func TestRepositoryResolverFindTypologyModelByQuestionnaireRequiresCatalog(t *testing.T) {
	resolver, err := newResolver(nil, nil)
	if err != nil {
		t.Fatalf("newResolver: %v", err)
	}
	if _, err := resolver.FindTypologyModelByQuestionnaire(context.Background(), "Q-MBTI", "1.0.0"); err == nil {
		t.Fatal("expected error when typology catalog is not configured")
	}
}

type stubRuleReader struct {
	snapshot *domain.RuleSetSnapshot
}

func (s stubRuleReader) GetPublishedByRef(context.Context, rulesetport.RuleSetRef) (*domain.RuleSetSnapshot, error) {
	if s.snapshot == nil {
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}

func (s stubRuleReader) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.RuleSetSnapshot, error) {
	if s.snapshot == nil {
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}

type stubPublishedModelReader struct {
	snapshot *domain.PublishedModelSnapshot
	err      error
}

func (s stubPublishedModelReader) GetPublishedModelByRef(context.Context, rulesetport.Ref) (*domain.PublishedModelSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.snapshot == nil {
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}

func (s stubPublishedModelReader) FindPublishedModelByQuestionnaire(context.Context, string, string) (*domain.PublishedModelSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.snapshot == nil {
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}
