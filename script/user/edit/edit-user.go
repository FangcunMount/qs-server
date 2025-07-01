package main

import (
	"context"

	userApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user/port"
	userInfra "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mysql/user"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/script/base"
)

// UserEditScript 用户编辑脚本
type UserEditScript struct {
	env    *base.ScriptEnv
	editor port.UserEditor
	query  port.UserQueryer
}

// NewUserEditScript 创建用户编辑脚本
func NewUserEditScript() *UserEditScript {
	return &UserEditScript{}
}

// Initialize 初始化阶段
func (s *UserEditScript) Initialize() error {
	env, err := base.NewScriptEnv(&base.InitOptions{
		EnableMySQL: true,
		ScriptName:  "edit-user",
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
	s.editor = userApp.NewUserEditor(userRepo)
	s.query = userApp.NewUserQueryer(userRepo)

	return nil
}

// Execute 执行业务逻辑
func (s *UserEditScript) Execute() error {
	ctx := context.Background()

	// 预设要编辑的用户数据
	usersToEdit := []struct {
		Username        string
		NewNickname     string
		NewEmail        string
		NewPhone        string
		NewIntroduction string
	}{
		{
			Username:        "admin",
			NewNickname:     "超级管理员",
			NewEmail:        "super.admin@example.com",
			NewPhone:        "13900139000",
			NewIntroduction: "系统超级管理员，拥有所有权限",
		},
		{
			Username:        "testuser",
			NewNickname:     "高级测试员",
			NewEmail:        "senior.test@example.com",
			NewPhone:        "13900139001",
			NewIntroduction: "高级测试账户，用于功能测试",
		},
		{
			Username:        "demo",
			NewNickname:     "产品演示员",
			NewEmail:        "product.demo@example.com",
			NewPhone:        "13900139002",
			NewIntroduction: "产品演示账户，用于客户展示",
		},
	}

	log.Info("开始批量编辑用户信息...")

	successCount := 0
	for i, editData := range usersToEdit {
		log.Infof("正在编辑用户 [%d/%d]: %s", i+1, len(usersToEdit), editData.Username)

		// 先查询用户
		existingUser, err := s.query.GetUserByUsername(ctx, editData.Username)
		if err != nil {
			log.Errorf("查询用户失败: %v", err)
			continue
		}

		if existingUser == nil {
			log.Errorf("用户 '%s' 不存在", editData.Username)
			continue
		}

		log.Infof("原信息 - 昵称: %s, 邮箱: %s, 电话: %s",
			existingUser.Nickname, existingUser.Email, existingUser.Phone)

		// 更新用户信息
		updateReq := port.UserBasicInfoRequest{
			ID:           existingUser.ID,
			Nickname:     editData.NewNickname,
			Email:        editData.NewEmail,
			Phone:        editData.NewPhone,
			Introduction: editData.NewIntroduction,
		}

		// 保存更新
		if _, err := s.editor.UpdateBasicInfo(ctx, updateReq); err != nil {
			log.Errorf("更新用户失败: %v", err)
			continue
		}

		successCount++
		log.Infof("✅ 用户 '%s' 信息更新成功", editData.Username)
		log.Infof("新信息 - 昵称: %s, 邮箱: %s, 电话: %s",
			editData.NewNickname, editData.NewEmail, editData.NewPhone)
	}

	log.Infof("批量编辑完成！成功: %d/%d", successCount, len(usersToEdit))
	return nil
}

// Finalize 清理阶段
func (s *UserEditScript) Finalize() error {
	if s.env != nil {
		s.env.Close()
	}
	return nil
}

func main() {
	script := NewUserEditScript()
	template := base.NewScriptTemplate("edit-user", &base.InitOptions{
		EnableMySQL: true,
		ScriptName:  "edit-user",
	})

	if err := template.Run(script); err != nil {
		panic(err)
	}
}
