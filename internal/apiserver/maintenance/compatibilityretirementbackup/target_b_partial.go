package compatibilityretirementbackup

import (
	"context"
	"encoding/hex"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"go.mongodb.org/mongo-driver/bson"
)

// These package-private values select a closed physical grammar. They are not
// caller-supplied permission modes. The ordinary reader retains its original
// completed-migration requirement. A SQL-only grammar never proves pair success.
type targetBJournalKind uint8

const (
	targetBJournalComplete targetBJournalKind = iota
	targetBJournalSQLOnly
)

func targetBJournalKindValid(kind targetBJournalKind) bool {
	return kind == targetBJournalComplete || kind == targetBJournalSQLOnly
}

func targetTransitionJournalKind(v *targetBMigrationTransition) targetBJournalKind {
	if v == nil {
		return targetBJournalComplete
	}
	return v.journalKind
}

// This accepts only an actual returned SQL success followed by no Mongo call.
// A client/network/dirty/unknown result is not converted to this finite state.
// Fixed Pair.Run's ordering makes 99/39 inconsistent with this batch's path.
func targetValidateSQLOnlyAttempt(result targetBMigrationResult) error {
	if result.State != "partial_or_unknown" || result.Observation != nil {
		return ErrRecoveryUnknown
	}
	a := result.Attempt
	if a.Completion != "partial_or_unknown" || !a.SQLAttempted || a.SQLResponse != "success" || a.SQLReturnedVersion != 100 || !a.SQLChanged || a.MongoAttempted || a.MongoResponse != "not_attempted" || a.MongoReturnedVersion != 0 || a.MongoChanged {
		return ErrRecoveryUnknown
	}
	return nil
}

func targetActualMigrationUUID(cols map[string]bson.Raw) (string, error) {
	raw, exists := cols["schema_migrations"]
	if !exists {
		return "", ErrIdentity
	}
	subtype, uuid, ok := raw.Lookup("info", "uuid").BinaryOK()
	if !ok || subtype != 4 || len(uuid) != 16 {
		return "", ErrIdentity
	}
	return hex.EncodeToString(uuid), nil
}

// This observes and registers the actual original private journal only. The
// resulting digest must still be independently approved before reconciliation.
// It supplies no proof of DROP, completion, writer fence or production permit.
func InspectTargetBSQLOnlyRecoveryJournal(ctx context.Context, a *Archive, r TargetRecoveryRequest, path string, w *fence.MaintenanceWindow) (TargetRecoveryJournalSummary, error) {
	return inspectTargetRecoveryJournalKind(ctx, a, r, path, w, targetBJournalSQLOnly)
}

// Recoverable here means a native bounded four-target recovery may be prepared:
// actual clean100/38, original Mongo generation/kept UUIDs, unchanged full schema,
// all four original DROP responses and absence/content reconciliation must pass.
// No migration/head/dirty writes occur. The fixed host must use the validated A
// recovery image with migration.enabled=false, and retain actual writer fencing.
// Unknown/dirty/in-flight cases need a separate real native observer, not an
// assertion or a clean-head inference; this entry refuses them deliberately.
func ReconcileTargetBSQLOnlyRecovery(ctx context.Context, a *Archive, b TargetRecoveryBorrowed, r TargetBRecoveryResumeRequest, dir string, w *fence.MaintenanceWindow) (*TargetRecoveryReconciliation, error) {
	return reconcileTargetBRecoveryKind(ctx, a, b, r, dir, w, targetBJournalSQLOnly)
}
