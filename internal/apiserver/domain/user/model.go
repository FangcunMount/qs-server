package user

import (
	"errors"
	"time"
)

// 领域错误
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrDuplicateUsername = errors.New("username already exists")
	ErrDuplicateEmail    = errors.New("email already exists")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrInvalidStatus     = errors.New("invalid user status")
	ErrUserBlocked       = errors.New("user is blocked")
)

// User 用户聚合根
type User struct {
	id        UserID
	username  string
	email     string
	password  string // 加密后的密码
	status    Status
	createdAt time.Time
	updatedAt time.Time
}

// UserID 用户唯一标识
type UserID struct {
	value string
}

// NewUserID 创建用户ID
func NewUserID(value string) UserID {
	return UserID{value: value}
}

// Value 获取ID值
func (id UserID) Value() string {
	return id.value
}

// Status 用户状态
type Status int

const (
	StatusActive   Status = 1 // 活跃
	StatusInactive Status = 2 // 非活跃
	StatusBlocked  Status = 3 // 被封禁
)

// String 获取状态字符串
func (s Status) String() string {
	switch s {
	case StatusActive:
		return "active"
	case StatusInactive:
		return "inactive"
	case StatusBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}

// NewUser 创建新用户
func NewUser(username, email, password string) *User {
	now := time.Now()
	return &User{
		id:        NewUserID(generateUserID()),
		username:  username,
		email:     email,
		password:  password,
		status:    StatusActive,
		createdAt: now,
		updatedAt: now,
	}
}

// ID 获取用户ID
func (u *User) ID() UserID {
	return u.id
}

// Username 获取用户名
func (u *User) Username() string {
	return u.username
}

// Email 获取邮箱
func (u *User) Email() string {
	return u.email
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

// ChangeUsername 修改用户名
func (u *User) ChangeUsername(newUsername string) error {
	if newUsername == "" {
		return errors.New("username cannot be empty")
	}
	u.username = newUsername
	u.updatedAt = time.Now()
	return nil
}

// ChangeEmail 修改邮箱
func (u *User) ChangeEmail(newEmail string) error {
	if newEmail == "" {
		return errors.New("email cannot be empty")
	}
	u.email = newEmail
	u.updatedAt = time.Now()
	return nil
}

// ChangePassword 修改密码
func (u *User) ChangePassword(newPassword string) error {
	if len(newPassword) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	u.password = newPassword
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
		return ErrUserBlocked
	}
	u.status = StatusBlocked
	u.updatedAt = time.Now()
	return nil
}

// Activate 激活用户
func (u *User) Activate() error {
	if u.status == StatusActive {
		return ErrInvalidStatus
	}
	u.status = StatusActive
	u.updatedAt = time.Now()
	return nil
}

// Deactivate 停用用户
func (u *User) Deactivate() error {
	if u.status == StatusInactive {
		return ErrInvalidStatus
	}
	u.status = StatusInactive
	u.updatedAt = time.Now()
	return nil
}

// 辅助函数
func generateUserID() string {
	return time.Now().Format("20060102150405")
}
