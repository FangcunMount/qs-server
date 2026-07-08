package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	cataloglegacy "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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
		want    port.RuleSetRef
	}{
		{
			name:    "sbti",
			code:    cataloglegacy.SBTIQuestionnaireCode,
			version: cataloglegacy.SBTIModelVersion,
			want: port.RuleSetRef{
				Kind:      domain.KindPersonality,
				SubKind:   domain.SubKindTypology,
				Algorithm: domain.AlgorithmSBTI,
				Code:      cataloglegacy.SBTIModelCode,
				Version:   cataloglegacy.SBTIModelVersion,
				Title:     cataloglegacy.SBTIModelTitle,
			},
		},
		{
			name:    "mbti",
			code:    cataloglegacy.MBTIQuestionnaireCode,
			version: cataloglegacy.MBTIModelVersion,
			want: port.RuleSetRef{
				Kind:      domain.KindPersonality,
				SubKind:   domain.SubKindTypology,
				Algorithm: domain.AlgorithmMBTI,
				Code:      cataloglegacy.MBTIModelCode,
				Version:   cataloglegacy.MBTIModelVersion,
				Title:     cataloglegacy.MBTIModelTitle,
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

	ref := port.RuleSetRef{
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKindTypology,
		Algorithm: domain.AlgorithmMBTI,
		Code:      cataloglegacy.MBTIModelCode,
		Version:   cataloglegacy.MBTIModelVersion,
	}
	snapshot, err := catalog.GetPublishedByRef(t.Context(), ref)
	if err != nil {
		t.Fatalf("GetPublishedByRef: %v", err)
	}
	if snapshot.DecisionKind != domain.DecisionKindPoleComposition {
		t.Fatalf("DecisionKind = %s, want %s", snapshot.DecisionKind, domain.DecisionKindPoleComposition)
	}
	if snapshot.Binding.QuestionnaireCode != cataloglegacy.MBTIQuestionnaireCode {
		t.Fatalf("binding code = %s", snapshot.Binding.QuestionnaireCode)
	}
	if len(snapshot.Payload) == 0 {
		t.Fatal("expected payload")
	}
}

func TestStaticCompositeCatalogResolveScaleBindingPropagatesError(t *testing.T) {
	wantErr := context.DeadlineExceeded
	catalog := NewStaticCompositeCatalog(nil, failingScaleBindingSource{err: wantErr})

	_, ok, err := catalog.ResolveByQuestionnaire(t.Context(), "QNR-SCALE", "1.0.0")
	if err != wantErr {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if ok {
		t.Fatal("expected ok=false")
	}
}

func TestStaticCompositeCatalogResolveScaleBindingNotFoundFallsThrough(t *testing.T) {
	catalog := NewStaticCompositeCatalog(nil, failingScaleBindingSource{err: domain.ErrNotFound})

	_, ok, err := catalog.ResolveByQuestionnaire(t.Context(), "QNR-SCALE", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveByQuestionnaire: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when scale source returns NotFound")
	}
}

type failingScaleBindingSource struct {
	err error
}

func (f failingScaleBindingSource) FindScaleByQuestionnaire(context.Context, string, string) (*scalesnapshot.ScaleSnapshot, error) {
	return nil, f.err
}

func (f failingScaleBindingSource) GetScaleByRef(context.Context, string, string) (*scalesnapshot.ScaleSnapshot, error) {
	return nil, f.err
}

func TestStaticCompositeCatalogResolveScaleBinding(t *testing.T) {
	scaleModel := &scalesnapshot.ScaleSnapshot{
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
	if ref.Kind != domain.RuleSetKindScale || ref.Code != "SCL-001" || ref.Version != "1.0.0" {
		t.Fatalf("ref = %#v", ref)
	}

	snapshot, err := catalog.GetPublishedByRef(t.Context(), ref)
	if err != nil {
		t.Fatalf("GetPublishedByRef: %v", err)
	}
	if snapshot.DecisionKind != domain.DecisionKindScoreRange {
		t.Fatalf("decision = %s", snapshot.DecisionKind)
	}
}

type stubScaleBindingSource struct {
	model *scalesnapshot.ScaleSnapshot
}

func (s stubScaleBindingSource) FindScaleByQuestionnaire(_ context.Context, questionnaireCode, questionnaireVersion string) (*scalesnapshot.ScaleSnapshot, error) {
	if s.model == nil {
		return nil, domain.ErrNotFound
	}
	if s.model.QuestionnaireCode == questionnaireCode && s.model.QuestionnaireVersion == questionnaireVersion {
		return s.model, nil
	}
	return nil, domain.ErrNotFound
}

func (s stubScaleBindingSource) GetScaleByRef(_ context.Context, code, version string) (*scalesnapshot.ScaleSnapshot, error) {
	if s.model == nil || s.model.Code != code || s.model.ScaleVersion != version {
		return nil, domain.ErrNotFound
	}
	return s.model, nil
}
