package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Delete 删除量表
func (s *lifecycleService) Delete(ctx context.Context, code string) error {
	if code == "" {
		return errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return err
	}
	if !m.IsDraft() {
		return errors.WithCode(errorCode.ErrInvalidArgument, "只能删除草稿状态的量表")
	}

	if err := s.repo.Remove(ctx, code); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "删除量表失败")
	}

	s.refreshListCache(ctx)

	return nil
}
