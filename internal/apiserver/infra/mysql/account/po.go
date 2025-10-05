package account

import (
	"time"
)

// WechatAccountPO 微信账户持久化对象
type WechatAccountPO struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID       *uint64    `gorm:"column:user_id;index;comment:关联的用户ID"`
	AppID        int64      `gorm:"column:app_id;not null;index:idx_app_openid;comment:微信应用ID"`
	WxAppID      string     `gorm:"column:wx_app_id;type:varchar(64);not null;comment:微信AppID"`
	Platform     string     `gorm:"column:platform;type:varchar(16);not null;index:idx_app_openid;comment:平台类型:mini/oa"`
	OpenID       string     `gorm:"column:open_id;type:varchar(128);not null;uniqueIndex:idx_app_openid;comment:微信OpenID"`
	UnionID      *string    `gorm:"column:union_id;type:varchar(128);index;comment:微信UnionID"`
	Nickname     string     `gorm:"column:nickname;type:varchar(128);comment:微信昵称"`
	AvatarURL    string     `gorm:"column:avatar_url;type:varchar(512);comment:微信头像"`
	SessionKey   string     `gorm:"column:session_key;type:varchar(128);comment:SessionKey(小程序)"`
	Followed     bool       `gorm:"column:followed;type:tinyint(1);default:0;comment:是否关注(公众号)"`
	FollowedAt   *time.Time `gorm:"column:followed_at;comment:关注时间"`
	UnfollowedAt *time.Time `gorm:"column:unfollowed_at;comment:取关时间"`
	LastLoginAt  *time.Time `gorm:"column:last_login_at;comment:最近登录时间"`
	IsActive     bool       `gorm:"column:is_active;type:tinyint(1);not null;default:1;comment:是否活跃"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt    time.Time  `gorm:"column:deleted_at;index"`
	CreatedBy    uint64     `gorm:"column:created_by"`
	UpdatedBy    uint64     `gorm:"column:updated_by"`
	DeletedBy    uint64     `gorm:"column:deleted_by"`
}

// TableName 表名
func (WechatAccountPO) TableName() string {
	return "wx_accounts"
}

// 实现 Syncable 接口
func (po *WechatAccountPO) GetID() uint64            { return po.ID }
func (po *WechatAccountPO) GetCreatedAt() time.Time  { return po.CreatedAt }
func (po *WechatAccountPO) GetUpdatedAt() time.Time  { return po.UpdatedAt }
func (po *WechatAccountPO) GetDeletedAt() time.Time  { return po.DeletedAt }
func (po *WechatAccountPO) GetCreatedBy() uint64     { return po.CreatedBy }
func (po *WechatAccountPO) GetUpdatedBy() uint64     { return po.UpdatedBy }
func (po *WechatAccountPO) GetDeletedBy() uint64     { return po.DeletedBy }
func (po *WechatAccountPO) SetID(id uint64)          { po.ID = id }
func (po *WechatAccountPO) SetCreatedAt(t time.Time) { po.CreatedAt = t }
func (po *WechatAccountPO) SetUpdatedAt(t time.Time) { po.UpdatedAt = t }
func (po *WechatAccountPO) SetDeletedAt(t time.Time) { po.DeletedAt = t }
func (po *WechatAccountPO) SetCreatedBy(id uint64)   { po.CreatedBy = id }
func (po *WechatAccountPO) SetUpdatedBy(id uint64)   { po.UpdatedBy = id }
func (po *WechatAccountPO) SetDeletedBy(id uint64)   { po.DeletedBy = id }

// MergeLogPO 账号合并日志持久化对象
type MergeLogPO struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"column:user_id;not null;index;comment:用户ID"`
	AccountID uint64    `gorm:"column:account_id;not null;comment:账户ID"`
	Reason    string    `gorm:"column:reason;type:varchar(32);not null;comment:合并原因:unionid/phone/manual"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt time.Time `gorm:"column:deleted_at;index"`
	CreatedBy uint64    `gorm:"column:created_by"`
	UpdatedBy uint64    `gorm:"column:updated_by"`
	DeletedBy uint64    `gorm:"column:deleted_by"`
}

// TableName 表名
func (MergeLogPO) TableName() string {
	return "account_merge_logs"
}

// 实现 Syncable 接口
func (po *MergeLogPO) GetID() uint64            { return po.ID }
func (po *MergeLogPO) GetCreatedAt() time.Time  { return po.CreatedAt }
func (po *MergeLogPO) GetUpdatedAt() time.Time  { return po.UpdatedAt }
func (po *MergeLogPO) GetDeletedAt() time.Time  { return po.DeletedAt }
func (po *MergeLogPO) GetCreatedBy() uint64     { return po.CreatedBy }
func (po *MergeLogPO) GetUpdatedBy() uint64     { return po.UpdatedBy }
func (po *MergeLogPO) GetDeletedBy() uint64     { return po.DeletedBy }
func (po *MergeLogPO) SetID(id uint64)          { po.ID = id }
func (po *MergeLogPO) SetCreatedAt(t time.Time) { po.CreatedAt = t }
func (po *MergeLogPO) SetUpdatedAt(t time.Time) { po.UpdatedAt = t }
func (po *MergeLogPO) SetDeletedAt(t time.Time) { po.DeletedAt = t }
func (po *MergeLogPO) SetCreatedBy(id uint64)   { po.CreatedBy = id }
func (po *MergeLogPO) SetUpdatedBy(id uint64)   { po.UpdatedBy = id }
func (po *MergeLogPO) SetDeletedBy(id uint64)   { po.DeletedBy = id }
