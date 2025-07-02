package questionnaire

import (
	"time"

	"github.com/yshujie/questionnaire-scale/pkg/util/idutil"
	"gorm.io/gorm"

	base "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mysql"
)

// QuestionnairePO 问卷持久化对象
// 对应数据库表结构
type QuestionnairePO struct {
	base.AuditFields
	ID          uint64 `gorm:"column:id;type:bigint(20) unsigned;primary_key;auto_increment" json:"id"`
	Code        string `gorm:"column:code;type:varchar(255)" json:"code"`
	Title       string `gorm:"column:title;type:varchar(255)" json:"title"`
	Description string `gorm:"column:description;type:varchar(255)" json:"description"`
	ImgUrl      string `gorm:"column:img_url;type:varchar(255)" json:"img_url"`
	Version     string `gorm:"column:version;type:varchar(255);" json:"version"`
	Status      uint8  `gorm:"column:status;type:tinyint;" json:"status"`
}

// TableName 指定表名
func (QuestionnairePO) TableName() string {
	return "questionnaires"
}

// BeforeCreate 在创建前设置信息
func (p *QuestionnairePO) BeforeCreate(tx *gorm.DB) error {
	p.ID = idutil.GetIntID()
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()

	p.CreatedBy = 0
	p.UpdatedBy = 0
	p.DeletedBy = 0

	return nil
}

// BeforeUpdate 在更新前设置信息
func (p *QuestionnairePO) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = time.Now()
	p.UpdatedBy = 0

	return nil
}
