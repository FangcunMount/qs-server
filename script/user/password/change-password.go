package main

import (
	"context"

	userApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/port"
	userInfra "github.com/fangcun-mount/qs-server/internal/apiserver/infrastructure/mysql/user"
	"github.com/fangcun-mount/qs-server/pkg/log"
	"github.com/fangcun-mount/qs-server/script/base"
)

// PasswordChangeScript 密码修改脚本
type PasswordChangeScript struct {
	env             *base.ScriptEnv
	passwordChanger port.PasswordChanger
	query           port.UserQueryer
}

// PasswordResetData 密码重置数据
type PasswordResetData struct {
	Username    string
	NewPassword string
}

// NewPasswordChangeScript 创建密码修改脚本
func NewPasswordChangeScript() *PasswordChangeScript {
	return &PasswordChangeScript{}
}

// Initialize 初始化阶段
func (s *PasswordChangeScript) Initialize() error {
	env, err := base.NewScriptEnv(&base.InitOptions{
		EnableMySQL: true,
		ScriptName:  "change-password",
	})
	if err != nil {
		return err
	}
	s.env = env

	// 初始化用户存储库和服务
	db, err := s.env.GetMySQLDB()
	if err != nil {
		return err
	}
	userRepo := userInfra.NewRepository(db)
	s.passwordChanger = userApp.NewPasswordChanger(userRepo)
	s.query = userApp.NewUserQueryer(userRepo)

	return nil
}

// Execute 执行业务逻辑
func (s *PasswordChangeScript) Execute() error {
	ctx := context.Background()

	// 预设要重置密码的用户数据
	passwordResets := []PasswordResetData{
		{
			Username:    "admin",
			NewPassword: "1q2w3e4r5T@",
		},
		{
			Username:    "testuser",
			NewPassword: "test@1q2w3e4r5T",
		},
		{
			Username:    "demo",
			NewPassword: "demo@1q2w3e4r5T",
		},
	}

	log.Info("开始批量重置用户密码...")

	successCount := 0
	for i, resetData := range passwordResets {
		log.Infof("正在重置密码 [%d/%d]: %s", i+1, len(passwordResets), resetData.Username)

		// 先查询用户
		existingUser, err := s.query.GetUserByUsername(ctx, resetData.Username)
		if err != nil {
			log.Errorf("查询用户失败: %v", err)
			continue
		}

		if existingUser == nil {
			log.Errorf("用户 '%s' 不存在", resetData.Username)
			continue
		}

		// 重置密码（管理员级别重置，不需要验证旧密码）- 使用原始参数
		err = s.passwordChanger.ChangePassword(ctx,
			existingUser.ID().Value(),
			"", // oldPassword 管理员重置，传空字符串
			resetData.NewPassword)
		if err != nil {
			log.Errorf("重置密码失败: %v", err)
			continue
		}

		successCount++
		log.Infof("✅ 用户 '%s' 密码重置成功", resetData.Username)
		log.Infof("   新密码: %s", resetData.NewPassword)
		log.Warn("   ⚠️  请确保用户在首次登录时更改密码")
	}

	log.Infof("批量密码重置完成！成功: %d/%d", successCount, len(passwordResets))
	log.Info("🔐 安全提醒：")
	log.Info("   1. 请及时通知用户新密码")
	log.Info("   2. 建议用户首次登录后立即修改密码")
	log.Info("   3. 确保密码传输过程安全")

	return nil
}

// Finalize 清理阶段
func (s *PasswordChangeScript) Finalize() error {
	if s.env != nil {
		s.env.Close()
	}
	return nil
}

func main() {
	script := NewPasswordChangeScript()
	template := base.NewScriptTemplate("change-password", &base.InitOptions{
		EnableMySQL: true,
		ScriptName:  "change-password",
	})

	if err := template.Run(script); err != nil {
		panic(err)
	}
}
