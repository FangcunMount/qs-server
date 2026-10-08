package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"gorm.io/gorm/clause"
)

// BackfillCommittedEventEvidence borrows the caller's transaction. A baseline
// comparison covers every stored field; it cannot overwrite a different proof
// or attach a conclusion to facts changed since verification.
func BackfillCommittedEventEvidence(ctx context.Context, baseline EvaluationOutcomePO, proof *eventevidence.EventEvidenceV1) error {
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	// The host context lookup alone is insufficient: verify a real borrowed
	// SQL transaction without opening, committing or closing it.
	if _, err := sdkmysql.BindGORM(tx); err != nil {
		return err
	}
	if baseline.CommittedEventID != nil || baseline.CommittedEventEvidence != nil {
		return fmt.Errorf("backfill requires an unclassified baseline")
	}
	if proof == nil {
		return fmt.Errorf("committed event evidence is required")
	}
	candidate := baseline
	candidate.CommittedEventEvidence = proof.Clone()
	candidate.CommittedEventID = optionalString(proof.EventID)
	if _, err := outcomeFromPO(&candidate); err != nil {
		return err
	}
	body, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	query := tx.WithContext(ctx).Table("evaluation_outcome").Where("id=? AND committed_event_id IS NULL AND committed_event_evidence IS NULL", baseline.ID)
	// Compare original source bytes rather than database collation equality.
	for _, field := range []struct {
		name  string
		value any
	}{
		{"org_id", baseline.OrgID}, {"assessment_id", baseline.AssessmentID}, {"testee_id", baseline.TesteeID}, {"evaluation_run_id", baseline.EvaluationRunID},
		{"model_kind", baseline.ModelKind}, {"model_sub_kind", baseline.ModelSubKind}, {"model_algorithm", baseline.ModelAlgorithm}, {"model_code", baseline.ModelCode},
		{"model_version", baseline.ModelVersion}, {"model_title", baseline.ModelTitle}, {"decision_kind", baseline.DecisionKind}, {"input_snapshot_ref", baseline.InputSnapshotRef},
		{"report_input_json", baseline.ReportInputJSON}, {"payload_json", baseline.PayloadJSON}, {"schema_version", baseline.SchemaVersion}, {"evaluated_at", baseline.EvaluatedAt}, {"created_at", baseline.CreatedAt},
	} {
		switch v := field.value.(type) {
		case *string:
			if v == nil {
				query = query.Where(field.name + " IS NULL")
			} else {
				query = query.Where("CAST("+field.name+" AS BINARY)=?", []byte(*v))
			}
		case string:
			query = query.Where("CAST("+field.name+" AS BINARY)=?", []byte(v))
		default:
			query = query.Where(field.name+"=?", v)
		}
	}
	result := query.Updates(map[string]any{"committed_event_id": candidate.CommittedEventID, "committed_event_evidence": string(body)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var current EvaluationOutcomePO
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, baseline.ID).Error; err != nil {
		return err
	}
	if _, err := outcomeFromPO(&current); err != nil {
		return fmt.Errorf("committed event evidence CAS conflict: stored evidence or identity is invalid")
	}
	if !reflect.DeepEqual(current.CommittedEventID, candidate.CommittedEventID) {
		return fmt.Errorf("committed event evidence CAS conflict: stored event identity changed")
	}
	currentProof := current.CommittedEventEvidence
	current.CommittedEventID = nil
	current.CommittedEventEvidence = nil
	if reflect.DeepEqual(current, baseline) && reflect.DeepEqual(currentProof, proof) {
		return nil
	}
	return fmt.Errorf("committed event evidence CAS conflict: baseline or conclusion changed")
}
