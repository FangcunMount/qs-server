package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleset"
)

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
	return nil, domain.ErrNotFound
}

func TestRuleSetSBTICatalogDecodesRulesetPayload(t *testing.T) {
	model := &port.SBTIModelSnapshot{
		Code:                 port.DefaultSBTIModelCode,
		Version:              port.DefaultSBTIModelVersion,
		QuestionnaireCode:    port.DefaultSBTIQuestionnaireCode,
		QuestionnaireVersion: port.DefaultSBTIModelVersion,
		Status:               "published",
	}
	payload, format, err := codec.EncodeSBTI(model)
	if err != nil {
		t.Fatalf("EncodeSBTI: %v", err)
	}
	reader := stubRuleReader{snapshot: &domain.RuleSetSnapshot{
		SchemaVersion: domain.RuleSetSchemaVersionV1,
		PayloadFormat: format,
		Definition: domain.RuleSetDefinition{
			Kind:    domain.RuleSetKindSBTI,
			Code:    model.Code,
			Version: model.Version,
		},
		Payload: payload,
	}}
	catalog := NewRuleSetSBTICatalog(reader)
	got, err := catalog.GetSBTIModelByRef(t.Context(), port.ModelRef{
		Code:    model.Code,
		Version: model.Version,
	})
	if err != nil {
		t.Fatalf("GetSBTIModelByRef: %v", err)
	}
	if got.Code != model.Code {
		t.Fatalf("Code = %s", got.Code)
	}
}

func TestRuleSetSBTICatalogRequiresVersion(t *testing.T) {
	catalog := NewRuleSetSBTICatalog(stubRuleReader{})
	_, err := catalog.GetSBTIModelByRef(t.Context(), port.ModelRef{Code: port.DefaultSBTIModelCode})
	if !domain.IsVersionRequired(err) {
		t.Fatalf("err = %v, want ErrVersionRequired", err)
	}
}
