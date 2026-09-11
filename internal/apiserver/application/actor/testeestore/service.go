// Package testeestore coordinates headquarters ownership changes in the Actor module.
package testeestore

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/testeestore"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Actor struct{ OrgID, UserID int64 }
type Change struct {
	StoreID           uint64
	ExpectedVersion   uint32
	Reason, RequestID string
}
type Service struct {
	repo port.Repository
	tx   transaction.Runner
}

func NewService(repo port.Repository, tx transaction.Runner) *Service {
	return &Service{repo: repo, tx: tx}
}
func authorize(ctx context.Context, actor Actor) error {
	snapshot, ok := authz.FromContext(ctx)
	if actor.OrgID <= 0 || actor.UserID <= 0 || !ok || snapshot == nil || !snapshot.IsQSAdmin() {
		return errors.WithCode(code.ErrPermissionDenied, "company administrator permission required")
	}
	return nil
}

func (s *Service) AssignInitial(ctx context.Context, actor Actor, id uint64, change Change) (*port.History, error) {
	return s.change(ctx, actor, id, change, "initial")
}
func (s *Service) Transfer(ctx context.Context, actor Actor, id uint64, change Change) (*port.History, error) {
	return s.change(ctx, actor, id, change, "transfer")
}
func (s *Service) change(ctx context.Context, actor Actor, id uint64, change Change, kind string) (*port.History, error) {
	if err := authorize(ctx, actor); err != nil {
		return nil, err
	}
	change.Reason, change.RequestID = strings.TrimSpace(change.Reason), strings.TrimSpace(change.RequestID)
	if id == 0 || change.StoreID == 0 || change.ExpectedVersion == 0 || change.Reason == "" || utf8.RuneCountInString(change.Reason) > 500 || change.RequestID == "" || len(change.RequestID) > 64 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "testee, store, version, reason and request ID required")
	}
	historyID, now := meta.New().Uint64(), time.Now().UTC()
	var result *port.History
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		target, err := s.repo.LockStore(ctx, actor.OrgID, change.StoreID)
		if err != nil {
			return err
		}
		subject, err := s.repo.LockTestee(ctx, actor.OrgID, id)
		if err != nil {
			return err
		}
		previous, err := s.repo.FindChange(ctx, actor.OrgID, id, change.RequestID)
		if err != nil {
			return err
		}
		if previous != nil {
			if previous.ToStoreID != change.StoreID || previous.Kind != kind || previous.ActorID != actor.UserID || previous.Reason != change.Reason || previous.Version != change.ExpectedVersion+1 {
				return errors.WithCode(code.ErrConflict, "request ID already used for a different ownership change")
			}
			result = previous
			return nil
		}
		from := subject.StoreID()
		var changed bool
		if kind == "initial" {
			changed, err = subject.AssignInitialStore(target, change.ExpectedVersion)
		} else {
			changed, err = subject.TransferStore(target, change.ExpectedVersion)
		}
		if err != nil {
			return err
		}
		result = &port.History{ID: historyID, OrgID: actor.OrgID, TesteeID: id, FromStoreID: from, ToStoreID: change.StoreID, Kind: kind, ActorID: actor.UserID, CreatedAt: now, Reason: change.Reason, RequestID: change.RequestID, Version: subject.StoreVersion()}
		if !changed {
			result.ID = 0
			result.Kind = "unchanged"
			return nil
		}
		if err = s.repo.SaveOwnership(ctx, result, change.ExpectedVersion); err != nil {
			return err
		}
		return s.repo.AppendHistory(ctx, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) History(ctx context.Context, actor Actor, id, before uint64, limit int) ([]port.History, error) {
	if err := authorize(ctx, actor); err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "testee required")
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.History(ctx, actor.OrgID, id, before, limit)
}
