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

// InventoryAuditIssue records a DefinitionV2 cutover finding.
type InventoryAuditIssue struct {
	Scope   string
	Code    string
	Field   string
	Rule    string
	Message string
}

// ModelFromPublishedSnapshot builds a minimal model for runtime materialization.
func ModelFromPublishedSnapshot(snapshot *port.PublishedModel) *domain.AssessmentModel {
	if snapshot == nil {
		return nil
	}
	return &domain.AssessmentModel{
		Code:      snapshot.Code,
		Kind:      snapshot.Kind,
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

// AuditPublishedSnapshotInventory checks DefinitionV2 hash and runtime materializability.
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
		return append(issues, inventoryIssue("published", snapshot.Code, "definition_v2", "definition_v2.required", "DefinitionV2 is required for inventory validation"))
	}
	storedDefHash := port.DefinitionHashFromSource(snapshot.Source)
	defHash, err := modeldefinition.CanonicalContentHash(snapshot.DefinitionV2)
	if err != nil {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "definition", "definition.content_hash.failed", err.Error()))
	} else if storedDefHash == "" {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "source.definition_content_hash", "definition.hash.missing", "definition content hash is missing"))
	} else if storedDefHash != defHash {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "source.definition_content_hash", "definition.hash.mismatch", "stored definition hash does not match current DefinitionV2"))
	}
	if handler == nil {
		return append(issues, inventoryIssue("published", snapshot.Code, "handler", "handler.missing", "definition handler is unavailable for runtime materialization"))
	}
	model := ModelFromPublishedSnapshot(snapshot)
	materialized, err := handler.MaterializeSnapshot(ctx, model)
	if err != nil {
		return append(issues, inventoryIssue("published", snapshot.Code, "definition_v2", "definition.runtime.invalid", err.Error()))
	}
	if materialized.AlgorithmFamily != snapshot.AlgorithmFamily || materialized.DecisionKind != snapshot.DecisionKind {
		issues = append(issues, inventoryIssue("published", snapshot.Code, "runtime_identity", "runtime.identity.mismatch", "stored runtime identity does not match DefinitionV2 materialization"))
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
