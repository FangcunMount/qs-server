package evaluationinput

import (
	"context"
	"errors"
	"testing"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
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

func TestConfiguredTypologyModelInputProviderReturnsTypologyPayload(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:                 "MBTI_TEST",
		Version:              "1.0.0",
		QuestionnaireCode:    "MBTI_TEST",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Algorithm:            modelcatalog.AlgorithmPersonalityTypology,
	}
	provider := NewConfiguredTypologyModelInputProvider(
		fakeTypologyCatalog{payload: payload},
		nil,
		fakeAnswerSheetReader{sheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    "MBTI_TEST",
			QuestionnaireVersion: "1.0.0",
		}},
		fakeQuestionnaireReader{},
	)
	if provider.ExecutionIdentity() != evaldomain.ExecutionIdentityPersonalityTypology {
		t.Fatalf("key = %#v", provider.ExecutionIdentity())
	}
	snapshot, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1})
	if err != nil {
		t.Fatalf("ResolveInput: %v", err)
	}
	got, ok := port.TypologyPayload(snapshot)
	if !ok || got.Algorithm != modelcatalog.AlgorithmPersonalityTypology {
		t.Fatalf("payload = %#v, ok=%v", got, ok)
	}
}

func TestConfiguredTypologyModelInputProviderRejectsAlgorithmMismatch(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:                 "SBTI_FUN",
		Version:              "1.0.0",
		QuestionnaireCode:    "SBTI_FUN",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Algorithm:            modelcatalog.Algorithm("unsupported"),
	}
	provider := NewConfiguredTypologyModelInputProvider(
		fakeTypologyCatalog{payload: payload},
		nil,
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

func TestConfiguredTypologyProviderResolvesBigFivePayload(t *testing.T) {
	payload := &modeltypology.Payload{
		Code:      "BF",
		Version:   "1.0.0",
		Algorithm: modelcatalog.AlgorithmPersonalityTypology,
		Status:    "published",
	}
	provider := NewConfiguredTypologyModelInputProvider(
		fakeTypologyCatalog{payload: payload},
		nil,
		fakeAnswerSheetReader{sheet: &port.AnswerSheetSnapshot{}},
		fakeQuestionnaireReader{},
	)
	snapshot, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1})
	if err != nil {
		t.Fatalf("ResolveInput: %v", err)
	}
	got, ok := port.TypologyPayload(snapshot)
	if !ok || got.Algorithm != modelcatalog.AlgorithmPersonalityTypology {
		t.Fatalf("payload = %#v, ok=%v", got, ok)
	}
}

