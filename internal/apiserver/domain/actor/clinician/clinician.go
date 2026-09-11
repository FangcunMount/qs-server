package clinician

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Clinician 机构内业务从业者聚合根。
// 它承载医生/咨询师等业务身份，不承载后台 RBAC。
type Clinician struct {
	storeID       *uint64
	version       uint32
	id            ID
	orgID         int64
	name          string
	department    string
	title         string
	clinicianType Type
	employeeCode  string
	isActive      bool
}

// NewClinician 创建从业者。
func NewClinician(
	orgID int64,
	name, department, title string,
	clinicianType Type,
	employeeCode string,
	isActive bool,
) *Clinician {
	return &Clinician{
		orgID:         orgID,
		name:          name,
		department:    department,
		title:         title,
		clinicianType: clinicianType,
		employeeCode:  employeeCode,
		isActive:      isActive,
	}
}

// ID 获取从业者ID。
func (p *Clinician) ID() ID {
	return p.id
}

// OrgID 获取机构ID。
func (p *Clinician) OrgID() int64 {
	return p.orgID
}

// Name 获取姓名。
func (p *Clinician) Name() string {
	return p.name
}

// Department 获取科室。
func (p *Clinician) Department() string {
	return p.department
}

// Title 获取职称。
func (p *Clinician) Title() string {
	return p.title
}

// ClinicianType 获取从业者类型。
func (p *Clinician) ClinicianType() Type {
	return p.clinicianType
}

// EmployeeCode 获取工号。
func (p *Clinician) EmployeeCode() string {
	return p.employeeCode
}

// IsActive 是否激活。
func (p *Clinician) IsActive() bool {
	return p.isActive
}

// SetID 设置ID。
func (p *Clinician) SetID(id ID) {
	p.id = id
}

// UpdateProfile 更新从业者业务档案。
func (p *Clinician) UpdateProfile(
	name, department, title string,
	clinicianType Type,
	employeeCode string,
) {
	p.name = name
	p.department = department
	p.title = title
	p.clinicianType = clinicianType
	p.employeeCode = employeeCode
}

// Activate 激活从业者。
func (p *Clinician) Activate() {
	p.isActive = true
}

// Deactivate 停用从业者。
func (p *Clinician) Deactivate() {
	p.isActive = false
}

// StoreID 当前服务门店；nil 仅表示未配置。
func (p *Clinician) StoreID() *uint64 {
	if p.storeID == nil {
		return nil
	}
	v := *p.storeID
	return &v
}
func (p *Clinician) Version() uint32 { return p.version }

// RestoreStore 恢复持久化归属，普通资料更新不得改变此状态。
func (p *Clinician) RestoreStore(id *uint64, version uint32) {
	p.storeID = nil
	if id != nil {
		v := *id
		p.storeID = &v
	}
	p.version = version
}

// AssignStore 维护当前服务门店；停用医生仍可配置，已配置后不可清空。
// 调店产生的入口失效和历史由应用层在同一事务中协调。
func (p *Clinician) AssignStore(target *store.Store, expected uint32) (bool, error) {
	if target == nil || target.ID() == 0 || target.OrgID() != p.orgID {
		return false, errors.WithCode(code.ErrUserNotFound, "store not found in current company")
	}
	if !target.IsActive() {
		return false, errors.WithCode(code.ErrConflict, "target store is inactive")
	}
	if p.storeID != nil && *p.storeID == target.ID() {
		return false, nil
	}
	if expected == 0 || expected != p.version {
		return false, errors.WithCode(code.ErrConflict, "configuration changed; refresh and retry")
	}
	id := target.ID()
	p.storeID = &id
	p.version++
	return true, nil
}
