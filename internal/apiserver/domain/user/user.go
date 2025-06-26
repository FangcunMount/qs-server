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

// ChangePassword 修改密码
func (u *User) ChangePassword(newPassword string) {
	u.password = newPassword
	u.updatedAt = time.Now()
}

// Block 封禁用户
func (u *User) Block() {
	u.status = StatusBlocked
	u.updatedAt = time.Now()
}

// Activate 激活用户
func (u *User) Activate() {
	u.status = StatusActive
	u.updatedAt = time.Now()
}

// 辅助函数
func generateUserID() string {
	return "user_" + time.Now().Format("20060102150405")
}
