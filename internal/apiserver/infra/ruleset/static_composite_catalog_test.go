package ruleset

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	seedfixtures "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/seedfixtures"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestStaticCompositeCatalogResolveEmbeddedTypologyByQuestionnaire(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog()
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	ref, ok, err := catalog.ResolveByQuestionnaire(t.Context(), seedfixtures.SBTIQuestionnaireCode, seedfixtures.SBTIModelVersion)
	if err != nil || !ok {
		t.Fatalf("ResolveByQuestionnaire: ref=%#v ok=%v err=%v", ref, ok, err)
	}
	if ref.Kind != domain.KindTypology || ref.Algorithm != domain.AlgorithmSBTI || ref.Code != seedfixtures.SBTIModelCode {
		t.Fatalf("ref = %#v", ref)
	}
}

func TestStaticCompositeCatalogGetsEmbeddedTypologyByRef(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog()
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	snapshot, err := catalog.GetPublishedModelByRef(t.Context(), port.Ref{
		Kind: domain.KindTypology, SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmMBTI,
		Code: seedfixtures.MBTIModelCode, Version: seedfixtures.MBTIModelVersion,
	})
	if err != nil {
		t.Fatalf("GetPublishedModelByRef: %v", err)
	}
	if snapshot.DecisionKind != domain.DecisionKindPoleComposition || snapshot.QuestionnaireCode != seedfixtures.MBTIQuestionnaireCode {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
