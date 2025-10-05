package user

import (
	"time"

	"github.com/fangcun-mount/qs-server/pkg/util/idutil"
	"gorm.io/gorm"

	base "github.com/fangcun-mount/qs-server/internal/apiserver/infrastructure/mysql"
)

// UserPO 用户持久化对象
// 对应数据库表结构
type UserPO struct {
	base.AuditFields
	Username     string `gorm:"uniqueIndex;column:username;type:varchar(50)" json:"username"`
	Nickname     string `gorm:"column:nickname;type:varchar(50)" json:"nickname"`
	Avatar       string `gorm:"column:avatar;type:varchar(255)" json:"avatar"`
	Phone        string `gorm:"column:phone;type:varchar(20)" json:"phone"`
	Introduction string `gorm:"column:introduction;type:varchar(255)" json:"introduction"`
	Email        string `gorm:"uniqueIndex;column:email;type:varchar(100)" json:"email"`
	Password     string `gorm:"column:password;type:varchar(255)" json:"-"`
	Status       uint8  `gorm:"column:status;type:tinyint;default:0" json:"status"`
}

// TableName 指定表名
func (UserPO) TableName() string {
	return "users"
}

// BeforeCreate 在创建前设置信息
func (p *UserPO) BeforeCreate(tx *gorm.DB) error {
	p.ID = idutil.GetIntID()
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	p.CreatedBy = 0
	p.UpdatedBy = 0
	p.DeletedBy = 0

	return nil
}

// BeforeUpdate 在更新前设置信息
func (p *UserPO) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = time.Now()
	p.UpdatedBy = 0

	return nil
}
