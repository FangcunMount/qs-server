package questionnaire

import (
	"time"

	"github.com/yshujie/questionnaire-scale/pkg/util/idutil"
	"gorm.io/gorm"

	base "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mysql"
)

// UserEntity 用户数据库实体
// 对应数据库表结构
type QuestionnaireEntity struct {
	base.AuditFields
	Code        string `gorm:"column:code;type:varchar(255)" json:"code"`
	Title       string `gorm:"column:title;type:varchar(255)" json:"title"`
	Description string `gorm:"column:description;type:varchar(255)" json:"description"`
	ImgUrl      string `gorm:"column:img_url;type:varchar(255)" json:"img_url"`
	Version     uint8  `gorm:"column:version;type:tinyint;default:0" json:"version"`
	Status      uint8  `gorm:"column:status;type:tinyint;default:0" json:"status"`
}

// TableName 指定表名
func (QuestionnaireEntity) TableName() string {
	return "questionnaires"
}

// BeforeCreate 在创建前设置信息
func (e *QuestionnaireEntity) BeforeCreate(tx *gorm.DB) error {
	e.ID = idutil.GetIntID()
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()

	e.CreatedBy = 0
	e.UpdatedBy = 0
	e.DeletedBy = 0

	return nil
}

// BeforeUpdate 在更新前设置信息
func (e *QuestionnaireEntity) BeforeUpdate(tx *gorm.DB) error {
	e.UpdatedAt = time.Now()
	e.UpdatedBy = 0

	return nil
}
