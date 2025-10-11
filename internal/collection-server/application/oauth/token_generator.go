package oauth

import (
    "fmt"

    "github.com/fangcun-mount/qs-server/internal/collection-server/infrastructure/auth"
)

// JWTTokenGenerator JWT Token 生成器
type JWTTokenGenerator struct {
    jwtManager *auth.JWTManager
}

// NewJWTTokenGenerator 创建 JWT Token 生成器
func NewJWTTokenGenerator(jwtManager *auth.JWTManager) *JWTTokenGenerator {
    return &JWTTokenGenerator{
        jwtManager: jwtManager,
    }
}

// GenerateToken 生成 JWT Token
func (g *JWTTokenGenerator) GenerateToken(userID, appID, openID string) (string, error) {
    token, err := g.jwtManager.GenerateToken(userID, appID, openID)
    if err != nil {
        return "", fmt.Errorf("failed to generate JWT token: %w", err)
    }
    return token, nil
}
