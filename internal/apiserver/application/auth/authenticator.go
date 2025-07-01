package auth

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/auth"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// Authenticator 认证器
type Authenticator struct {
	userRepo  port.UserRepository
	secretKey string
}

// NewAuthenticator 创建认证器
func NewAuthenticator(userRepo port.UserRepository, secretKey string) port.Authenticator {
	return &Authenticator{
		userRepo:  userRepo,
		secretKey: secretKey,
	}
}

// Authenticate 认证用户
func (a *Authenticator) Authenticate(ctx context.Context, username, password string) (*user.User, string, error) {
	// 1. 根据用户名查找用户
	userObj, err := a.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, "", errors.WithCode(code.ErrUserNotFound, "user not found")
	}

	// 2. 验证密码 - 使用与用户创建时一致的bcrypt算法
	if err := auth.Compare(userObj.Password(), password); err != nil {
		return nil, "", errors.WithCode(code.ErrPasswordIncorrect, "password incorrect")
	}

	// 3. 生成JWT token
	token, err := a.generateToken(userObj)
	if err != nil {
		return nil, "", err
	}

	return userObj, token, nil
}

// generateToken 生成JWT token
func (a *Authenticator) generateToken(user *user.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  user.ID().Value(),
		"username": user.Username(),
		"exp":      time.Now().Add(time.Hour * 24).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(a.secretKey))
}
