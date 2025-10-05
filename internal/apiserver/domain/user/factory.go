package user

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// NewUser 创建新用户（完整信息）
func NewUser(username, password, nickname, email, phone string) (*User, error) {
	if username == "" {
		return nil, errors.WithCode(code.ErrUserBasicInfoInvalid, "username cannot be empty")
	}
	if len(password) < 6 {
		return nil, errors.WithCode(code.ErrUserBasicInfoInvalid, "password must be at least 6 characters")
	}

	user := &User{
		username: username,
		nickname: nickname,
		email:    email,
		phone:    phone,
		status:   StatusActive,
	}

	// 加密密码
	if err := user.ChangePassword(password); err != nil {
		return nil, err
	}

	return user, nil
}

// NewUserMinimal 创建最小化用户（用于微信登录自动创建）
func NewUserMinimal() *User {
	return &User{
		status: StatusActive,
	}
}

// NewUserWithPhone 创建带手机号的用户
func NewUserWithPhone(phone string) (*User, error) {
	if phone == "" {
		return nil, errors.WithCode(code.ErrUserBasicInfoInvalid, "phone cannot be empty")
	}

	return &User{
		phone:  phone,
		status: StatusActive,
	}, nil
}

// NewUserWithWechatInfo 创建带微信信息的用户（用于首次微信登录）
func NewUserWithWechatInfo(nickname, avatar string) *User {
	user := &User{
		nickname: nickname,
		avatar:   avatar,
		status:   StatusActive,
	}
	return user
}

// Reconstitute 从持久化数据重建用户聚合根
func Reconstitute(
	id UserID,
	username, password, nickname, avatar, email, phone, introduction string,
	status Status,
	createdAt, updatedAt time.Time,
) *User {
	return &User{
		id:           id,
		username:     username,
		password:     password,
		nickname:     nickname,
		avatar:       avatar,
		email:        email,
		phone:        phone,
		introduction: introduction,
		status:       status,
		createdAt:    createdAt,
		updatedAt:    updatedAt,
	}
}
