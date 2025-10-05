package account

import (
	"context"

	"gorm.io/gorm"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account/port"
	"github.com/fangcun-mount/qs-server/internal/apiserver/infra/mysql"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	pkgerrors "github.com/fangcun-mount/qs-server/pkg/errors"
)

// MergeLogRepository 账号合并日志存储库实现
type MergeLogRepository struct {
	mysql.BaseRepository[*MergeLogPO]
	mapper *MergeLogMapper
}

// NewMergeLogRepository 创建合并日志存储库
func NewMergeLogRepository(db *gorm.DB) port.MergeLogRepository {
	return &MergeLogRepository{
		BaseRepository: mysql.NewBaseRepository[*MergeLogPO](db),
		mapper:         NewMergeLogMapper(),
	}
}

// Save 保存合并日志
func (r *MergeLogRepository) Save(ctx context.Context, log *account.MergeLog) error {
	po := r.mapper.ToPO(log)
	return r.CreateAndSync(ctx, po, func(saved *MergeLogPO) {
		log.SetID(account.NewMergeLogID(saved.ID))
		log.SetCreatedAt(saved.CreatedAt)
	})
}

// FindByUserID 根据用户ID查找合并日志
func (r *MergeLogRepository) FindByUserID(ctx context.Context, userID user.UserID) ([]*account.MergeLog, error) {
	var pos []*MergeLogPO
	err := r.DB().WithContext(ctx).
		Where("user_id = ?", userID.Value()).
		Order("created_at DESC").
		Find(&pos).Error

	if err != nil {
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find merge logs: %v", err)
	}

	logs := make([]*account.MergeLog, 0, len(pos))
	for _, po := range pos {
		logs = append(logs, r.mapper.ToDomain(po))
	}

	return logs, nil
}

// FindByAccountID 根据账户ID查找合并日志
func (r *MergeLogRepository) FindByAccountID(ctx context.Context, accountID account.AccountID) ([]*account.MergeLog, error) {
	var pos []*MergeLogPO
	err := r.DB().WithContext(ctx).
		Where("account_id = ?", accountID.Value()).
		Order("created_at DESC").
		Find(&pos).Error

	if err != nil {
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find merge logs: %v", err)
	}

	logs := make([]*account.MergeLog, 0, len(pos))
	for _, po := range pos {
		logs = append(logs, r.mapper.ToDomain(po))
	}

	return logs, nil
}
