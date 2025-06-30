package main

import (
	"context"
	"fmt"

	userInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mysql/user"
	userApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/script/base"
	"gorm.io/gorm"
)

// PasswordChangeScript 密码更改脚本 - 实现 ScriptRunner 接口
type PasswordChangeScript struct {
	template        *base.ScriptTemplate
	db              *gorm.DB
	userRepo        port.UserRepository
	userQuery       port.UserQueryer
	passwordChanger port.PasswordChanger
	changeTasks     []PasswordChangeTask
	stats           *ChangeStats
}

// PasswordChangeTask 密码更改任务
type PasswordChangeTask struct {
	Description string // 任务描述
	Username    string // 用户名（用于查找用户）
	UserID      uint64 // 用户ID
	NewPassword string // 新密码
}

// ChangeStats 更改统计信息
type ChangeStats struct {
	Total   int
	Success int
	Failed  int
}

// 要更改密码的用户任务 - 在这里维护需要更改密码的用户
var passwordChangeTasks = []PasswordChangeTask{
	{
		Description: "重置管理员密码",
		Username:    "admin",
		NewPassword: "1q2w3e4r5T@",
	},
	{
		Description: "重置测试用户密码",
		Username:    "testuser",
		NewPassword: "Test123456!",
	},
	{
		Description: "重置演示用户密码",
		Username:    "demo",
		NewPassword: "Demo123456!",
	},
	// 可以在这里添加更多密码更改任务...
}

// NewPasswordChangeScript 创建密码更改脚本实例
func NewPasswordChangeScript() *PasswordChangeScript {
	return &PasswordChangeScript{
		changeTasks: passwordChangeTasks,
		stats: &ChangeStats{
			Total: len(passwordChangeTasks),
		},
	}
}

// Initialize 初始化运行环境（模版方法第一阶段）
func (script *PasswordChangeScript) Initialize() error {
	log.Info("🔧 初始化密码更改脚本")

	// 获取环境实例
	env := script.template.GetEnv()
	if env == nil {
		return fmt.Errorf("无法获取脚本环境")
	}

	// 获取数据库连接
	db, err := env.GetMySQLDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	script.db = db

	// 初始化用户仓储和服务
	script.userRepo = userInfra.NewRepository(db)
	script.userQuery = userApp.NewUserQueryer(script.userRepo)
	script.passwordChanger = userApp.NewPasswordChanger(script.userRepo)

	log.Infof("🔐 准备执行 %d 个密码更改任务", script.stats.Total)
	return nil
}

// Execute 执行业务操作（模版方法第二阶段）
func (script *PasswordChangeScript) Execute() error {
	log.Info("🔑 开始批量更改用户密码")

	ctx := context.Background()

	for i, task := range script.changeTasks {
		log.Infof("🔐 执行密码更改任务 %d/%d: %s (用户: %s)",
			i+1, script.stats.Total, task.Description, task.Username)

		// 根据用户名查找用户ID
		if task.UserID == 0 && task.Username != "" {
			userResp, err := script.userQuery.GetUserByUsername(ctx, task.Username)
			if err != nil {
				log.Errorf("   ❌ 查找用户失败: %v", err)
				script.stats.Failed++
				continue
			}
			task.UserID = userResp.ID
		}

		// 执行密码更改
		changeReq := port.UserPasswordChangeRequest{
			ID:          task.UserID,
			NewPassword: task.NewPassword,
			// 注意：这里没有设置 OldPassword，因为脚本是管理员操作
		}

		err := script.passwordChanger.ChangePassword(ctx, changeReq)
		if err != nil {
			log.Errorf("   ❌ 密码更改失败: %v", err)
			script.stats.Failed++
		} else {
			log.Infof("   ✅ 密码更改成功 - 用户: %s (ID: %d)",
				task.Username, task.UserID)
			log.Infof("      🔑 新密码: %s", task.NewPassword)
			script.stats.Success++
		}
	}

	return nil
}

// Finalize 执行完毕后的清理操作（模版方法第三阶段）
func (script *PasswordChangeScript) Finalize() error {
	log.Info("📊 输出密码更改结果统计")

	fmt.Println()
	fmt.Println("📊 密码更改结果统计:")
	fmt.Printf("   ✅ 成功: %d 个\n", script.stats.Success)
	fmt.Printf("   ❌ 失败: %d 个\n", script.stats.Failed)
	fmt.Printf("   📋 总计: %d 个\n", script.stats.Total)

	if script.stats.Failed == 0 {
		log.Info("🎉 所有密码更改完成！")
		fmt.Println()
		fmt.Println("🔐 新密码信息:")
		for _, task := range script.changeTasks {
			fmt.Printf("   🔑 用户: %s -> 新密码: %s\n", task.Username, task.NewPassword)
		}
	} else {
		log.Warnf("⚠️ 有 %d 个密码更改任务失败，请检查错误信息", script.stats.Failed)
	}

	// 安全提示
	fmt.Println()
	fmt.Println("🔒 安全提示:")
	fmt.Println("   ⚠️ 请确保新密码的安全性")
	fmt.Println("   ⚠️ 建议用户在首次登录后立即更改密码")
	fmt.Println("   ⚠️ 请妥善保管密码信息，避免泄露")

	// 可以在这里添加其他清理操作，比如：
	// - 发送密码更改通知邮件
	// - 记录安全审计日志
	// - 强制用户下次登录时更改密码等

	return nil
}

func main() {
	fmt.Println("🔑 批量更改用户密码工具")
	fmt.Println()

	// 安全警告
	fmt.Println("⚠️ 安全警告:")
	fmt.Println("   本工具将批量更改用户密码，请确保在安全环境下运行")
	fmt.Println("   请妥善保管新密码信息")
	fmt.Println()

	// 创建脚本实例
	script := NewPasswordChangeScript()

	// 创建脚本模版
	template := base.NewScriptTemplate("change-password", &base.InitOptions{
		EnableMySQL: true,
		LogLevel:    "info",
	})
	script.template = template

	// 使用模版方法运行脚本
	if err := template.Run(script); err != nil {
		log.Fatalf("❌ 脚本运行失败: %v", err)
	}
}
