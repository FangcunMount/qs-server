package clinician

// Clinician 机构内业务从业者聚合根。
// 它承载医生/咨询师等业务身份，不承载后台 RBAC。
type Clinician struct {
	id            ID
	orgID         int64
	operatorID    *uint64
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
	operatorID *uint64,
	name, department, title string,
	clinicianType Type,
	employeeCode string,
	isActive bool,
) *Clinician {
	var copiedOperatorID *uint64
	if operatorID != nil {
		value := *operatorID
		copiedOperatorID = &value
	}

	return &Clinician{
		orgID:         orgID,
		operatorID:    copiedOperatorID,
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

// OperatorID 获取关联的后台操作者ID。
func (p *Clinician) OperatorID() *uint64 {
	if p.operatorID == nil {
		return nil
	}
	value := *p.operatorID
	return &value
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

// BindOperator 绑定后台操作者。
func (p *Clinician) BindOperator(operatorID uint64) {
	value := operatorID
	p.operatorID = &value
}

// UnbindOperator 解绑后台操作者。
func (p *Clinician) UnbindOperator() {
	p.operatorID = nil
}

// Activate 激活从业者。
func (p *Clinician) Activate() {
	p.isActive = true
}

// Deactivate 停用从业者。
func (p *Clinician) Deactivate() {
	p.isActive = false
}
