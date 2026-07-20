package publication_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestAuditPublishedSnapshotInventoryDetectsMissingHashes(t *testing.T) {
	t.Parallel()
	snapshot := &port.PublishedModel{
		Code: "SPM", Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM,
		Version: "v1", Payload: []byte(`{"x":1}`),
		DefinitionV2: &modeldefinition.Definition{},
		Source:       map[string]any{},
	}
	issues := publication.AuditPublishedSnapshotInventory(context.Background(), snapshot, snapshotHandler{})
	if len(issues) == 0 {
		t.Fatal("expected missing hash issues")
	}
	foundMissing := false
	for _, issue := range issues {
		if issue.Rule == "projection.hash.missing" {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestModelFromPublishedSnapshotPreservesBinding(t *testing.T) {
	t.Parallel()
	snapshot := &port.PublishedModel{
		Code: "PHQ9", Version: "v3", Kind: domain.KindScale,
		QuestionnaireCode: "Q-PHQ9", QuestionnaireVersion: "1.0.0",
	}
	model := publication.ModelFromPublishedSnapshot(snapshot)
	if model == nil || model.Binding.QuestionnaireCode != "Q-PHQ9" || model.Revision() != 3 {
		t.Fatalf("model = %#v revision=%d", model, model.Revision())
	}
}
