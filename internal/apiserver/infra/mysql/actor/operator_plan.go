package actor

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// RequireCurrentOperatorPlanSchema prevents current maintenance plans from
// spanning the actor-column DDL. Future schema changes must explicitly review
// this boundary before enabling these tools on their new layout.
func RequireCurrentOperatorPlanSchema(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("current Operator schema required; repeat preflight after migration")
	}
	var rows []struct {
		Version *uint64
		Dirty   *bool
	}
	if err := db.WithContext(ctx).Raw("SELECT version,dirty FROM schema_migrations LIMIT 2").Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 1 || rows[0].Version == nil || *rows[0].Version != 102 || rows[0].Dirty == nil || *rows[0].Dirty {
		return fmt.Errorf("clean SQL102 required; repeat preflight after migration")
	}
	return nil
}
