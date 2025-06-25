package user

import (
	userDomain "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
)

// Converter 用户转换器
type Converter struct{}

// NewConverter 创建用户转换器
func NewConverter() *Converter {
	return &Converter{}
}

// DomainToModel 将领域对象转换为数据模型
func (c *Converter) DomainToModel(u *userDomain.User) *Model {
	if u == nil {
		return nil
	}

	return &Model{
		ID:        u.ID().Value(),
		Username:  u.Username(),
		Email:     u.Email(),
		Password:  u.Password(),
		Status:    int(u.Status()),
		CreatedAt: u.CreatedAt(),
		UpdatedAt: u.UpdatedAt(),
	}
}

// ModelToDomain 将数据模型转换为领域对象
func (c *Converter) ModelToDomain(model *Model) *userDomain.User {
	if model == nil || model.ID == "" {
		return nil
	}

	// TODO: 这里需要完善，应该通过工厂方法或构造函数创建完整的User对象
	// 目前使用简单的工厂方法
	return userDomain.NewUser(model.Username, model.Email, model.Password)
}
