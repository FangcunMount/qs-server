package evaluationinput

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/interpretationmodel/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	interpretationmodelport "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

type stubRuleReader struct {
	snapshot *domain.RuleSetSnapshot
}

func (s stubRuleReader) GetPublishedByRef(context.Context, interpretationmodelport.ModelRef) (*domain.RuleSetSnapshot, error) {
	if s.snapshot == nil {
		return nil, domain.ErrNotFound
	}
	return s.snapshot, nil
}

func (s stubRuleReader) FindPublishedByQuestionnaire(context.Context, string, string) (*domain.RuleSetSnapshot, error) {
	return nil, domain.ErrNotFound
}

func TestInterpretationSBTICatalogDecodesRulesetPayload(t *testing.T) {
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
		Definition: domain.ModelDefinition{
			Kind:    domain.ModelKindSBTI,
			Code:    model.Code,
			Version: model.Version,
		},
		Payload: payload,
	}}
	catalog := NewInterpretationSBTICatalog(reader)
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

func TestInterpretationSBTICatalogRequiresVersion(t *testing.T) {
	catalog := NewInterpretationSBTICatalog(stubRuleReader{})
	_, err := catalog.GetSBTIModelByRef(t.Context(), port.ModelRef{Code: port.DefaultSBTIModelCode})
	if !domain.IsVersionRequired(err) {
		t.Fatalf("err = %v, want ErrVersionRequired", err)
	}
}
