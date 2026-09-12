package answeringstart

import (
	"context"
	"errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answeringstart"
)

var ErrDuplicate = errors.New("answering start already exists")

var ErrConflict = errors.New("answering start request key reused with different input")

// Repository participates in the application transaction. The application locks
// current Actor ownership before inserting; a duplicate insert never overwrites.
type Repository interface {
	FindRequest(context.Context, int64, uint64, string) (*domain.Record, error)
	Find(context.Context, uint64) (*domain.Record, error)
	Insert(context.Context, *domain.Record) error
}
