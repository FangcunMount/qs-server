package user

import (
	"time"

	"github.com/yshujie/questionnaire-scale/pkg/auth"
)

type UserBuilder struct {
	u *User
}

func NewUserBuilder() *UserBuilder {
	return &UserBuilder{u: &User{}}
}

func (b *UserBuilder) WithID(id UserID) *UserBuilder {
	b.u.id = id
	return b
}
func (b *UserBuilder) WithUsername(username string) *UserBuilder {
	b.u.username = username
	return b
}
func (b *UserBuilder) WithNickname(nickname string) *UserBuilder {
	b.u.nickname = nickname
	return b
}
func (b *UserBuilder) WithAvatar(avatar string) *UserBuilder {
	b.u.avatar = avatar
	return b
}
func (b *UserBuilder) WithEmail(email string) *UserBuilder {
	b.u.email = email
	return b
}
func (b *UserBuilder) WithPhone(phone string) *UserBuilder {
	b.u.phone = phone
	return b
}
func (b *UserBuilder) WithStatus(status Status) *UserBuilder {
	b.u.status = status
	return b
}
func (b *UserBuilder) WithIntroduction(introduction string) *UserBuilder {
	b.u.introduction = introduction
	return b
}
func (b *UserBuilder) WithCreatedAt(t time.Time) *UserBuilder {
	b.u.createdAt = t
	return b
}
func (b *UserBuilder) WithUpdatedAt(t time.Time) *UserBuilder {
	b.u.updatedAt = t
	return b
}

// WithPassword 设置密码（自动加密）
func (b *UserBuilder) WithPassword(password string) *UserBuilder {
	// 如果密码为空，直接设置空密码（用于从数据库读取的场景）
	if password == "" {
		b.u.password = ""
		return b
	}

	// 使用 bcrypt 加密密码
	hashedPassword, err := auth.Encrypt(password)
	if err != nil {
		// 在builder中处理错误的方式，可以存储错误状态
		// 这里简化处理，实际项目中可能需要更复杂的错误处理
		b.u.password = "" // 设置为空表示错误
		return b
	}
	b.u.password = hashedPassword
	return b
}

func (b *UserBuilder) Build() *User {
	return b.u
}
