package publication_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestAuditSnapshotProjectionDetectsPayloadDrift(t *testing.T) {
	t.Parallel()
	model := newPublishedTestModel(t)
	model.DefinitionV2 = completeScaleDefinitionForPublishTest()
	snapshot := &port.AssessmentSnapshot{Payload: []byte(`{"a":1}`)}
	handler := &driftReplayHandler{}
	issues := publication.AuditSnapshotProjection(context.Background(), model, handler, snapshot)
	if len(issues) == 0 {
		t.Fatal("expected projection drift issue")
	}
	if issues[0].Code != "payload.projection.drift" {
		t.Fatalf("issue code = %q, want payload.projection.drift", issues[0].Code)
	}
}

func TestAuditSnapshotProjectionPassesDeterministicReplay(t *testing.T) {
	t.Parallel()
	model := newPublishedTestModel(t)
	model.DefinitionV2 = completeScaleDefinitionForPublishTest()
	payload := []byte(`{"dimensions":[{"code":"total"}]}`)
	snapshot := &port.AssessmentSnapshot{Payload: append([]byte(nil), payload...)}
	if issues := publication.AuditSnapshotProjection(context.Background(), model, snapshotHandler{}, snapshot); len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestPublisherPublishFailsOnProjectionDrift(t *testing.T) {
	t.Parallel()
	model := newPublishedTestModel(t)
	model.DefinitionV2 = completeScaleDefinitionForPublishTest()
	publisher := publication.Publisher{
		Registry:  definition.NewRegistry(&driftReplayHandler{}),
		ModelRepo: &publishedModelRepo{},
		Repo:      &publishedRepo{},
	}
	if _, err := publisher.Publish(context.Background(), model, publication.PublishOptions{}); err == nil {
		t.Fatal("Publish() error = nil, want projection drift")
	} else {
		var validationErr *definition.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("Publish() error = %v, want ValidationError", err)
		}
		if len(validationErr.Issues) == 0 || validationErr.Issues[0].Code != "payload.projection.drift" {
			t.Fatalf("issues = %#v", validationErr.Issues)
		}
	}
}

type driftReplayHandler struct {
	calls int
}

func (driftReplayHandler) Supports(domain.Identity) bool { return true }

func (driftReplayHandler) ValidateForPublish(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue {
	return nil
}

func (h *driftReplayHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (definition.SnapshotBuildResult, error) {
	h.calls++
	payload := []byte(`{"first":true}`)
	if h.calls > 1 {
		payload = []byte(`{"drift":true}`)
	}
	return definition.SnapshotBuildResult{
		Kind:          domain.KindCognitive,
		Algorithm:     domain.AlgorithmSPM,
		PayloadFormat: domain.PayloadFormatCognitiveDefaultV1,
		DecisionKind:  domain.DecisionKindAbilityLevel,
		Payload:       payload,
	}, nil
}

func TestAttachProjectionHashesUsesDeterministicPayload(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"x":1}`)
	hash := modeldefinition.PayloadProjectionHash(payload)
	snapshot := &port.AssessmentSnapshot{Payload: payload}
	port.AttachProjectionHashes(snapshot, "def-hash", hash)
	if snapshot.Source[port.SourcePayloadProjectionHash] != hash {
		t.Fatalf("hash = %q, want %q", snapshot.Source[port.SourcePayloadProjectionHash], hash)
	}
}
