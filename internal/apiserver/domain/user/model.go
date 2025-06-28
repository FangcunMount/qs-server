package user

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// User 用户聚合根
type User struct {
	id           UserID
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

// NewUser 创建新用户
func NewUser(username, nickname, email, phone string) *User {
	return &User{
		username: username,
		nickname: nickname,
		email:    email,
		phone:    phone,
		status:   StatusActive,
	}
}

// ID 获取用户ID
func (u *User) ID() UserID {
	return u.id
}

// SetID 设置用户ID
func (u *User) SetID(id UserID) {
	u.id = id
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

// SetCreatedAt 设置创建时间
func (u *User) SetCreatedAt(createdAt time.Time) {
	u.createdAt = createdAt
}

// UpdatedAt 获取更新时间
func (u *User) UpdatedAt() time.Time {
	return u.updatedAt
}

// SetUpdatedAt 设置更新时间
func (u *User) SetUpdatedAt(updatedAt time.Time) {
	u.updatedAt = updatedAt
}

// ChangeUsername 修改用户名
func (u *User) ChangeUsername(newUsername string) error {
	if newUsername == "" {
		return errors.NewWithCode(errors.ErrUserInvalidUsername, "username cannot be empty")
	}
	u.username = newUsername
	u.updatedAt = time.Now()
	return nil
}

// ChangeNickname 修改昵称
func (u *User) ChangeNickname(newNickname string) error {
	if newNickname == "" {
		return errors.NewWithCode(errors.ErrUserInvalidUsername, "nickname cannot be empty")
	}
	u.nickname = newNickname
	u.updatedAt = time.Now()
	return nil
}

// ChangeEmail 修改邮箱
func (u *User) ChangeEmail(newEmail string) error {
	if newEmail == "" {
		return errors.NewWithCode(errors.ErrUserInvalidEmail, "email cannot be empty")
	}
	u.email = newEmail
	u.updatedAt = time.Now()
	return nil
}

// ChangePhone 修改手机号
func (u *User) ChangePhone(newPhone string) error {
	if newPhone == "" {
		return errors.NewWithCode(errors.ErrUserInvalidPhone, "phone cannot be empty")
	}
	u.phone = newPhone
	u.updatedAt = time.Now()
	return nil
}

// ChangePassword 修改密码
func (u *User) ChangePassword(newPassword string) error {
	if len(newPassword) < 6 {
		return errors.NewWithCode(errors.ErrUserInvalidPassword, "password must be at least 6 characters")
	}
	u.password = newPassword
	u.updatedAt = time.Now()
	return nil
}

// ChangeAvatar 修改头像
func (u *User) ChangeAvatar(newAvatar string) error {
	if newAvatar == "" {
		return errors.NewWithCode(errors.ErrUserInvalidAvatar, "avatar cannot be empty")
	}
	u.avatar = newAvatar
	u.updatedAt = time.Now()
	return nil
}

// ValidatePassword 验证密码
func (u *User) ValidatePassword(password string) bool {
	// TODO: 实现真正的密码验证逻辑（应该使用加密后的密码比较）
	return u.password == password
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
	if u.status == StatusBlocked {
		return errors.NewWithCode(errors.ErrUserBlocked, "user is already blocked")
	}
	u.status = StatusBlocked
	u.updatedAt = time.Now()
	return nil
}

// Activate 激活用户
func (u *User) Activate() error {
	if u.status == StatusActive {
		return errors.NewWithCode(errors.ErrUserInvalidStatus, "user is already active")
	}
	u.status = StatusActive
	u.updatedAt = time.Now()
	return nil
}

// Deactivate 停用用户
func (u *User) Deactivate() error {
	if u.status == StatusInactive {
		return errors.NewWithCode(errors.ErrUserInvalidStatus, "user is already inactive")
	}
	u.status = StatusInactive
	u.updatedAt = time.Now()
	return nil
}
