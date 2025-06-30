package apiserver

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/container"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/middleware"
	"github.com/yshujie/questionnaire-scale/internal/pkg/middleware/auth"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// 使用已存在的常量 APIServerAudience 和 APIServerIssuer

// LoginInfo 登录信息
type LoginInfo struct {
	Username string `form:"username" json:"username" binding:"required"`
	Password string `form:"password" json:"password" binding:"required"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	container   *container.Container
	authService *user.AuthService
}

// NewAuthConfig 创建认证配置
func NewAuthConfig(container *container.Container) *AuthConfig {
	authService := container.GetUserModule().GetAuthService()
	return &AuthConfig{
		container:   container,
		authService: authService,
	}
}

// NewBasicAuth 创建Basic认证策略
func (cfg *AuthConfig) NewBasicAuth() middleware.AuthStrategy {
	return auth.NewBasicStrategy(func(username string, password string) bool {
		ctx := context.Background()
		userResponse, err := cfg.authService.ValidatePasswordOnly(ctx, username, password)
		if err != nil {
			log.Warnf("Basic auth failed for user %s: %v", username, err)
			return false
		}

		// 检查用户状态
		isActive, err := cfg.authService.IsUserActive(ctx, username)
		if err != nil || !isActive {
			log.Warnf("User %s is not active", username)
			return false
		}

		log.Infof("Basic auth successful for user: %s", userResponse.Username)
		return true
	})
}

// NewJWTAuth 创建JWT认证策略
func (cfg *AuthConfig) NewJWTAuth() middleware.AuthStrategy {
	ginjwt, _ := jwt.New(&jwt.GinJWTMiddleware{
		Realm:            viper.GetString("jwt.realm"),
		SigningAlgorithm: "HS256",
		Key:              []byte(viper.GetString("jwt.key")),
		Timeout:          viper.GetDuration("jwt.timeout"),
		MaxRefresh:       viper.GetDuration("jwt.max-refresh"),
		Authenticator:    cfg.createAuthenticator(),
		LoginResponse:    cfg.createLoginResponse(),
		LogoutResponse: func(c *gin.Context, code int) {
			c.JSON(http.StatusOK, gin.H{"message": "Successfully logged out"})
		},
		RefreshResponse: cfg.createRefreshResponse(),
		PayloadFunc:     cfg.createPayloadFunc(),
		IdentityHandler: func(c *gin.Context) interface{} {
			claims := jwt.ExtractClaims(c)
			return claims[jwt.IdentityKey]
		},
		IdentityKey:  middleware.UsernameKey,
		Authorizator: cfg.createAuthorizator(),
		Unauthorized: func(c *gin.Context, code int, message string) {
			c.JSON(code, gin.H{
				"code":    code,
				"message": message,
			})
		},
		TokenLookup:   "header: Authorization, query: token, cookie: jwt",
		TokenHeadName: "Bearer",
		SendCookie:    true,
		TimeFunc:      time.Now,
	})

	return auth.NewJWTStrategy(*ginjwt)
}

// NewAutoAuth 创建自动认证策略
func (cfg *AuthConfig) NewAutoAuth() middleware.AuthStrategy {
	return auth.NewAutoStrategy(
		cfg.NewBasicAuth().(auth.BasicStrategy),
		cfg.NewJWTAuth().(auth.JWTStrategy),
	)
}

// createAuthenticator 创建认证器
func (cfg *AuthConfig) createAuthenticator() func(c *gin.Context) (interface{}, error) {
	return func(c *gin.Context) (interface{}, error) {
		var login LoginInfo
		var err error

		// 支持Header和Body两种方式
		if c.Request.Header.Get("Authorization") != "" {
			login, err = cfg.parseWithHeader(c)
		} else {
			login, err = cfg.parseWithBody(c)
		}
		if err != nil {
			return "", jwt.ErrFailedAuthentication
		}

		// 使用AuthService进行认证
		ctx := c.Request.Context()
		authReq := user.AuthenticateRequest{
			Username: login.Username,
			Password: login.Password,
		}

		authResp, err := cfg.authService.Authenticate(ctx, authReq)
		if err != nil {
			log.Errorf("Authentication failed for user %s: %v", login.Username, err)
			return "", jwt.ErrFailedAuthentication
		}

		log.Infof("Authentication successful for user: %s", authResp.User.Username)

		// 将用户信息设置到context中，供LoginResponse使用
		c.Set("user", authResp.User)

		return authResp.User, nil
	}
}

// parseWithHeader 解析请求头中的Authorization字段
func (cfg *AuthConfig) parseWithHeader(c *gin.Context) (LoginInfo, error) {
	authHeader := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
	if len(authHeader) != 2 || authHeader[0] != "Basic" {
		log.Errorf("Invalid Authorization header format")
		return LoginInfo{}, jwt.ErrFailedAuthentication
	}

	payload, err := base64.StdEncoding.DecodeString(authHeader[1])
	if err != nil {
		log.Errorf("Failed to decode basic auth string: %v", err)
		return LoginInfo{}, jwt.ErrFailedAuthentication
	}

	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		log.Errorf("Invalid basic auth payload format")
		return LoginInfo{}, jwt.ErrFailedAuthentication
	}

	return LoginInfo{
		Username: pair[0],
		Password: pair[1],
	}, nil
}

// parseWithBody 解析请求体中的登录信息
func (cfg *AuthConfig) parseWithBody(c *gin.Context) (LoginInfo, error) {
	var login LoginInfo
	if err := c.ShouldBindJSON(&login); err != nil {
		log.Errorf("Failed to parse login parameters: %v", err)
		return LoginInfo{}, jwt.ErrFailedAuthentication
	}

	return login, nil
}

// createLoginResponse 创建登录响应
func (cfg *AuthConfig) createLoginResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		// 从context中获取用户信息
		userInterface, exists := c.Get("user")
		var userData interface{}
		if exists {
			userData = userInterface
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"token":   token,
			"expire":  expire.Format(time.RFC3339),
			"user":    userData,
			"message": "Login successful",
		})
	}
}

// createRefreshResponse 创建刷新响应
func (cfg *AuthConfig) createRefreshResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"code":   code,
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

// createPayloadFunc 创建负载函数
func (cfg *AuthConfig) createPayloadFunc() func(data interface{}) jwt.MapClaims {
	return func(data interface{}) jwt.MapClaims {
		APIServerIssuer := "questionnaire-scale-apiserver"
		APIServerAudience := "questionnaire-scale.com"
		claims := jwt.MapClaims{
			"iss": APIServerIssuer,
			"aud": APIServerAudience,
		}

		if user, ok := data.(*port.UserResponse); ok {
			claims[jwt.IdentityKey] = user.Username
			claims["sub"] = user.Username
			claims["user_id"] = user.ID
			claims["nickname"] = user.Nickname
		}

		return claims
	}
}

// createAuthorizator 创建授权器
func (cfg *AuthConfig) createAuthorizator() func(data interface{}, c *gin.Context) bool {
	return func(data interface{}, c *gin.Context) bool {
		if username, ok := data.(string); ok {
			log.L(c).Infof("User `%s` is authorized.", username)

			// 将用户名设置到上下文中
			c.Set(middleware.UsernameKey, username)

			// 可以在这里添加更多的授权逻辑
			// 例如：检查用户权限、角色等

			return true
		}

		return false
	}
}

// CreateAuthMiddleware 创建认证中间件
// 这是一个便捷方法，用于在路由中设置认证中间件
func (cfg *AuthConfig) CreateAuthMiddleware(authType string) gin.HandlerFunc {
	switch strings.ToLower(authType) {
	case "basic":
		return cfg.NewBasicAuth().AuthFunc()
	case "jwt":
		return cfg.NewJWTAuth().AuthFunc()
	case "auto":
		return cfg.NewAutoAuth().AuthFunc()
	default:
		// 默认使用自动认证
		return cfg.NewAutoAuth().AuthFunc()
	}
}
