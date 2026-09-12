package actor

import (
	"context"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"gorm.io/gorm"
	"strings"
	"testing"
	"time"
)

func TestOperatorSavePreservesExplicitActivity(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "inactive", true: "active"}[active], func(t *testing.T) {
			db := newDryRunActorDB(t).Session(&gorm.Session{SkipDefaultTransaction: true})
			var saved *OperatorPO
			if err := db.Callback().Create().After("gorm:create").Register("capture_operator_activity", func(tx *gorm.DB) {
				var ok bool
				saved, ok = tx.Statement.Dest.(*OperatorPO)
				if !ok {
					t.Fatalf("unexpected insert type %T", tx.Statement.Dest)
				}
			}); err != nil {
				t.Fatal(err)
			}
			op := domain.NewOperator(1, 2, "prepared operator")
			op.SetID(3)
			if !active {
				if err := domain.NewLifecycler().Deactivate(op); err != nil {
					t.Fatal(err)
				}
			}
			if err := NewOperatorRepository(db).Save(context.Background(), op); err != nil {
				t.Fatal(err)
			}
			if saved == nil || saved.IsActive != active {
				t.Fatalf("insert changed explicit activity %v: %+v", active, saved)
			}
		})
	}
}

func TestOperatorUpdatePersistsClearedProjectionPending(t *testing.T) {
	db := newDryRunActorDB(t).Session(&gorm.Session{DryRun: true, SkipDefaultTransaction: true})
	var statement string
	var values []any
	if err := db.Callback().Update().After("gorm:update").Register("capture_projection", func(tx *gorm.DB) {
		tx.RowsAffected = 1 // DryRun records SQL without executing it.
		statement = tx.Statement.SQL.String()
		values = tx.Statement.Vars
	}); err != nil {
		t.Fatal(err)
	}
	op := domain.NewOperator(1, 2, "projection test")
	op.SetID(3)
	now := time.Now()
	op.ReplaceRolesProjection([]domain.Role{domain.Role("qs:assessment_operator")}, nil, 12, &now, false)
	if err := NewOperatorRepository(db).Update(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statement, "`authz_projection_pending`=?") {
		t.Fatalf("pending=false was omitted: %s", statement)
	}
	if !strings.Contains(statement, "version+1") || !strings.Contains(statement, "version=?") {
		t.Fatalf("missing version guard: %s", statement)
	}
	foundFalse := false
	for _, value := range values {
		if b, ok := value.(bool); ok && !b {
			foundFalse = true
		}
	}
	if !foundFalse {
		t.Fatalf("pending=false value missing: %v", values)
	}
	if strings.Contains(statement, "`created_at`=") {
		t.Fatal("projection update overwrites creation audit")
	}
}
