package wechat

import "github.com/fangcun-mount/qs-server/pkg/util/idutil"

// AppID 微信应用ID值对象
type AppID = idutil.ID[uint64]

// NewAppID 创建应用ID
func NewAppID(value uint64) AppID {
	return idutil.NewID[uint64](value)
}
