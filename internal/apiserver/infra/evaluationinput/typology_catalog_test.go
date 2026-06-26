package evaluationinput

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type fakeTypologyCatalog struct {
	payload *modeltypology.Payload
	err     error
}

func (f fakeTypologyCatalog) GetTypologyModelByRef(context.Context, port.ModelRef) (*modeltypology.Payload, error) {
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
		assessmentmodel.AlgorithmMBTI,
		fakeTypologyCatalog{payload: payload},
		fakeAnswerSheetReader{sheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    "MBTI_TEST",
			QuestionnaireVersion: "1.0.0",
		}},
		fakeQuestionnaireReader{},
	)
	if provider.EvaluatorKey() != evaldomain.EvaluatorKeyMBTI {
		t.Fatalf("key = %#v", provider.EvaluatorKey())
	}
	snapshot, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1})
	if err != nil {
		t.Fatalf("ResolveInput: %v", err)
	}
	got, ok := port.TypologyPayload(snapshot)
	if !ok || got.Algorithm != assessmentmodel.AlgorithmMBTI {
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
		assessmentmodel.AlgorithmMBTI,
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

func TestMBTIModelInputProviderDelegatesToTypologyProvider(t *testing.T) {
	payload := modeltypology.FromMBTI(&modeltypology.MBTILegacyModel{
		Code:                 "MBTI_TEST",
		Version:              "1.0.0",
		QuestionnaireCode:    "MBTI_TEST",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
	})
	provider := NewMBTIModelInputProvider(
		fakeMBTICatalog{payload: payload},
		fakeAnswerSheetReader{sheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    "MBTI_TEST",
			QuestionnaireVersion: "1.0.0",
		}},
		fakeQuestionnaireReader{},
	)
	snapshot, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1})
	if err != nil {
		t.Fatalf("ResolveInput: %v", err)
	}
	if _, ok := snapshot.ModelPayload.(port.TypologyModelPayload); !ok {
		t.Fatalf("payload type = %T, want TypologyModelPayload", snapshot.ModelPayload)
	}
}

type fakeMBTICatalog struct {
	payload *modeltypology.Payload
}

func (f fakeMBTICatalog) GetMBTIModelByRef(context.Context, port.ModelRef) (*modeltypology.MBTILegacyModel, error) {
	legacy, err := modeltypology.ToMBTI(f.payload)
	return legacy, err
}

func (fakeMBTICatalog) FindMBTIModelByQuestionnaire(context.Context, string, string) (*modeltypology.MBTILegacyModel, error) {
	return nil, nil
}
