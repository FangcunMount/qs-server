// Package store orchestrates headquarters store administration within Actor.
package store

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/actorstore"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"strings"
	"time"
	"unicode/utf8"
)

type Actor struct{ OrgID, UserID int64 }
type Change struct {
	StoreID           uint64
	ExpectedVersion   uint32
	Reason, RequestID string
}

// CompanyScope verifies active company membership and the range paired with an action.
type CompanyScope interface {
	ResolveStoreRange(context.Context, int64, int64, string, string) (authz.StoreRange, error)
}
type Service struct {
	scope CompanyScope
	repo  port.Repository
	tx    transaction.Runner
}

func NewService(r port.Repository, tx transaction.Runner, scope CompanyScope) *Service {
	return &Service{repo: r, tx: tx, scope: scope}
}
func (s *Service) authorize(ctx context.Context, a Actor, resource, action string) error {
	snapshot, ok := authz.FromContext(ctx)
	if s.scope == nil || a.OrgID <= 0 || a.UserID <= 0 || actorctx.OperatorOrgID(ctx) != a.OrgID || actorctx.GrantingUserID(ctx) != uint64(a.UserID) || !ok || snapshot == nil || !snapshot.IsQSAdmin() {
		return errors.WithCode(code.ErrPermissionDenied, "company administrator permission required")
	}
	allowed, err := s.scope.ResolveStoreRange(ctx, a.OrgID, a.UserID, resource, action)
	if err != nil {
		return err
	}
	if !allowed.AllStores {
		return errors.WithCode(code.ErrPermissionDenied, "headquarters company scope required")
	}
	return nil
}
func (s *Service) List(ctx context.Context, a Actor, f port.Filter) (port.Page, error) {
	if err := s.authorize(ctx, a, "qs:actor:collection:stores", "list"); err != nil {
		return port.Page{}, err
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	return s.repo.List(ctx, a.OrgID, f)
}
func (s *Service) Get(ctx context.Context, a Actor, id uint64) (*port.Item, error) {
	if err := s.authorize(ctx, a, "qs:actor:collection:stores", "read"); err != nil {
		return nil, err
	}
	v, err := s.repo.Get(ctx, a.OrgID, id)
	if err != nil {
		return nil, err
	}
	n, err := s.repo.CountClinicians(ctx, a.OrgID, id)
	if err != nil {
		return nil, err
	}
	return &port.Item{Store: v, ClinicianCount: n}, nil
}
func (s *Service) Create(ctx context.Context, a Actor, c, n, address string) (*domain.Store, error) {
	if err := s.authorize(ctx, a, "qs:actor:collection:stores", "create"); err != nil {
		return nil, err
	}
	v, err := domain.New(meta.New().Uint64(), a.OrgID, c, n, address, a.UserID, time.Now())
	if err != nil {
		return nil, err
	}
	return v, s.repo.Create(ctx, v)
}
func (s *Service) Update(ctx context.Context, a Actor, id uint64, version uint32, name, address string, active *bool) (*domain.Store, error) {
	if err := s.authorize(ctx, a, "qs:actor:collection:stores", "update"); err != nil {
		return nil, err
	}
	if version == 0 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "expected_version required")
	}
	var result *domain.Store
	now := time.Now()
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		v, err := s.repo.LockStore(ctx, a.OrgID, id)
		if err != nil {
			return err
		}
		if active == nil {
			err = v.UpdateProfile(name, address, version, a.UserID, now)
		} else {
			var count int64
			count, err = s.repo.CountClinicians(ctx, a.OrgID, id)
			if err != nil {
				return err
			}
			err = v.SetActive(*active, count, version, a.UserID, now)
		}
		if err != nil {
			return err
		}
		if err = s.repo.Save(ctx, v, version); err != nil {
			return err
		}
		result = v
		return nil
	})
	return result, err
}
func (s *Service) Assign(ctx context.Context, a Actor, id uint64, c Change) (*port.History, error) {
	if err := s.authorize(ctx, a, "qs:actor:collection:clinicians", "update"); err != nil {
		return nil, err
	}
	c.Reason = strings.TrimSpace(c.Reason)
	c.RequestID = strings.TrimSpace(c.RequestID)
	if id == 0 || c.StoreID == 0 || c.ExpectedVersion == 0 || c.Reason == "" || utf8.RuneCountInString(c.Reason) > 500 || c.RequestID == "" || len(c.RequestID) > 64 {
		return nil, errors.WithCode(code.ErrInvalidArgument, "store, version, reason and request ID required")
	}
	historyID, now := meta.New().Uint64(), time.Now()
	var result *port.History
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		target, err := s.repo.LockStore(ctx, a.OrgID, c.StoreID)
		if err != nil {
			return err
		}
		cl, err := s.repo.LockClinician(ctx, a.OrgID, id)
		if err != nil {
			return err
		}
		old, err := s.repo.FindChange(ctx, a.OrgID, id, c.RequestID)
		if err != nil {
			return err
		}
		if old != nil {
			if old.ToStoreID != c.StoreID || old.Reason != c.Reason || old.ActorID != a.UserID {
				return errors.WithCode(code.ErrConflict, "request ID already used for a different assignment")
			}
			result = old
			return nil
		}
		from := cl.StoreID()
		changed, err := cl.AssignStore(target, c.ExpectedVersion)
		if err != nil {
			return err
		}
		if !changed {
			result = &port.History{OrgID: a.OrgID, ClinicianID: id, FromStoreID: from, ToStoreID: c.StoreID, Kind: "unchanged", Version: cl.Version()}
			return nil
		}
		h := &port.History{ID: historyID, OrgID: a.OrgID, ClinicianID: id, FromStoreID: from, ToStoreID: c.StoreID, Kind: "initial", ActorID: a.UserID, CreatedAt: now, Reason: c.Reason, RequestID: c.RequestID, Version: cl.Version()}
		if from != nil {
			h.Kind = "transfer"
			h.InvalidatedCount, err = s.repo.InvalidateEntries(ctx, a.OrgID, id, a.UserID, now)
			if err != nil {
				return err
			}
		}
		if err = s.repo.SaveAssignment(ctx, h); err != nil {
			return err
		}
		if err = s.repo.AppendHistory(ctx, h); err != nil {
			return err
		}
		result = h
		return nil
	})
	return result, err
}
func (s *Service) History(ctx context.Context, a Actor, id uint64) ([]port.History, error) {
	if err := s.authorize(ctx, a, "qs:actor:collection:clinicians", "read"); err != nil {
		return nil, err
	}
	return s.repo.History(ctx, a.OrgID, id)
}

func (s *Service) Progress(ctx context.Context, a Actor) (port.Progress, error) {
	if err := s.authorize(ctx, a, "qs:actor:collection:clinicians", "list"); err != nil {
		return port.Progress{}, err
	}
	return s.repo.Progress(ctx, a.OrgID)
}
