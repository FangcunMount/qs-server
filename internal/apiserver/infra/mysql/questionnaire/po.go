package questionnaire

import (
	"time"

	"github.com/fangcun-mount/qs-server/pkg/util/idutil"
	"gorm.io/gorm"

	base "github.com/fangcun-mount/qs-server/internal/apiserver/infra/mysql"
)

// QuestionnairePO 问卷持久化对象
// 对应数据库表结构
type QuestionnairePO struct {
	base.AuditFields
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
	p.AuditFields.ID = idutil.GetIntID()
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
