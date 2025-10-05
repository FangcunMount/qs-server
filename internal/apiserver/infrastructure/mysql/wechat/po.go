package wechat

import (
	"time"
)

// AppPO 微信应用持久化对象
type AppPO struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name           string    `gorm:"column:name;type:varchar(128);not null;comment:应用名称"`
	Platform       string    `gorm:"column:platform;type:varchar(16);not null;index:idx_platform_appid;comment:平台类型:mini/oa"`
	AppID          string    `gorm:"column:app_id;type:varchar(64);not null;uniqueIndex:idx_platform_appid;comment:微信AppID"`
	Secret         string    `gorm:"column:secret;type:varchar(128);not null;comment:微信AppSecret"`
	Token          string    `gorm:"column:token;type:varchar(128);comment:服务器配置Token"`
	EncodingAESKey string    `gorm:"column:encoding_aes_key;type:varchar(256);comment:消息加密密钥"`
	MchID          string    `gorm:"column:mch_id;type:varchar(64);comment:商户号"`
	SerialNo       string    `gorm:"column:serial_no;type:varchar(128);comment:商户证书序列号"`
	PayCertID      *uint64   `gorm:"column:pay_cert_id;comment:支付证书ID"`
	Env            string    `gorm:"column:env;type:varchar(16);not null;default:prod;comment:环境:prod/test/dev"`
	IsEnabled      bool      `gorm:"column:is_enabled;type:tinyint(1);not null;default:1;comment:是否启用"`
	Remark         string    `gorm:"column:remark;type:varchar(500);comment:备注"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt      time.Time `gorm:"column:deleted_at;index"`
	CreatedBy      uint64    `gorm:"column:created_by"`
	UpdatedBy      uint64    `gorm:"column:updated_by"`
	DeletedBy      uint64    `gorm:"column:deleted_by"`
}

// TableName 表名
func (AppPO) TableName() string {
	return "wx_apps"
}

// 实现 Syncable 接口
func (po *AppPO) GetID() uint64            { return po.ID }
func (po *AppPO) GetCreatedAt() time.Time  { return po.CreatedAt }
func (po *AppPO) GetUpdatedAt() time.Time  { return po.UpdatedAt }
func (po *AppPO) GetDeletedAt() time.Time  { return po.DeletedAt }
func (po *AppPO) GetCreatedBy() uint64     { return po.CreatedBy }
func (po *AppPO) GetUpdatedBy() uint64     { return po.UpdatedBy }
func (po *AppPO) GetDeletedBy() uint64     { return po.DeletedBy }
func (po *AppPO) SetID(id uint64)          { po.ID = id }
func (po *AppPO) SetCreatedAt(t time.Time) { po.CreatedAt = t }
func (po *AppPO) SetUpdatedAt(t time.Time) { po.UpdatedAt = t }
func (po *AppPO) SetDeletedAt(t time.Time) { po.DeletedAt = t }
func (po *AppPO) SetCreatedBy(id uint64)   { po.CreatedBy = id }
func (po *AppPO) SetUpdatedBy(id uint64)   { po.UpdatedBy = id }
func (po *AppPO) SetDeletedBy(id uint64)   { po.DeletedBy = id }
