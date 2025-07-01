package user

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
)

// UserMapper 用户映射器
// 负责领域模型与持久化对象之间的转换
type UserMapper struct{}

// NewUserMapper 创建用户映射器
func NewUserMapper() *UserMapper {
	return &UserMapper{}
}

// ToPO 将领域模型转换为持久化对象
func (m *UserMapper) ToPO(domainUser *user.User) *UserPO {
	if domainUser == nil {
		return nil
	}

	// 先创建持久化对象（不包含嵌入字段的成员）
	po := &UserPO{
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
	po.ID = domainUser.ID().Value()
	po.CreatedAt = domainUser.CreatedAt()
	po.UpdatedAt = domainUser.UpdatedAt()

	return po
}

// ToBO 将持久化对象转换为业务对象
func (m *UserMapper) ToBO(po *UserPO) *user.User {
	if po == nil {
		return nil
	}

	userObj := user.NewUserBuilder().
		WithID(user.NewUserID(po.ID)).
		WithUsername(po.Username).
		WithNickname(po.Nickname).
		WithAvatar(po.Avatar).
		WithEmail(po.Email).
		WithPhone(po.Phone).
		WithIntroduction(po.Introduction).
		WithStatus(user.Status(po.Status)).
		WithCreatedAt(po.CreatedAt).
		WithUpdatedAt(po.UpdatedAt).
		Build()

	// 直接设置已加密的密码，不需要重新加密
	userObj.SetPassword(po.Password)

	return userObj
}

// ToPOList 将领域模型列表转换为持久化对象列表
func (m *UserMapper) ToPOList(domainUsers []*user.User) []*UserPO {
	pos := make([]*UserPO, 0, len(domainUsers))
	for _, domainUser := range domainUsers {
		if po := m.ToPO(domainUser); po != nil {
			pos = append(pos, po)
		}
	}
	return pos
}

// ToBOList 将持久化对象列表转换为业务对象列表
func (m *UserMapper) ToBOList(pos []*UserPO) []*user.User {
	domainUsers := make([]*user.User, 0, len(pos))
	for _, po := range pos {
		if domainUser := m.ToBO(po); domainUser != nil {
			domainUsers = append(domainUsers, domainUser)
		}
	}
	return domainUsers
}
