package user

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
)

// UserMapper 用户映射器
// 负责领域模型与数据库实体之间的转换
type UserMapper struct{}

// NewUserMapper 创建用户映射器
func NewUserMapper() *UserMapper {
	return &UserMapper{}
}

// ToEntity 将领域模型转换为数据库实体
func (m *UserMapper) ToEntity(domainUser *user.User) *UserEntity {
	if domainUser == nil {
		return nil
	}

	// 先创建实体（不包含嵌入字段的成员）
	entity := &UserEntity{
		Username:     domainUser.Username(),
		Nickname:     domainUser.Nickname(),
		Avatar:       domainUser.Avatar(),
		Phone:        domainUser.Phone(),
		Introduction: domainUser.Introduction(),
		Email:        domainUser.Email(),
		Password:     domainUser.Password(),
		Status:       domainUser.Status().Value(),
	}

	// 然后设置嵌入字段的成员
	entity.ID = domainUser.ID().Value()
	entity.CreatedAt = domainUser.CreatedAt()
	entity.UpdatedAt = domainUser.UpdatedAt()

	return entity
}

// ToDomain 将数据库实体转换为领域模型
func (m *UserMapper) ToDomain(entity *UserEntity) *user.User {
	if entity == nil {
		return nil
	}

	userObj := user.NewUserBuilder().
		WithID(user.NewUserID(entity.ID)).
		WithUsername(entity.Username).
		WithNickname(entity.Nickname).
		WithAvatar(entity.Avatar).
		WithEmail(entity.Email).
		WithPhone(entity.Phone).
		WithIntroduction(entity.Introduction).
		WithStatus(user.Status(entity.Status)).
		WithCreatedAt(entity.CreatedAt).
		WithUpdatedAt(entity.UpdatedAt).
		Build()

	// 直接设置已加密的密码，不需要重新加密
	userObj.SetPassword(entity.Password)

	return userObj
}

// ToEntityList 将领域模型列表转换为实体列表
func (m *UserMapper) ToEntityList(domainUsers []*user.User) []*UserEntity {
	entities := make([]*UserEntity, 0, len(domainUsers))
	for _, domainUser := range domainUsers {
		if entity := m.ToEntity(domainUser); entity != nil {
			entities = append(entities, entity)
		}
	}
	return entities
}

// ToDomainList 将实体列表转换为领域模型列表
func (m *UserMapper) ToDomainList(entities []*UserEntity) []*user.User {
	domainUsers := make([]*user.User, 0, len(entities))
	for _, entity := range entities {
		if domainUser := m.ToDomain(entity); domainUser != nil {
			domainUsers = append(domainUsers, domainUser)
		}
	}
	return domainUsers
}
