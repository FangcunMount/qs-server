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

// UserEditScript 用户编辑脚本 - 实现 ScriptRunner 接口
type UserEditScript struct {
	template   *base.ScriptTemplate
	db         *gorm.DB
	userRepo   port.UserRepository
	userQuery  port.UserQueryer
	userEditor port.UserEditor
	editTasks  []UserEditTask
	stats      *EditStats
}

// UserEditTask 用户编辑任务
type UserEditTask struct {
	Description string                    // 任务描述
	UserID      uint64                    // 要编辑的用户ID
	Updates     port.UserBasicInfoRequest // 更新内容
}

// EditStats 编辑统计信息
type EditStats struct {
	Total   int
	Success int
	Failed  int
}

// 要编辑的用户任务 - 在这里维护需要编辑的用户信息
var userEditTasks = []UserEditTask{
	{
		Description: "更新管理员用户的联系方式",
		UserID:      0, // 将通过用户名查找实际ID
		Updates: port.UserBasicInfoRequest{
			Username:     "admin",       // 用于查找用户
			Phone:        "13900000001", // 新手机号
			Introduction: "系统超级管理员账户",   // 更新简介
		},
	},
	{
		Description: "更新测试用户的邮箱和昵称",
		UserID:      0, // 将通过用户名查找实际ID
		Updates: port.UserBasicInfoRequest{
			Username: "testuser",             // 用于查找用户
			Email:    "testuser@example.com", // 新邮箱
			Nickname: "高级测试用户",               // 新昵称
		},
	},
	{
		Description: "更新演示用户的完整信息",
		UserID:      0, // 将通过用户名查找实际ID
		Updates: port.UserBasicInfoRequest{
			Username:     "demo",
			Nickname:     "演示专用账户",
			Email:        "demo@example.com",
			Phone:        "13900000003",
			Introduction: "用于产品演示和展示的专用账户，具有完整的功能权限",
		},
	},
	// 可以在这里添加更多编辑任务...
}

// NewUserEditScript 创建用户编辑脚本实例
func NewUserEditScript() *UserEditScript {
	return &UserEditScript{
		editTasks: userEditTasks,
		stats: &EditStats{
			Total: len(userEditTasks),
		},
	}
}

// Initialize 初始化运行环境（模版方法第一阶段）
func (script *UserEditScript) Initialize() error {
	log.Info("🔧 初始化用户编辑脚本")

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
	script.userEditor = userApp.NewUserEditor(script.userRepo)

	log.Infof("📝 准备执行 %d 个用户编辑任务", script.stats.Total)
	return nil
}

// Execute 执行业务操作（模版方法第二阶段）
func (script *UserEditScript) Execute() error {
	log.Info("✏️ 开始批量编辑用户")

	ctx := context.Background()

	for i, task := range script.editTasks {
		log.Infof("📝 执行编辑任务 %d/%d: %s",
			i+1, script.stats.Total, task.Description)

		// 根据用户名查找用户ID
		if task.UserID == 0 && task.Updates.Username != "" {
			userResp, err := script.userQuery.GetUserByUsername(ctx, task.Updates.Username)
			if err != nil {
				log.Errorf("   ❌ 查找用户失败: %v", err)
				script.stats.Failed++
				continue
			}
			task.UserID = userResp.ID
			task.Updates.ID = userResp.ID
		}

		// 执行用户信息更新
		updatedUser, err := script.userEditor.UpdateBasicInfo(ctx, task.Updates)
		if err != nil {
			log.Errorf("   ❌ 编辑失败: %v", err)
			script.stats.Failed++
		} else {
			log.Infof("   ✅ 编辑成功 - 用户: %s (%s), 邮箱: %s, 电话: %s",
				updatedUser.Username, updatedUser.Nickname,
				updatedUser.Email, updatedUser.Phone)
			if updatedUser.Introduction != "" {
				log.Infof("      📄 简介: %s", updatedUser.Introduction)
			}
			script.stats.Success++
		}
	}

	return nil
}

// Finalize 执行完毕后的清理操作（模版方法第三阶段）
func (script *UserEditScript) Finalize() error {
	log.Info("📊 输出编辑结果统计")

	fmt.Println()
	fmt.Println("📊 编辑结果统计:")
	fmt.Printf("   ✅ 成功: %d 个\n", script.stats.Success)
	fmt.Printf("   ❌ 失败: %d 个\n", script.stats.Failed)
	fmt.Printf("   📋 总计: %d 个\n", script.stats.Total)

	if script.stats.Failed == 0 {
		log.Info("🎉 所有用户编辑完成！")
	} else {
		log.Warnf("⚠️ 有 %d 个编辑任务失败，请检查错误信息", script.stats.Failed)
	}

	// 可以在这里添加其他清理操作，比如：
	// - 生成编辑报告
	// - 发送变更通知
	// - 记录审计日志等

	return nil
}

func main() {
	fmt.Println("✏️ 批量编辑用户工具")
	fmt.Println()

	// 创建脚本实例
	script := NewUserEditScript()

	// 创建脚本模版
	template := base.NewScriptTemplate("edit-user", &base.InitOptions{
		EnableMySQL: true,
		LogLevel:    "info",
	})
	script.template = template

	// 使用模版方法运行脚本
	if err := template.Run(script); err != nil {
		log.Fatalf("❌ 脚本运行失败: %v", err)
	}
}
