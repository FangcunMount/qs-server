package operatorretirement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/operatorretirement"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ port.RecoveryRepository = (*Repository)(nil)

type recoveryPO struct {
	RequestID                                                                                string
	OperatorID                                                                               uint64
	OrgID, UserID                                                                            int64
	RetirementRequestID                                                                      string
	RecoveryPayload, RetirementPayload, RetirementBeforeState, OperatorBeforeState, Checksum string
	CreatedAt                                                                                time.Time
}

func (recoveryPO) TableName() string { return "operator_recovery_archives" }
func (p recoveryPO) checksum() string {
	// Length-delimited JSON avoids ambiguous concatenation and preserves the exact archived bytes.
	raw, _ := json.Marshal([]string{p.RecoveryPayload, p.RetirementPayload, p.RetirementBeforeState, p.OperatorBeforeState})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (r *Repository) FindRecovery(ctx context.Context, id string) (*domain.Recovery, error) {
	var p recoveryPO
	if err := r.dbFor(ctx).Where("request_id=?", id).Take(&p).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		// Read-only preflight is supported immediately before the compatible archive migration.
		// Missing storage after version79, or a dirty/unknown schema, is never treated as an empty archive.
		var mysqlErr *driver.MySQLError
		if stderrors.As(err, &mysqlErr) && mysqlErr.Number == 1146 {
			var schema struct {
				Version uint64
				Dirty   bool
			}
			if e := r.dbFor(ctx).Table("schema_migrations").Take(&schema).Error; e != nil {
				return nil, e
			}
			if schema.Version == 78 && !schema.Dirty {
				return nil, nil
			}
		}
		return nil, err
	}
	if p.Checksum != p.checksum() {
		return nil, fmt.Errorf("corrupt operator recovery archive")
	}
	var value domain.Recovery
	if err := json.Unmarshal([]byte(p.RecoveryPayload), &value); err != nil {
		return nil, err
	}
	if err := value.Validate(); err != nil {
		return nil, err
	}
	var task domain.Task
	if err := json.Unmarshal([]byte(p.RetirementPayload), &task); err != nil {
		return nil, err
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}
	if value.RequestID != p.RequestID || value.OperatorID != p.OperatorID || value.OrgID != p.OrgID || value.UserID != p.UserID || value.RetirementRequestID != p.RetirementRequestID || task.RequestID != value.RetirementRequestID || task.Stage != domain.Completed || task.OperatorID != value.OperatorID || task.OrgID != value.OrgID || task.UserID != value.UserID || value.ExpectedVersion != task.ExpectedVersion+2 || value.PolicyVersion < task.PolicyVersion || !json.Valid([]byte(p.RetirementBeforeState)) || !json.Valid([]byte(p.OperatorBeforeState)) {
		return nil, fmt.Errorf("operator recovery archive identity mismatch")
	}
	return &value, nil
}

// Recover is atomic, and is only available on the final operator schema.
// It intentionally does not restore any IAM grants, clinician bindings or QR state.
func (r *Repository) Recover(ctx context.Context, value domain.Recovery) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if r.table != operatorTable {
		return fmt.Errorf("operator recovery requires final schema")
	}
	if held, ok := ctx.Value(userLockKey{}).(int64); !ok || held != value.UserID {
		return fmt.Errorf("operator recovery requires user exclusion")
	}
	return r.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var task taskPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operator_id=? AND org_id=?", value.OperatorID, value.OrgID).Take(&task).Error; err != nil {
			return err
		}
		var exit domain.Task
		if err := json.Unmarshal([]byte(task.Payload), &exit); err != nil {
			return err
		}
		expected, err := domain.NewRecovery(exit, value.ActorID, value.ExpectedVersion, value.PolicyVersion, value.RequestID, value.Reason, value.CreatedAt)
		if err != nil {
			return err
		}
		if expected != value || task.UserID != value.UserID || task.Stage != string(domain.Completed) {
			return fmt.Errorf("retirement changed before recovery")
		}
		var before map[string]interface{}
		if err := tx.Table(r.table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND org_id=? AND user_id=? AND version=? AND is_active=FALSE AND deleted_at IS NOT NULL", value.OperatorID, value.OrgID, value.UserID, value.ExpectedVersion).Take(&before).Error; err != nil {
			return err
		}
		var others int64
		if err := tx.Table(r.table).Where("user_id=? AND id<>? AND deleted_at IS NULL", value.UserID, value.OperatorID).Count(&others).Error; err != nil {
			return err
		}
		if others != 0 {
			return fmt.Errorf("other operator membership prevents recovery")
		}
		// An authenticated HQ snapshot is checked by the application. Its local identity must still be active.
		var actors int64
		if err := tx.Table(r.table).Where("org_id=? AND user_id=? AND is_active=TRUE AND deleted_at IS NULL", value.OrgID, value.ActorID).Count(&actors).Error; err != nil {
			return err
		}
		if actors != 1 {
			return fmt.Errorf("active company administrator required")
		}
		var blocking int64
		if err := tx.Model(&taskPO{}).Where("user_id=?", value.ActorID).Count(&blocking).Error; err != nil {
			return err
		}
		if blocking != 0 {
			return fmt.Errorf("administrator is retiring")
		}
		raw, err := json.Marshal(before)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		archive := recoveryPO{RequestID: value.RequestID, OperatorID: value.OperatorID, OrgID: value.OrgID, UserID: value.UserID, RetirementRequestID: value.RetirementRequestID, RecoveryPayload: string(payload), RetirementPayload: task.Payload, RetirementBeforeState: task.BeforeState, OperatorBeforeState: string(raw), CreatedAt: value.CreatedAt}
		if !json.Valid([]byte(task.BeforeState)) {
			return fmt.Errorf("invalid original retirement facts")
		}
		archive.Checksum = archive.checksum()
		if err := tx.Create(&archive).Error; err != nil {
			return err
		}
		result := tx.Table(r.table).Where("id=? AND org_id=? AND user_id=? AND version=? AND is_active=FALSE AND deleted_at IS NOT NULL", value.OperatorID, value.OrgID, value.UserID, value.ExpectedVersion).
			Updates(map[string]interface{}{"is_active": true, "deleted_at": nil, "roles": "[]", "effective_roles": "[]", "authz_policy_version": value.PolicyVersion, "authz_projection_pending": false, "authz_projected_at": value.CreatedAt, "version": gorm.Expr("version+1"), "updated_at": value.CreatedAt, "updated_by": value.ActorID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("operator changed during recovery")
		}
		result = tx.Where("operator_id=? AND user_id=? AND stage=?", value.OperatorID, value.UserID, string(domain.Completed)).Delete(&taskPO{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("retirement changed during archival")
		}
		return nil
	})
}
