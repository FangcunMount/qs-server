package operatorretirement

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Runtime uses the final table; the maintenance tool chooses its explicit migration layout.
const operatorTable = "operators"

type Repository struct {
	db    *gorm.DB
	table string
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db, table: operatorTable} }

// NewLegacyRepository is only for the cutover maintenance tool before table rename.
func NewLegacyRepository(db *gorm.DB) *Repository { return &Repository{db: db, table: "staff"} }

var _ port.Repository = (*Repository)(nil)

type connectionKey struct{}

func (r *Repository) dbFor(ctx context.Context) *gorm.DB {
	if db, ok := ctx.Value(connectionKey{}).(*gorm.DB); ok {
		return db.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}
func (r *Repository) WithOperatorLock(ctx context.Context, org int64, id uint64, fn func(context.Context) error) error {
	var row struct{ UserID int64 }
	if err := r.db.WithContext(ctx).Table(r.table).Select("user_id").Where("org_id=? AND id=?", org, id).Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.WithCode(code.ErrUserNotFound, "operator not found in current company")
		}
		return err
	}
	return r.withUserLock(ctx, row.UserID, fn)
}

type userLockKey struct{}

func (r *Repository) withUserLock(ctx context.Context, userID int64, fn func(context.Context) error) error {
	if held, ok := ctx.Value(userLockKey{}).(int64); ok && held == userID {
		return fn(ctx)
	}
	return r.db.WithContext(ctx).Connection(func(db *gorm.DB) error {
		var acquired int
		key := fmt.Sprintf("qs:operator:user:%d", userID)
		if err := db.Raw("SELECT GET_LOCK(?, 0)", key).Scan(&acquired).Error; err != nil {
			return err
		}
		if acquired != 1 {
			return errors.WithCode(code.ErrConflict, "operator mutation in progress")
		}
		defer func() {
			// Release even if the caller cancelled; the pooled connection must not retain the lock.
			releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			db.WithContext(releaseCtx).Exec("SELECT RELEASE_LOCK(?)", key)
		}()
		return fn(context.WithValue(context.WithValue(ctx, connectionKey{}, db), userLockKey{}, userID))
	})
}
func (r *Repository) Find(ctx context.Context, org int64, id uint64) (port.Target, error) {
	var row port.Target
	err := r.dbFor(ctx).Table(r.table).Select("id,org_id,user_id,version").Where("org_id=? AND id=? AND deleted_at IS NULL", org, id).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return row, errors.WithCode(code.ErrUserNotFound, "operator not found in current company")
	}
	return row, err
}

type taskPO struct {
	OperatorID  uint64
	OrgID       int64
	UserID      int64
	Stage       string
	Payload     string
	BeforeState string
	UpdatedAt   time.Time
}

func (taskPO) TableName() string { return "operator_retirement_tasks" }
func (r *Repository) FindTask(ctx context.Context, org int64, id uint64) (*domain.Task, error) {
	var po taskPO
	err := r.dbFor(ctx).Where("org_id=? AND operator_id=?", org, id).Take(&po).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var task domain.Task
	if err := json.Unmarshal([]byte(po.Payload), &task); err != nil {
		return nil, err
	}
	if task.OperatorID != id || task.OrgID != org || task.UserID != po.UserID || string(task.Stage) != po.Stage {
		return nil, fmt.Errorf("retirement task identity mismatch")
	}
	return &task, task.Validate()
}
func (r *Repository) OtherMemberships(ctx context.Context, user int64, id uint64) (int64, error) {
	var count int64
	err := r.dbFor(ctx).Table(r.table).Where("user_id=? AND id<>? AND deleted_at IS NULL", user, id).Count(&count).Error
	return count, err
}
func (r *Repository) Begin(ctx context.Context, task domain.Task) error {
	if err := task.Validate(); err != nil {
		return err
	}
	return r.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var before map[string]interface{}
		if err := tx.Table(r.table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("org_id=? AND id=? AND version=? AND deleted_at IS NULL", task.OrgID, task.OperatorID, task.ExpectedVersion).Take(&before).Error; err != nil {
			return err
		}
		raw, err := json.Marshal(before)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(task)
		if err != nil {
			return err
		}
		if err := tx.Create(&taskPO{OperatorID: task.OperatorID, OrgID: task.OrgID, UserID: task.UserID, Stage: string(task.Stage), Payload: string(payload), BeforeState: string(raw), UpdatedAt: task.UpdatedAt}).Error; err != nil {
			return err
		}
		result := tx.Table(r.table).Where("org_id=? AND id=? AND version=? AND deleted_at IS NULL", task.OrgID, task.OperatorID, task.ExpectedVersion).
			Updates(map[string]interface{}{"is_active": false, "version": gorm.Expr("version+1"), "updated_at": task.UpdatedAt, "updated_by": task.ActorID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.WithCode(code.ErrConflict, "operator changed")
		}
		return nil
	})
}
func save(db *gorm.DB, task domain.Task) error {
	if err := task.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	// Completed task payloads are immutable, including failure metadata.
	result := db.Model(&taskPO{}).Where("org_id=? AND operator_id=? AND stage<>?", task.OrgID, task.OperatorID, string(domain.Completed)).Updates(map[string]interface{}{"stage": string(task.Stage), "payload": string(payload), "updated_at": task.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.WithCode(code.ErrConflict, "retirement task changed")
	}
	return nil
}
func (r *Repository) Save(ctx context.Context, task domain.Task) error {
	return save(r.dbFor(ctx), task)
}
func (r *Repository) Finish(ctx context.Context, task domain.Task) error {
	if task.Stage != domain.Completed {
		return fmt.Errorf("retirement has not completed")
	}
	return r.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Table(r.table).Where("org_id=? AND id=? AND user_id=? AND version=? AND is_active=FALSE AND deleted_at IS NULL", task.OrgID, task.OperatorID, task.UserID, task.ExpectedVersion+1).
			Updates(map[string]interface{}{"roles": "[]", "effective_roles": "[]", "authz_policy_version": task.PolicyVersion, "authz_projected_at": task.UpdatedAt, "authz_projection_pending": false, "deleted_at": task.UpdatedAt, "updated_at": task.UpdatedAt, "updated_by": task.ActorID, "version": gorm.Expr("version+1")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.WithCode(code.ErrConflict, "operator retirement state changed")
		}
		return save(tx, task)
	})
}

// WithinMutation rejects both in-progress and completed exits. Restoring access is an audited recovery operation.
func (r *Repository) WithinMutation(ctx context.Context, userID int64, fn func(context.Context) error) error {
	if userID <= 0 {
		return errors.WithCode(code.ErrInvalidArgument, "invalid operator user")
	}
	return r.withUserLock(ctx, userID, func(locked context.Context) error {
		var count int64
		if err := r.dbFor(locked).Model(&taskPO{}).Where("user_id=?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.WithCode(code.ErrConflict, "operator is retiring or retired")
		}
		return fn(locked)
	})
}
