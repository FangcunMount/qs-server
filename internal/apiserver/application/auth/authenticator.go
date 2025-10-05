package auth

import (
	"context"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/port"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/auth"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// Authenticator 认证器
type Authenticator struct {
	userRepo port.UserRepository
}

// NewAuthenticator 创建认证器
func NewAuthenticator(userRepo port.UserRepository) port.Authenticator {
	return &Authenticator{
		userRepo: userRepo,
	}
}

// Authenticate 认证用户
func (a *Authenticator) Authenticate(ctx context.Context, username, password string) (*user.User, error) {
	// 1. 根据用户名查找用户
	userObj, err := a.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, errors.WithCode(code.ErrUserNotFound, "user not found")
	}

	// 2. 验证密码 - 使用与用户创建时一致的bcrypt算法
	if err := auth.Compare(userObj.Password(), password); err != nil {
		return nil, errors.WithCode(code.ErrPasswordIncorrect, "password incorrect")
	}

	// 3. 返回用户对象，token由gin-jwt中间件生成
	// 这里不再生成token，因为gin-jwt会用正确的密钥重新生成
	return userObj, nil // 空字符串表示不生成token
}
