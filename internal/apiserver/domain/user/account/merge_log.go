package account

import (
	"time"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	"github.com/fangcun-mount/qs-server/pkg/util/idutil"
)

// MergeReason 合并原因
type MergeReason string

const (
	MergeReasonUnionID MergeReason = "unionid" // 通过UnionID合并
	MergeReasonPhone   MergeReason = "phone"   // 通过手机号合并
	MergeReasonManual  MergeReason = "manual"  // 手动合并
)

// MergeLog 账号合并日志（值对象）
type MergeLog struct {
	id        MergeLogID
	userID    user.UserID
	accountID AccountID
	reason    MergeReason
	createdAt time.Time
}

// NewMergeLog 创建账号合并日志
func NewMergeLog(userID user.UserID, accountID AccountID, reason MergeReason) *MergeLog {
	return &MergeLog{
		userID:    userID,
		accountID: accountID,
		reason:    reason,
		createdAt: time.Now(),
	}
}

// ReconstituteMergeLog 从持久化数据重建
func ReconstituteMergeLog(
	id MergeLogID,
	userID user.UserID,
	accountID AccountID,
	reason MergeReason,
	createdAt time.Time,
) *MergeLog {
	return &MergeLog{
		id:        id,
		userID:    userID,
		accountID: accountID,
		reason:    reason,
		createdAt: createdAt,
	}
}

// Getters
func (l *MergeLog) ID() MergeLogID       { return l.id }
func (l *MergeLog) UserID() user.UserID  { return l.userID }
func (l *MergeLog) AccountID() AccountID { return l.accountID }
func (l *MergeLog) Reason() MergeReason  { return l.reason }
func (l *MergeLog) CreatedAt() time.Time { return l.createdAt }

// SetID 设置ID（仓储用）
func (l *MergeLog) SetID(id MergeLogID) {
	l.id = id
}

// SetCreatedAt 设置创建时间（仓储用）
func (l *MergeLog) SetCreatedAt(t time.Time) {
	l.createdAt = t
}

// MergeLogID 合并日志ID值对象
type MergeLogID = idutil.ID[uint64]

// NewMergeLogID 创建合并日志ID
func NewMergeLogID(value uint64) MergeLogID {
	return idutil.NewID[uint64](value)
}
