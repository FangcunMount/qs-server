package main

import (
	"context"

	userApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	userInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mysql/user"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/script/base"
)

// UserCreateScript 用户创建脚本
type UserCreateScript struct {
	env     *base.ScriptEnv
	creator port.UserCreator
}

// UserCreateData 用户创建数据
type UserCreateData struct {
	Username     string
	Password     string
	Nickname     string
	Email        string
	Phone        string
	Introduction string
}

// NewUserCreateScript 创建用户创建脚本
func NewUserCreateScript() *UserCreateScript {
	return &UserCreateScript{}
}

// Initialize 初始化阶段
func (s *UserCreateScript) Initialize() error {
	env, err := base.NewScriptEnv(&base.InitOptions{
		EnableMySQL: true,
		ScriptName:  "create-user",
	})
	if err != nil {
		return err
	}
	s.env = env

	// 初始化用户存储库和创建器
	db, err := s.env.GetMySQLDB()
	if err != nil {
		return err
	}
	userRepo := userInfra.NewRepository(db)
	s.creator = userApp.NewUserCreator(userRepo)

	return nil
}

// Execute 执行业务逻辑
func (s *UserCreateScript) Execute() error {
	ctx := context.Background()

	// 预设用户数据
	usersToCreate := []UserCreateData{
		{
			Username:     "admin",
			Password:     "admin123",
			Nickname:     "管理员",
			Email:        "admin@example.com",
			Phone:        "13800138000",
			Introduction: "系统管理员",
		},
		{
			Username:     "testuser",
			Password:     "test123",
			Nickname:     "测试用户",
			Email:        "test@example.com",
			Phone:        "13800138001",
			Introduction: "测试账户",
		},
		{
			Username:     "demo",
			Password:     "demo123",
			Nickname:     "演示用户",
			Email:        "demo@example.com",
			Phone:        "13800138002",
			Introduction: "演示账户",
		},
	}

	log.Info("开始批量创建用户...")

	successCount := 0
	for i, userData := range usersToCreate {
		log.Infof("正在创建用户 [%d/%d]: %s", i+1, len(usersToCreate), userData.Username)

		// 创建用户 - 使用原始参数
		user, err := s.creator.CreateUser(ctx,
			userData.Username,
			userData.Password,
			userData.Nickname,
			userData.Email,
			userData.Phone,
			userData.Introduction)
		if err != nil {
			log.Errorf("创建用户失败: %v", err)
			continue
		}

		successCount++
		log.Infof("✅ 用户 '%s' 创建成功 (ID: %d)", userData.Username, user.ID().Value())
	}

	log.Infof("批量创建完成！成功: %d/%d", successCount, len(usersToCreate))
	return nil
}

// Finalize 清理阶段
func (s *UserCreateScript) Finalize() error {
	if s.env != nil {
		s.env.Close()
	}
	return nil
}

func main() {
	script := NewUserCreateScript()
	template := base.NewScriptTemplate("create-user", &base.InitOptions{
		EnableMySQL: true,
		ScriptName:  "create-user",
	})

	if err := template.Run(script); err != nil {
		panic(err)
	}
}
