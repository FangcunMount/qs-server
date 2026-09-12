package answeringstart

import (
	"context"
	"errors"
	"fmt"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answeringstart"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/answeringstart"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"time"
)

// Admission validates the authenticated participant/guardian relationship,
// content version and source admission. It must never infer an Operator role.
type Admission interface {
	ValidateStart(context.Context, *domain.Intent) error
}

// OwnershipLocker shares Actor's transaction lock with ownership transfer.
type OwnershipLocker interface {
	LockTestee(context.Context, int64, uint64) (*testee.Testee, error)
}

type Service struct {
	repo      port.Repository
	tx        transaction.Runner
	owners    OwnershipLocker
	admission Admission
	now       func() time.Time
	newID     func() uint64
}

func NewService(repo port.Repository, tx transaction.Runner, owners OwnershipLocker, admission Admission) *Service {
	return &Service{repo: repo, tx: tx, owners: owners, admission: admission, now: time.Now, newID: func() uint64 { return meta.New().Uint64() }}
}

type Result struct {
	Record  *domain.Record
	Created bool
}

// Start freezes ownership only after acquiring the transfer lock. A replay
// returns the original timestamp and ownership rather than observing a new store.
func (s *Service) Start(ctx context.Context, intent domain.Intent) (Result, error) {
	if err := intent.Validate(); err != nil {
		return Result{}, err
	}
	if s.repo == nil || s.tx == nil || s.owners == nil || s.admission == nil {
		return Result{}, errors.New("answering start dependencies unavailable")
	}
	if err := s.admission.ValidateStart(ctx, &intent); err != nil {
		return Result{}, err
	}
	if record, err := s.repo.FindRequest(ctx, intent.OrgID, intent.UserID, intent.RequestKey); err != nil || record != nil {
		return replay(record, intent, err)
	}
	id := s.newID()
	var result Result
	err := s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		subject, err := s.owners.LockTestee(txCtx, intent.OrgID, intent.TesteeID)
		if err != nil {
			return err
		}
		if subject == nil || subject.OrgID() != intent.OrgID {
			return errors.New("testee unavailable")
		}
		previous, err := s.repo.FindRequest(txCtx, intent.OrgID, intent.UserID, intent.RequestKey)
		if err != nil || previous != nil {
			result, err = replay(previous, intent, err)
			return err
		}
		record, err := domain.New(intent, id, subject.StoreID(), subject.StoreVersion(), s.now().UTC().Truncate(time.Microsecond))
		if err != nil {
			return err
		}
		if err = s.repo.Insert(txCtx, record); err != nil {
			return err
		}
		result = Result{Record: record, Created: true}
		return nil
	})
	if errors.Is(err, port.ErrDuplicate) {
		// The conflicting insert has committed. Read outside the rolled-back
		// transaction, including when the key was reused for a different testee.
		previous, readErr := s.repo.FindRequest(ctx, intent.OrgID, intent.UserID, intent.RequestKey)
		if readErr != nil {
			return Result{}, readErr
		}
		if previous == nil {
			return Result{}, err
		}
		return replay(previous, intent, nil)
	}
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func replay(record *domain.Record, intent domain.Intent, err error) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	if record == nil {
		return Result{}, errors.New("answering start replay missing")
	}
	if !record.Matches(intent) {
		return Result{}, port.ErrConflict
	}
	return Result{Record: record}, nil
}

// ResolveSubmission checks immutable identity and content, not current store.
// The caller must still apply today's participant and source admission rules.
func (s *Service) ResolveSubmission(ctx context.Context, id uint64, intent domain.Intent) (sheet.StartContext, error) {
	if id == 0 {
		return sheet.StartContext{}, nil
	}
	if s.repo == nil {
		return sheet.StartContext{}, errors.New("answering start repository unavailable")
	}
	record, err := s.repo.Find(ctx, id)
	if err != nil {
		return sheet.StartContext{}, fmt.Errorf("load answering start: %w", err)
	}
	if record == nil {
		return sheet.StartContext{}, errors.New("answering start not found")
	}
	expected := record.Intent()
	// A submission has its own idempotency key. Only the stored start key is
	// substituted; all caller identity, content and source fields must match.
	intent.RequestKey = expected.RequestKey
	if !record.Matches(intent) {
		return sheet.StartContext{}, port.ErrConflict
	}
	return record.Context(), nil
}
