package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"gorm.io/gorm"
)

// NewRunRepository creates an evaluation run repository backed by runtime_checkpoint.
func NewRunRepository(db *gorm.DB) evaluationrun.Repository {
	return checkpoint.NewRunRepository(db)
}
