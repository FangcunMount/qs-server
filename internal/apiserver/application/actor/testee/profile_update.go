package testee

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"time"
)

// UpdateProfileDTO preserves omitted fields and combines all backend edits.
type UpdateProfileDTO struct {
	TesteeID   uint64
	Name       *string
	Gender     *int8
	Birthday   *time.Time
	IsKeyFocus *bool
}
type ProfileUpdater interface {
	UpdateProfile(context.Context, UpdateProfileDTO) error
}

func (s *managementService) UpdateProfile(ctx context.Context, dto UpdateProfileDTO) error {
	id, err := testeeIDFromUint64("testee_id", dto.TesteeID)
	if err != nil {
		return err
	}
	if dto.Name == nil && dto.Gender == nil && dto.Birthday == nil && dto.IsKeyFocus == nil {
		return errors.WithCode(code.ErrInvalidArgument, "no profile changes supplied")
	}
	return s.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		target, err := s.loadForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		if err := s.authorizeUpdate(txCtx, target); err != nil {
			return err
		}
		var gender *domain.Gender
		if dto.Gender != nil {
			value := domain.Gender(*dto.Gender)
			gender = &value
		}
		if err := s.editor.UpdateBasicInfo(txCtx, target, dto.Name, gender, dto.Birthday); err != nil {
			return err
		}
		if dto.IsKeyFocus != nil {
			if *dto.IsKeyFocus {
				err = s.editor.MarkAsKeyFocus(txCtx, target)
			} else {
				err = s.editor.UnmarkAsKeyFocus(txCtx, target)
			}
			if err != nil {
				return err
			}
		}
		return s.repo.Update(txCtx, target)
	})
}