func TestConfiguredTypologyProviderPreservesResolveFailures(t *testing.T) {
	tests := []struct {
		name            string
		catalog         port.TypologyModelCatalog
		sheet           *port.AnswerSheetSnapshot
		kind            port.FailureKind
		message, reason string
		retryable       bool
	}{
		{name: "catalog absent", kind: port.FailureKindModelNotFound, message: "人格模型不存在", reason: "加载解释模型失败: typology model catalog is not configured"},
		{name: "model absent", catalog: fakeTypologyCatalog{err: modelcatalog.ErrNotFound}, kind: port.FailureKindModelNotFound, message: "人格模型不存在", reason: "加载解释模型失败: " + modelcatalog.ErrNotFound.Error()},
		{name: "catalog unavailable", catalog: fakeTypologyCatalog{err: context.DeadlineExceeded}, kind: port.FailureKindDependencyUnavailable, message: "加载解释模型依赖失败", reason: "加载解释模型失败: context deadline exceeded", retryable: true},
		{name: "payload absent", catalog: fakeTypologyCatalog{}, kind: port.FailureKindModelNotFound, message: "人格模型不存在", reason: "加载解释模型失败: typology model payload is nil"},
		{name: "algorithm mismatch", catalog: fakeTypologyCatalog{payload: &modeltypology.Payload{Algorithm: "unsupported"}}, kind: port.FailureKindUnsupportedModel, message: "不支持的解释模型", reason: "加载解释模型失败: typology algorithm unsupported does not match provider personality_typology"},
		{name: "model unpublished", catalog: fakeTypologyCatalog{payload: &modeltypology.Payload{Code: "MODEL", Algorithm: modelcatalog.AlgorithmPersonalityTypology, Status: "draft"}}, kind: port.FailureKindModelNotFound, message: "人格模型不可用", reason: "加载解释模型失败: typology model is not published: MODEL"},
		{name: "questionnaire mismatch", catalog: fakeTypologyCatalog{payload: &modeltypology.Payload{Algorithm: modelcatalog.AlgorithmPersonalityTypology, Status: "published", QuestionnaireCode: "Q", QuestionnaireVersion: "1"}}, sheet: &port.AnswerSheetSnapshot{QuestionnaireCode: "Q", QuestionnaireVersion: "2"}, kind: port.FailureKindQuestionnaireVersionMismatch, message: "问卷版本不匹配", reason: "加载问卷失败: answersheet questionnaire Q@2 does not match typology model questionnaire Q@1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewConfiguredTypologyModelInputProvider(tc.catalog, nil, fakeAnswerSheetReader{sheet: tc.sheet}, fakeQuestionnaireReader{})
			_, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1})
			var failure *port.ResolveError
			if !errors.As(err, &failure) {
				t.Fatalf("ResolveInput = %T %v, want ResolveError", err, err)
			}
			if failure.FailureKind() != tc.kind || failure.Error() != tc.message || failure.FailureReason() != tc.reason || failure.Retryable() != tc.retryable {
				t.Fatalf("failure = kind:%s message:%q reason:%q retryable:%t", failure.FailureKind(), failure.Error(), failure.FailureReason(), failure.Retryable())
			}
			if tc.retryable {
				assertModelCatalogDependency(t, err)
			}
		})
	}
}

type typologyPublishedReaderStub struct {
	ref   rulesetport.Ref
	model *rulesetport.PublishedModel
	err   error
}

func (s *typologyPublishedReaderStub) GetPublishedModelByRef(_ context.Context, ref rulesetport.Ref) (*rulesetport.PublishedModel, error) {
	s.ref = ref
	return s.model, s.err
}

func (s *typologyPublishedReaderStub) FindPublishedModelByQuestionnaire(context.Context, string, string) (*rulesetport.PublishedModel, error) {
	return s.model, s.err
}

func TestConfiguredTypologyProviderPreservesCanonicalAttach(t *testing.T) {
	definition := &modelcatalog.Definition{}
	for _, tc := range []struct {
		name  string
		model *rulesetport.PublishedModel
		err   error
	}{
		{name: "canonical definition", model: &rulesetport.PublishedModel{DefinitionV2: definition}},
		{name: "canonical absent", err: modelcatalog.ErrNotFound},
		{name: "canonical unavailable", err: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &typologyPublishedReaderStub{model: tc.model, err: tc.err}
			provider := NewConfiguredTypologyModelInputProvider(
				fakeTypologyCatalog{payload: &modeltypology.Payload{Code: "MODEL", Version: "1", Algorithm: modelcatalog.AlgorithmPersonalityTypology, Status: "published"}},
				reader, fakeAnswerSheetReader{sheet: &port.AnswerSheetSnapshot{}}, fakeQuestionnaireReader{},
			)
			snapshot, err := provider.ResolveInput(context.Background(), port.InputRef{AnswerSheetID: 1, ModelRef: port.ModelRef{Code: "MODEL", Version: "1"}})
			wantRef := rulesetport.Ref{Kind: modelcatalog.KindTypology, SubKind: modelcatalog.SubKindTypology, Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "MODEL", Version: "1"}
			if reader.ref != wantRef {
				t.Fatalf("canonical lookup = %#v, want %#v", reader.ref, wantRef)
			}
			if errors.Is(tc.err, context.DeadlineExceeded) {
				assertModelCatalogDependency(t, err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, attached := port.DefinitionV2FromSnapshot(snapshot)
			if tc.model != nil && (!attached || got != definition) {
				t.Fatal("canonical definition was not attached")
			}
			if tc.model == nil && attached {
				t.Fatal("missing canonical definition was fabricated")
			}
		})
	}
}
