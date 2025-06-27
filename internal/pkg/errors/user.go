package errors

import "net/http"

// 用户相关错误码 (11xxxx)
const (
	// ErrUserNotFound - 用户不存在
	ErrUserNotFound int = iota + 110000

	// ErrUserAlreadyExists - 用户已存在
	ErrUserAlreadyExists
	// ErrUsernameAlreadyExists - 用户名已存在
	ErrUsernameAlreadyExists
	// ErrEmailAlreadyExists - 邮箱已存在
	ErrEmailAlreadyExists
	// ErrUserInvalidPassword - 密码错误
	ErrUserInvalidPassword
	// ErrUserPasswordTooWeak - 密码强度不够
	ErrUserPasswordTooWeak
	// ErrUserInvalidAvatar - 头像无效
	ErrUserInvalidAvatar
	// ErrUserBlocked - 用户被封禁
	ErrUserBlocked
	// ErrUserInactive - 用户未激活
	ErrUserInactive
	// ErrUserPermissionDenied - 用户权限不足
	ErrUserPermissionDenied
	// ErrUserOperationNotAllowed - 用户操作不被允许
	ErrUserOperationNotAllowed
	// ErrUserInvalidStatus - 用户状态无效
	ErrUserInvalidStatus
	// ErrUserValidationFailed - 用户验证失败
	ErrUserValidationFailed
	// ErrUserCreateFailed - 用户创建失败
	ErrUserCreateFailed
	// ErrUserUpdateFailed - 用户更新失败
	ErrUserUpdateFailed
	// ErrUserDeleteFailed - 用户删除失败
	ErrUserDeleteFailed
	// ErrUserPasswordChangeFailed - 用户密码修改失败
	ErrUserPasswordChangeFailed
	// ErrUserActivationFailed - 用户激活失败
	ErrUserActivationFailed
	// ErrUserBlockingFailed - 用户封禁失败
	ErrUserBlockingFailed
	// ErrUserQueryFailed - 用户查询失败
	ErrUserQueryFailed
	// ErrUserInvalidID - 用户ID无效
	ErrUserInvalidID
	// ErrUserInvalidUsername - 用户名无效
	ErrUserInvalidUsername
	// ErrUserInvalidEmail - 邮箱无效
	ErrUserInvalidEmail
	// ErrUserInvalidPhone - 手机号无效
	ErrUserInvalidPhone
	// ErrUserLoginFailed - 用户登录失败
	ErrUserLoginFailed
	// ErrUserLogoutFailed - 用户登出失败
	ErrUserLogoutFailed
	// ErrUserSessionExpired - 用户会话过期
	ErrUserSessionExpired
	// ErrUserAccountLocked - 用户账户被锁定
	ErrUserAccountLocked
	// ErrUserEmailNotVerified - 用户邮箱未验证
	ErrUserEmailNotVerified
	// ErrUserPhoneNotVerified - 用户手机号未验证
	ErrUserPhoneNotVerified
	// ErrUserListQueryFailed - 用户列表查询失败
	ErrUserListQueryFailed
	// ErrUserStatsQueryFailed - 用户统计查询失败
	ErrUserStatsQueryFailed
	// ErrUserInvalidCredentials - 用户凭证无效
	ErrUserInvalidCredentials
	// ErrUserInvalidSortField - 无效的排序字段
	ErrUserInvalidSortField
	// ErrUserInvalidSortDirection - 无效的排序方向
	ErrUserInvalidSortDirection
)

// 用户错误码注册
func init() {
	register(ErrUserNotFound, http.StatusNotFound, "用户不存在", "")
	register(ErrUserAlreadyExists, http.StatusConflict, "用户已存在", "")
	register(ErrUsernameAlreadyExists, http.StatusConflict, "用户名已存在", "")
	register(ErrEmailAlreadyExists, http.StatusConflict, "邮箱已存在", "")
	register(ErrUserInvalidPassword, http.StatusBadRequest, "密码错误", "")
	register(ErrUserPasswordTooWeak, http.StatusBadRequest, "密码强度不够", "")
	register(ErrUserBlocked, http.StatusForbidden, "用户已被封禁", "")
	register(ErrUserInactive, http.StatusForbidden, "用户未激活", "")
	register(ErrUserPermissionDenied, http.StatusForbidden, "用户权限不足", "")
	register(ErrUserOperationNotAllowed, http.StatusForbidden, "操作不被允许", "")
	register(ErrUserInvalidStatus, http.StatusBadRequest, "用户状态无效", "")
	register(ErrUserValidationFailed, http.StatusBadRequest, "用户验证失败", "")
	register(ErrUserCreateFailed, http.StatusInternalServerError, "用户创建失败", "")
	register(ErrUserUpdateFailed, http.StatusInternalServerError, "用户更新失败", "")
	register(ErrUserDeleteFailed, http.StatusInternalServerError, "用户删除失败", "")
	register(ErrUserPasswordChangeFailed, http.StatusInternalServerError, "密码修改失败", "")
	register(ErrUserActivationFailed, http.StatusInternalServerError, "用户激活失败", "")
	register(ErrUserBlockingFailed, http.StatusInternalServerError, "用户封禁失败", "")
	register(ErrUserQueryFailed, http.StatusInternalServerError, "用户查询失败", "")
	register(ErrUserInvalidID, http.StatusBadRequest, "用户ID无效", "")
	register(ErrUserInvalidUsername, http.StatusBadRequest, "用户名格式无效", "")
	register(ErrUserInvalidEmail, http.StatusBadRequest, "邮箱格式无效", "")
	register(ErrUserLoginFailed, http.StatusUnauthorized, "登录失败", "")
	register(ErrUserLogoutFailed, http.StatusInternalServerError, "登出失败", "")
	register(ErrUserSessionExpired, http.StatusUnauthorized, "会话已过期", "")
	register(ErrUserAccountLocked, http.StatusForbidden, "账户已被锁定", "")
	register(ErrUserEmailNotVerified, http.StatusForbidden, "邮箱未验证", "")
	register(ErrUserPhoneNotVerified, http.StatusForbidden, "手机号未验证", "")
	register(ErrUserListQueryFailed, http.StatusInternalServerError, "用户列表查询失败", "")
	register(ErrUserStatsQueryFailed, http.StatusInternalServerError, "用户统计查询失败", "")
	register(ErrUserInvalidCredentials, http.StatusUnauthorized, "用户凭证无效", "")
	register(ErrUserInvalidSortField, http.StatusBadRequest, "无效的排序字段", "")
	register(ErrUserInvalidSortDirection, http.StatusBadRequest, "无效的排序方向", "")
}
