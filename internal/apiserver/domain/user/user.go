package user

import (
	"time"

	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/auth"
	"github.com/fangcun-mount/qs-server/pkg/errors"
	"github.com/fangcun-mount/qs-server/pkg/util/idutil"
)

// User 用户聚合根
type User struct {
	id           idutil.ID[uint64]
	username     string
	password     string
	nickname     string
	avatar       string
	email        string
	phone        string
	introduction string
	status       Status
	createdAt    time.Time
	updatedAt    time.Time
}

// ID 获取用户ID
func (u *User) ID() idutil.ID[uint64] {
	return u.id
}

// Username 获取用户名
func (u *User) Username() string {
	return u.username
}

// Nickname 获取昵称
func (u *User) Nickname() string {
	return u.nickname
}

// Email 获取邮箱
func (u *User) Email() string {
	return u.email
}

// Phone 获取手机号
func (u *User) Phone() string {
	return u.phone
}

// Avatar 获取头像
func (u *User) Avatar() string {
	return u.avatar
}

// Introduction 获取简介
func (u *User) Introduction() string {
	return u.introduction
}

// Password 获取密码（加密后）
func (u *User) Password() string {
	return u.password
}

// Status 获取状态
func (u *User) Status() Status {
	return u.status
}

// CreatedAt 获取创建时间
func (u *User) CreatedAt() time.Time {
	return u.createdAt
}

// UpdatedAt 获取更新时间
func (u *User) UpdatedAt() time.Time {
	return u.updatedAt
}

// SetID 设置用户ID
func (u *User) SetID(id idutil.ID[uint64]) {
	u.id = id
}

// SetCreatedAt 设置创建时间
func (u *User) SetCreatedAt(createdAt time.Time) {
	u.createdAt = createdAt
}

// SetUpdatedAt 设置更新时间
func (u *User) SetUpdatedAt(updatedAt time.Time) {
	u.updatedAt = updatedAt
}

// SetPassword 设置已加密的密码（用于从数据库读取）
func (u *User) SetPassword(hashedPassword string) {
	u.password = hashedPassword
}

// ChangeUsername 修改用户名
func (u *User) ChangeUsername(newUsername string) error {
	if newUsername == "" {
		return errors.WithCode(code.ErrUserBasicInfoInvalid, "username cannot be empty")
	}
	u.username = newUsername
	return nil
}

// ChangeNickname 修改昵称
func (u *User) ChangeNickname(newNickname string) error {
	if newNickname == "" {
		return errors.WithCode(code.ErrUserBasicInfoInvalid, "nickname cannot be empty")
	}
	u.nickname = newNickname
	return nil
}

// ChangeEmail 修改邮箱
func (u *User) ChangeEmail(newEmail string) error {
	if newEmail == "" {
		return errors.WithCode(code.ErrUserBasicInfoInvalid, "email cannot be empty")
	}
	u.email = newEmail
	return nil
}

// ChangePhone 修改手机号
func (u *User) ChangePhone(newPhone string) error {
	if newPhone == "" {
		return errors.WithCode(code.ErrUserBasicInfoInvalid, "phone cannot be empty")
	}
	u.phone = newPhone
	return nil
}

// ChangePassword 修改密码
func (u *User) ChangePassword(newPassword string) error {
	if len(newPassword) < 6 {
		return errors.WithCode(code.ErrUserBasicInfoInvalid, "password must be at least 6 characters")
	}

	// 使用 bcrypt 加密密码
	hashedPassword, err := auth.Encrypt(newPassword)
	if err != nil {
		return errors.WithCode(code.ErrEncrypt, "failed to encrypt password")
	}

	u.password = hashedPassword
	return nil
}

// ChangeAvatar 修改头像
func (u *User) ChangeAvatar(newAvatar string) error {
	if newAvatar == "" {
		return errors.WithCode(code.ErrUserBasicInfoInvalid, "avatar cannot be empty")
	}
	u.avatar = newAvatar
	return nil
}

// ChangeIntroduction 修改简介
func (u *User) ChangeIntroduction(newIntroduction string) error {
	if newIntroduction == "" {
		return errors.WithCode(code.ErrUserBasicInfoInvalid, "introduction cannot be empty")
	}
	u.introduction = newIntroduction
	return nil
}

// ValidatePassword 验证密码
func (u *User) ValidatePassword(password string) bool {
	// 使用 bcrypt 验证密码
	err := auth.Compare(u.password, password)
	return err == nil
}

// IsActive 检查用户是否活跃
func (u *User) IsActive() bool {
	return u.status == StatusActive
}

// IsBlocked 检查用户是否被封禁
func (u *User) IsBlocked() bool {
	return u.status == StatusBlocked
}

// IsInactive 检查用户是否非活跃
func (u *User) IsInactive() bool {
	return u.status == StatusInactive
}

// Block 封禁用户
func (u *User) Block() error {
	return u.updateStatus(StatusBlocked)
}

// Activate 激活用户
func (u *User) Activate() error {
	return u.updateStatus(StatusActive)
}

// Deactivate 停用用户
func (u *User) Deactivate() error {
	return u.updateStatus(StatusInactive)
}

func (u *User) updateStatus(status Status) error {
	if u.status == status {
		return errors.WithCode(code.ErrUserStatusInvalid, "user is already in this status")
	}
	u.status = status
	return nil
}
