package interpretationmodel

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	evaluationinputPort "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

func TestStaticCompositeCatalogResolveByQuestionnaire(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}

	tests := []struct {
		name    string
		code    string
		version string
		want    port.ModelRef
	}{
		{
			name:    "sbti",
			code:    evaluationinputPort.DefaultSBTIQuestionnaireCode,
			version: evaluationinputPort.DefaultSBTIModelVersion,
			want: port.ModelRef{
				Kind:    domain.ModelKindSBTI,
				Code:    evaluationinputPort.DefaultSBTIModelCode,
				Version: evaluationinputPort.DefaultSBTIModelVersion,
				Title:   evaluationinputPort.DefaultSBTIModelTitle,
			},
		},
		{
			name:    "mbti",
			code:    evaluationinputPort.DefaultMBTIQuestionnaireCode,
			version: evaluationinputPort.DefaultMBTIModelVersion,
			want: port.ModelRef{
				Kind:    domain.ModelKindMBTI,
				Code:    evaluationinputPort.DefaultMBTIModelCode,
				Version: evaluationinputPort.DefaultMBTIModelVersion,
				Title:   evaluationinputPort.DefaultMBTIModelTitle,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := catalog.ResolveByQuestionnaire(t.Context(), tt.code, tt.version)
			if err != nil {
				t.Fatalf("ResolveByQuestionnaire: %v", err)
			}
			if !ok {
				t.Fatal("expected binding")
			}
			if got != tt.want {
				t.Fatalf("ref = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestStaticCompositeCatalogGetPublishedByRef(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}

	ref := port.ModelRef{
		Kind:    domain.ModelKindMBTI,
		Code:    evaluationinputPort.DefaultMBTIModelCode,
		Version: evaluationinputPort.DefaultMBTIModelVersion,
	}
	snapshot, err := catalog.GetPublishedByRef(t.Context(), ref)
	if err != nil {
		t.Fatalf("GetPublishedByRef: %v", err)
	}
	if snapshot.DecisionKind != domain.DecisionKindPoleComposition {
		t.Fatalf("DecisionKind = %s, want %s", snapshot.DecisionKind, domain.DecisionKindPoleComposition)
	}
	if snapshot.Binding.QuestionnaireCode != evaluationinputPort.DefaultMBTIQuestionnaireCode {
		t.Fatalf("binding code = %s", snapshot.Binding.QuestionnaireCode)
	}
	if len(snapshot.Payload) == 0 {
		t.Fatal("expected payload")
	}
}

func TestStaticCompositeCatalogResolveScaleBinding(t *testing.T) {
	scaleModel := &evaluationinputPort.ScaleSnapshot{
		Code:                 "SCL-001",
		ScaleVersion:         "1.0.0",
		Title:                "Demo Scale",
		QuestionnaireCode:    "QNR-SCALE",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
	}
	catalog := NewStaticCompositeCatalog(nil, stubScaleBindingSource{model: scaleModel})

	ref, ok, err := catalog.ResolveByQuestionnaire(t.Context(), "QNR-SCALE", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveByQuestionnaire: %v", err)
	}
	if !ok {
		t.Fatal("expected scale binding")
	}
	if ref.Kind != domain.ModelKindScale || ref.Code != "SCL-001" || ref.Version != "1.0.0" {
		t.Fatalf("ref = %#v", ref)
	}

	snapshot, err := catalog.GetPublishedByRef(t.Context(), ref)
	if err != nil {
		t.Fatalf("GetPublishedByRef: %v", err)
	}
	if snapshot.DecisionKind != domain.DecisionKindScoreRangeInterpretation {
		t.Fatalf("decision = %s", snapshot.DecisionKind)
	}
}

type stubScaleBindingSource struct {
	model *evaluationinputPort.ScaleSnapshot
}

func (s stubScaleBindingSource) FindScaleByQuestionnaire(_ context.Context, questionnaireCode, questionnaireVersion string) (*evaluationinputPort.ScaleSnapshot, error) {
	if s.model == nil {
		return nil, domain.ErrNotFound
	}
	if s.model.QuestionnaireCode == questionnaireCode && s.model.QuestionnaireVersion == questionnaireVersion {
		return s.model, nil
	}
	return nil, domain.ErrNotFound
}

func (s stubScaleBindingSource) GetScaleByRef(_ context.Context, code, version string) (*evaluationinputPort.ScaleSnapshot, error) {
	if s.model == nil || s.model.Code != code || s.model.ScaleVersion != version {
		return nil, domain.ErrNotFound
	}
	return s.model, nil
}
