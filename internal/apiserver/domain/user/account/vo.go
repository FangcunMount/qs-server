package account

import "github.com/fangcun-mount/qs-server/pkg/util/idutil"

// AccountID 账户ID值对象
type AccountID = idutil.ID[uint64]

// NewAccountID 创建账户ID
func NewAccountID(value uint64) AccountID {
	return idutil.NewID[uint64](value)
}
