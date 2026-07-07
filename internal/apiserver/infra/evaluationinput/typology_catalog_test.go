package evaluationinput

import (
	"context"
	"testing"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type fakeTypologyCatalog struct {
	payload *modeltypology.Payload
	err     error
}

func (f fakeTypologyCatalog) GetTypologyModelByRef(context.Context, port.ModelRef) (*modeltypology.Payload, error) {
	return f.payload, f.err
}

func (f fakeTypologyCatalog) FindTypologyModelByQuestionnaire(context.Context, string, string) (*modeltypology.Payload, error) {
	return f.payload, f.err
}

type fakeAnswerSheetReader struct {
	sheet *port.AnswerSheetSnapshot
}

func (f fakeAnswerSheetReader) GetAnswerSheet(context.Context, uint64) (*port.AnswerSheetSnapshot, error) {
	return f.sheet, nil
}

type fakeQuestionnaireReader struct{}

func (fakeQuestionnaireReader) GetQuestionnaire(context.Context, string, string) (*port.QuestionnaireSnapshot, error) {
	return &port.QuestionnaireSnapshot{Code: "MBTI_TEST", Version: "1.0.0"}, nil
}

func TestTypologyModelInputProviderReturnsTypologyPayload(t *testing.T) {
	payload := modeltypology.FromMBTI(&modeltypology.MBTILegacyModel{
		Code:                 "MBTI_TEST",
		Version:              "1.0.0",
		QuestionnaireCode:    "MBTI_TEST",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
	})
	provider := NewTypologyModelInputProvider(
		modelcatalog.AlgorithmMBTI,
		fakeTypologyCatalog{payload: payload},
		fakeAnswerSheetReader{sheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    "MBTI_TEST",
			QuestionnaireVersion: "1.0.0",
		}},
		fakeQuestionnaireReader{},
	)
	if provider.ExecutionIdentity() != evaldomain.ExecutionIdentityMBTI {
		t.Fatalf("key = %#v", provider.ExecutionIdentity())
	}
	snapshot, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1})
	if err != nil {
		t.Fatalf("ResolveInput: %v", err)
	}
	got, ok := port.TypologyPayload(snapshot)
	if !ok || got.Algorithm != modelcatalog.AlgorithmMBTI {
		t.Fatalf("payload = %#v, ok=%v", got, ok)
	}
}

func TestTypologyModelInputProviderRejectsAlgorithmMismatch(t *testing.T) {
	payload := modeltypology.FromSBTI(&modeltypology.SBTILegacyModel{
		Code:                 "SBTI_FUN",
		Version:              "1.0.0",
		QuestionnaireCode:    "SBTI_FUN",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
	})
	provider := NewTypologyModelInputProvider(
		modelcatalog.AlgorithmMBTI,
		fakeTypologyCatalog{payload: payload},
		fakeAnswerSheetReader{sheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    "SBTI_FUN",
			QuestionnaireVersion: "1.0.0",
		}},
		fakeQuestionnaireReader{},
	)
	_, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1})
	if err == nil {
		t.Fatal("ResolveInput error = nil, want algorithm mismatch")
	}
}

func TestTypologyProviderResolvesBigFivePayload(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:      "BF",
		Version:   "1.0.0",
		Algorithm: modelcatalog.AlgorithmBigFive,
		Status:    "published",
	}
	provider := NewTypologyModelInputProvider(
		modelcatalog.AlgorithmBigFive,
		fakeTypologyCatalog{payload: payload},
		fakeAnswerSheetReader{sheet: &port.AnswerSheetSnapshot{}},
		fakeQuestionnaireReader{},
	)
	snapshot, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1})
	if err != nil {
		t.Fatalf("ResolveInput: %v", err)
	}
	got, ok := port.TypologyPayload(snapshot)
	if !ok || got.Algorithm != modelcatalog.AlgorithmBigFive {
		t.Fatalf("payload = %#v, ok=%v", got, ok)
	}
}
