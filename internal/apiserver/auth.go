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

	"github.com/yshujie/questionnaire-scale/internal/apiserver/container"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/internal/pkg/middleware"
	auth "github.com/yshujie/questionnaire-scale/internal/pkg/middleware/auth"
	authStrategys "github.com/yshujie/questionnaire-scale/internal/pkg/middleware/auth/strategys"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// 使用已存在的常量 APIServerAudience 和 APIServerIssuer

// LoginInfo 登录信息
type LoginInfo struct {
	Username string `form:"username" json:"username" binding:"required"`
	Password string `form:"password" json:"password" binding:"required"`
}

// Auth 认证
type Auth struct {
	container     *container.Container
	authenticator port.Authenticator
}

// NewAuth 创建认证
func NewAuth(container *container.Container) *Auth {
	authenticator := container.AuthModule.Authenticator
	return &Auth{
		container:     container,
		authenticator: authenticator,
	}
}

// NewBasicAuth 创建Basic认证策略
func (cfg *Auth) NewBasicAuth() authStrategys.BasicStrategy {
	return authStrategys.NewBasicStrategy(func(username string, password string) bool {
		// 使用Authenticator进行认证
		authResp, err := cfg.authenticator.Authenticate(context.Background(), port.AuthenticateRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			log.Warnf("Basic auth failed for user %s: %v", username, err)
			return false
		}
		log.Infof("Basic auth successful for user: %s", authResp.User.Username)
		return true
	})
}

// NewJWTAuth 创建JWT认证策略
func (cfg *Auth) NewJWTAuth() authStrategys.JWTStrategy {
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

	return authStrategys.NewJWTStrategy(*ginjwt)
}

// NewAutoAuth 创建自动认证策略
func (cfg *Auth) NewAutoAuth() auth.AutoStrategy {
	return auth.NewAutoStrategy(
		cfg.NewBasicAuth(),
		cfg.NewJWTAuth(),
	)
}

// createAuthenticator 创建认证器
func (cfg *Auth) createAuthenticator() func(c *gin.Context) (interface{}, error) {
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
		authReq := port.AuthenticateRequest{
			Username: login.Username,
			Password: login.Password,
		}

		authResp, err := cfg.authenticator.Authenticate(ctx, authReq)
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
func (cfg *Auth) parseWithHeader(c *gin.Context) (LoginInfo, error) {
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
func (cfg *Auth) parseWithBody(c *gin.Context) (LoginInfo, error) {
	var login LoginInfo
	if err := c.ShouldBindJSON(&login); err != nil {
		log.Errorf("Failed to parse login parameters: %v", err)
		return LoginInfo{}, jwt.ErrFailedAuthentication
	}

	return login, nil
}

// createLoginResponse 创建登录响应
func (cfg *Auth) createLoginResponse() func(c *gin.Context, code int, token string, expire time.Time) {
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
func (cfg *Auth) createRefreshResponse() func(c *gin.Context, code int, token string, expire time.Time) {
	return func(c *gin.Context, code int, token string, expire time.Time) {
		c.JSON(http.StatusOK, gin.H{
			"code":   code,
			"token":  token,
			"expire": expire.Format(time.RFC3339),
		})
	}
}

// createPayloadFunc 创建负载函数
func (cfg *Auth) createPayloadFunc() func(data interface{}) jwt.MapClaims {
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
func (cfg *Auth) createAuthorizator() func(data interface{}, c *gin.Context) bool {
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
func (cfg *Auth) CreateAuthMiddleware(authType string) gin.HandlerFunc {
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
