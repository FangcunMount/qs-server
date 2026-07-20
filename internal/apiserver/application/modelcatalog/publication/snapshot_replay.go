package publication

import (
	"context"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// InventoryAuditIssue records a published snapshot projection/hash finding.
type InventoryAuditIssue struct {
	Scope   string
	Code    string
	Field   string
	Rule    string
	Message string
}

// ModelFromPublishedSnapshot builds a minimal draft model for deterministic replay.
func ModelFromPublishedSnapshot(snapshot *port.PublishedModel) *domain.AssessmentModel {
	if snapshot == nil {
		return nil
	}
	return &domain.AssessmentModel{
		Code:      snapshot.Code,
		Kind:      snapshot.Kind,
		SubKind:   snapshot.SubKind,
		Algorithm: snapshot.Algorithm,
		Title:     snapshot.Title,
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    snapshot.QuestionnaireCode,
			QuestionnaireVersion: snapshot.QuestionnaireVersion,
		},
		DefinitionV2: snapshot.DefinitionV2,
		Version:      revisionFromVersion(snapshot.Version),
	}
}

// AuditPublishedSnapshotInventory checks stored projection hashes and replay drift.
func AuditPublishedSnapshotInventory(
	ctx context.Context,
	snapshot *port.PublishedModel,
	handler definition.Handler,
) []InventoryAuditIssue {
	if snapshot == nil {
		return []InventoryAuditIssue{inventoryIssue("published", "", "model", "model.required", "published snapshot is nil")}
	}
	issues := make([]InventoryAuditIssue, 0)
	if snapshot.DefinitionV2 == nil {
		return append(issues, inventoryIssue("published", snapshot.Code, "definition_v2", "definition_v2.required", "DefinitionV2 is required for projection inventory"))
	}
	storedDefHash, storedPayloadHash := port.ProjectionHashesFromSource(snapshot.Source)
	payloadHash := modeldefinition.PayloadProjectionHash(snapshot.Payload)
	if storedPayloadHash == "" {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "source.payload_projection_hash", "projection.hash.missing", "payload projection hash is missing"))
	} else if storedPayloadHash != payloadHash {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "source.payload_projection_hash", "projection.hash.payload_mismatch", "stored payload hash does not match current payload bytes"))
	}
	defHash, err := modeldefinition.CanonicalContentHash(snapshot.DefinitionV2)
	if err != nil {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "definition", "definition.content_hash.failed", err.Error()))
	} else if storedDefHash == "" {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "source.definition_content_hash", "projection.hash.missing", "definition content hash is missing"))
	} else if storedDefHash != defHash {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "source.definition_content_hash", "projection.hash.definition_mismatch", "stored definition hash does not match current DefinitionV2"))
	}
	if handler == nil {
		return append(issues, inventoryIssue("published", snapshot.Code, "handler", "handler.missing", "definition handler is unavailable for replay audit"))
	}
	model := ModelFromPublishedSnapshot(snapshot)
	for _, issue := range AuditSnapshotProjection(ctx, model, handler, snapshot) {
		issues = append(issues, inventoryIssue("published", snapshot.Code, issue.Field, issue.Code, issue.Message))
	}
	return issues
}

func inventoryIssue(scope, code, field, rule, message string) InventoryAuditIssue {
	return InventoryAuditIssue{Scope: scope, Code: code, Field: field, Rule: rule, Message: message}
}

func revisionFromVersion(version string) int64 {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" {
		return 0
	}
	rev, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		return 0
	}
	return rev
}
