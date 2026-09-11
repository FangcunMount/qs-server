package testee

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// StoreID 返回当前服务门店的副本。未归属不能解释为全公司访问范围。
func (t *Testee) StoreID() *uint64 {
	if t.storeID == nil {
		return nil
	}
	id := *t.storeID
	return &id
}

// StoreVersion 只保护归属变更；资料编辑不能覆盖归属状态。
func (t *Testee) StoreVersion() uint32 { return t.storeVersion }

// RestoreStore 供仓储恢复归属事实；业务变更应使用首次归属或转店行为。
func (t *Testee) RestoreStore(id *uint64, version uint32) {
	t.storeID = nil
	if id != nil {
		value := *id
		t.storeID = &value
	}
	t.storeVersion = version
}

// AssignInitialStore 仅建立首次归属；扫码不得隐式转店。
// 授权、事务锁、请求幂等与历史记录由应用用例协调。
func (t *Testee) AssignInitialStore(target *store.Store, expected uint32) (bool, error) {
	if err := t.validateStoreChange(target, expected); err != nil {
		return false, err
	}
	if t.storeID != nil {
		if *t.storeID == target.ID() {
			return false, nil
		}
		return false, errors.WithCode(code.ErrConflict, "testee already belongs to another store; explicit transfer required")
	}
	t.setStore(target.ID())
	return true, nil
}

// TransferStore 调整已有归属，不迁移历史业务记录或医生关系。
// 调用方必须经过总部授权入口，不能用于扫码入组。
func (t *Testee) TransferStore(target *store.Store, expected uint32) (bool, error) {
	if err := t.validateStoreChange(target, expected); err != nil {
		return false, err
	}
	if t.storeID == nil {
		return false, errors.WithCode(code.ErrConflict, "testee has no store; initial assignment required")
	}
	if *t.storeID == target.ID() {
		return false, nil
	}
	t.setStore(target.ID())
	return true, nil
}

func (t *Testee) validateStoreChange(target *store.Store, expected uint32) error {
	if target == nil || target.ID() == 0 || t.orgID <= 0 || target.OrgID() != t.orgID {
		return errors.WithCode(code.ErrUserNotFound, "store not found in current company")
	}
	if !target.IsActive() {
		return errors.WithCode(code.ErrConflict, "target store is inactive")
	}
	if expected == 0 || expected != t.storeVersion {
		return errors.WithCode(code.ErrConflict, "store ownership changed; refresh and retry")
	}
	return nil
}

func (t *Testee) setStore(id uint64) {
	t.storeID = &id
	t.storeVersion++
}
