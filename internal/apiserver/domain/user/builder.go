package user

import "time"

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
func (b *UserBuilder) Build() *User {
	return b.u
}
