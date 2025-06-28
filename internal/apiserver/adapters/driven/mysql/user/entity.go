package user

import (
	"time"

	"github.com/yshujie/questionnaire-scale/pkg/util/idutil"
	"gorm.io/gorm"

	base "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mysql"
)

// UserEntity 用户数据库实体
// 对应数据库表结构
type UserEntity struct {
	base.AuditFields
	Username     string `gorm:"uniqueIndex;column:username;type:varchar(50)" json:"username"`
	Nickname     string `gorm:"column:nickname;type:varchar(50)" json:"nickname"`
	Avatar       string `gorm:"column:avatar;type:varchar(255)" json:"avatar"`
	Phone        string `gorm:"column:phone;type:varchar(20)" json:"phone"`
	Introduction string `gorm:"column:introduction;type:varchar(255)" json:"introduction"`
	Email        string `gorm:"uniqueIndex;column:email;type:varchar(100)" json:"email"`
	Password     string `gorm:"column:password;type:varchar(255)" json:"-"`
	Status       uint8  `gorm:"column:status;type:tinyint;default:1" json:"status"`
}

// TableName 指定表名
func (UserEntity) TableName() string {
	return "users"
}

// BeforeCreate 在创建前设置信息
func (u *UserEntity) BeforeCreate(tx *gorm.DB) error {
	u.ID = idutil.GetIntID()
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()

	return nil
}

// BeforeUpdate 在更新前设置信息
func (u *UserEntity) BeforeUpdate(tx *gorm.DB) error {
	u.UpdatedAt = time.Now()
	return nil
}

// 实现 Syncable 接口
func (u *UserEntity) GetID() uint64            { return u.ID }
func (u *UserEntity) GetCreatedAt() time.Time  { return u.CreatedAt }
func (u *UserEntity) GetUpdatedAt() time.Time  { return u.UpdatedAt }
func (u *UserEntity) SetID(id uint64)          { u.ID = id }
func (u *UserEntity) SetCreatedAt(t time.Time) { u.CreatedAt = t }
func (u *UserEntity) SetUpdatedAt(t time.Time) { u.UpdatedAt = t }
