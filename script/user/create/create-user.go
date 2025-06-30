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

// UserCreateScript 用户创建脚本 - 实现 ScriptRunner 接口
type UserCreateScript struct {
	template *base.ScriptTemplate
	db       *gorm.DB
	userRepo port.UserRepository
	creator  port.UserCreator
	users    []port.UserCreateRequest
	stats    *CreateStats
}

// CreateStats 创建统计信息
type CreateStats struct {
	Total   int
	Success int
	Failed  int
}

// 要创建的用户数据 - 在这里维护需要创建的用户
var usersToCreate = []port.UserCreateRequest{
	{
		Username:     "admin",
		Password:     "admin123456",
		Nickname:     "系统管理员",
		Email:        "admin@questionnaire.com",
		Phone:        "13800000001",
		Introduction: "系统管理员账户",
	},
	{
		Username:     "testuser",
		Password:     "test123456",
		Nickname:     "测试用户",
		Email:        "test@questionnaire.com",
		Phone:        "13800000002",
		Introduction: "用于测试的用户账户",
	},
	{
		Username:     "demo",
		Password:     "demo123456",
		Nickname:     "演示用户",
		Email:        "demo@questionnaire.com",
		Phone:        "13800000003",
		Introduction: "演示和展示用的用户账户",
	},
	// 可以在这里添加更多用户...
}

// NewUserCreateScript 创建用户创建脚本实例
func NewUserCreateScript() *UserCreateScript {
	return &UserCreateScript{
		users: usersToCreate,
		stats: &CreateStats{
			Total: len(usersToCreate),
		},
	}
}

// Initialize 初始化运行环境（模版方法第一阶段）
func (script *UserCreateScript) Initialize() error {
	log.Info("🔧 初始化用户创建脚本")

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
	script.creator = userApp.NewUserCreator(script.userRepo)

	log.Infof("📝 准备创建 %d 个用户", script.stats.Total)
	return nil
}

// Execute 执行业务操作（模版方法第二阶段）
func (script *UserCreateScript) Execute() error {
	log.Info("👥 开始批量创建用户")

	ctx := context.Background()

	for i, userReq := range script.users {
		log.Infof("📝 创建用户 %d/%d: %s (%s)",
			i+1, script.stats.Total, userReq.Username, userReq.Nickname)

		userResp, err := script.creator.CreateUser(ctx, userReq)
		if err != nil {
			log.Errorf("   ❌ 创建失败: %v", err)
			script.stats.Failed++
		} else {
			log.Infof("   ✅ 创建成功 - ID: %d, 邮箱: %s, 状态: %s",
				userResp.ID, userResp.Email, userResp.Status)
			script.stats.Success++
		}
	}

	return nil
}

// Finalize 执行完毕后的清理操作（模版方法第三阶段）
func (script *UserCreateScript) Finalize() error {
	log.Info("📊 输出创建结果统计")

	fmt.Println()
	fmt.Println("📊 创建结果统计:")
	fmt.Printf("   ✅ 成功: %d 个\n", script.stats.Success)
	fmt.Printf("   ❌ 失败: %d 个\n", script.stats.Failed)
	fmt.Printf("   📋 总计: %d 个\n", script.stats.Total)

	if script.stats.Failed == 0 {
		log.Info("🎉 所有用户创建完成！")
	} else {
		log.Warnf("⚠️ 有 %d 个用户创建失败，请检查错误信息", script.stats.Failed)
	}

	// 可以在这里添加其他清理操作，比如：
	// - 发送通知邮件
	// - 更新统计表
	// - 清理临时文件等

	return nil
}

func main() {
	fmt.Println("🚀 批量创建用户工具")
	fmt.Println()

	// 创建脚本实例
	script := NewUserCreateScript()

	// 创建脚本模版
	template := base.NewScriptTemplate("create-user", &base.InitOptions{
		EnableMySQL: true,
		LogLevel:    "info",
	})
	script.template = template

	// 使用模版方法运行脚本
	if err := template.Run(script); err != nil {
		log.Fatalf("❌ 脚本运行失败: %v", err)
	}
}
