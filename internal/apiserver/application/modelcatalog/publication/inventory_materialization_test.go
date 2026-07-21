package publication_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestInventoryAuditDetectsDefinitionHashMismatch(t *testing.T) {
	model := newPublishedTestModel(t)
	snapshot := &port.PublishedModel{Code: model.Code, Kind: model.Kind, Algorithm: model.Algorithm, AlgorithmFamily: domain.AlgorithmFamilyTaskPerformance, DecisionKind: domain.DecisionKindAbilityLevel, Version: "v3", DefinitionV2: model.DefinitionV2, Source: map[string]any{port.SourceDefinitionContentHash: "wrong"}}
	issues := publication.AuditPublishedSnapshotInventory(context.Background(), snapshot, snapshotHandler{})
	if len(issues) == 0 || issues[0].Rule != "definition.hash.mismatch" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestInventoryAuditDetectsRuntimeMaterializationFailure(t *testing.T) {
	model := newPublishedTestModel(t)
	snapshot := &port.PublishedModel{Code: model.Code, Kind: model.Kind, Version: "v3", DefinitionV2: model.DefinitionV2}
	issues := publication.AuditPublishedSnapshotInventory(context.Background(), snapshot, failingMaterializer{})
	if len(issues) == 0 || issues[len(issues)-1].Rule != "definition.runtime.invalid" {
		t.Fatalf("issues = %#v", issues)
	}
}

type failingMaterializer struct{}

func (failingMaterializer) Supports(domain.Identity) bool { return true }
func (failingMaterializer) ValidateForPublish(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue {
	return nil
}
func (failingMaterializer) MaterializeSnapshot(context.Context, *domain.AssessmentModel) (definition.Materialization, error) {
	return definition.Materialization{}, errors.New("invalid runtime")
}
