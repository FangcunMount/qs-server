package account

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
)

// Type 账户类型
type Type string

const (
	TypeWechat Type = "wechat" // 微信账户
	TypePhone  Type = "phone"  // 手机号账户
	TypeEmail  Type = "email"  // 邮箱账户
)

// Account 账户接口（多态）
type Account interface {
	// GetID 获取账户ID
	GetID() AccountID

	// GetUserID 获取关联的用户ID
	GetUserID() *user.UserID

	// GetType 获取账户类型
	GetType() Type

	// BindUser 绑定用户
	BindUser(userID user.UserID)

	// UnbindUser 解绑用户
	UnbindUser()

	// IsBound 是否已绑定用户
	IsBound() bool

	// IsActive 是否活跃
	IsActive() bool

	// CreatedAt 创建时间
	CreatedAt() time.Time

	// UpdatedAt 更新时间
	UpdatedAt() time.Time
}

// BaseAccount 账户基类（内嵌组合模式）
type BaseAccount struct {
	id        AccountID
	userID    *user.UserID
	accType   Type
	isActive  bool
	createdAt time.Time
	updatedAt time.Time
}

// NewBaseAccount 创建基础账户
func NewBaseAccount(accType Type) *BaseAccount {
	return &BaseAccount{
		accType:  accType,
		isActive: true,
	}
}

// GetID 获取账户ID
func (a *BaseAccount) GetID() AccountID {
	return a.id
}

// SetID 设置账户ID（仓储用）
func (a *BaseAccount) SetID(id AccountID) {
	a.id = id
}

// GetUserID 获取关联的用户ID
func (a *BaseAccount) GetUserID() *user.UserID {
	return a.userID
}

// GetType 获取账户类型
func (a *BaseAccount) GetType() Type {
	return a.accType
}

// BindUser 绑定用户
func (a *BaseAccount) BindUser(userID user.UserID) {
	a.userID = &userID
}

// UnbindUser 解绑用户
func (a *BaseAccount) UnbindUser() {
	a.userID = nil
}

// IsBound 是否已绑定用户
func (a *BaseAccount) IsBound() bool {
	return a.userID != nil
}

// IsActive 是否活跃
func (a *BaseAccount) IsActive() bool {
	return a.isActive
}

// SetActive 设置活跃状态
func (a *BaseAccount) SetActive(active bool) {
	a.isActive = active
}

// CreatedAt 创建时间
func (a *BaseAccount) CreatedAt() time.Time {
	return a.createdAt
}

// SetCreatedAt 设置创建时间（仓储用）
func (a *BaseAccount) SetCreatedAt(t time.Time) {
	a.createdAt = t
}

// UpdatedAt 更新时间
func (a *BaseAccount) UpdatedAt() time.Time {
	return a.updatedAt
}

// SetUpdatedAt 设置更新时间（仓储用）
func (a *BaseAccount) SetUpdatedAt(t time.Time) {
	a.updatedAt = t
}
