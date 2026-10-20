package plan

import (
	"context"
	"fmt"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/tasktransaction"
)

// This optional internal seam lets an operator retain its ownership check in
// the original root without preparing MQ inside the old outer transaction.
type transactionalTaskOpening interface {
	TransactionalTaskOpening() bool
	OpenTaskWithGuard(context.Context, int64, string, port.LocalGuard) (*TaskResult, error)
}

func (s *commandService) TransactionalTaskOpening() bool {
	opener, ok := s.taskManagement.(transactionalTaskOpening)
	return ok && opener.TransactionalTaskOpening()
}
func (s *commandService) OpenTaskWithGuard(ctx context.Context, orgID int64, id string, guard port.LocalGuard) (*TaskResult, error) {
	opener, ok := s.taskManagement.(transactionalTaskOpening)
	if !ok || !opener.TransactionalTaskOpening() {
		return nil, fmt.Errorf("transactional task opening is not configured")
	}
	return opener.OpenTaskWithGuard(ctx, orgID, id, guard)
}

func (s *taskManagementService) TransactionalTaskOpening() bool {
	return s.persistence.transactionOwnerSelected()
}
func (s *taskManagementService) OpenTaskWithGuard(ctx context.Context, orgID int64, id string, guard port.LocalGuard) (*TaskResult, error) {
	if !s.TransactionalTaskOpening() {
		return nil, fmt.Errorf("transactional task opening is not configured")
	}
	return s.openTask(ctx, orgID, id, guard)
}
