package staff

// Editor 员工编辑器领域服务
// 负责 Staff 信息的变更，包含业务规则验证
type Editor interface {
	// UpdateContactInfo 更新联系信息
	UpdateContactInfo(staff *Staff, email, phone string) error

	// UpdateName 更新姓名
	UpdateName(staff *Staff, name string) error

	// Activate 激活员工
	Activate(staff *Staff) error

	// Deactivate 停用员工
	Deactivate(staff *Staff, reason string) error
}

// editor 编辑器实现
type editor struct {
	validator Validator
}

// NewEditor 创建编辑器
func NewEditor(validator Validator) Editor {
	return &editor{
		validator: validator,
	}
}

// UpdateContactInfo 更新联系信息
func (e *editor) UpdateContactInfo(staff *Staff, email, phone string) error {
	// 验证邮箱
	if err := e.validator.ValidateEmail(email); err != nil {
		return err
	}

	// 验证手机号
	if err := e.validator.ValidatePhone(phone); err != nil {
		return err
	}

	// 执行更新
	staff.updateContactInfo(email, phone)

	return nil
}

// UpdateName 更新姓名
func (e *editor) UpdateName(staff *Staff, name string) error {
	// 验证姓名
	if err := e.validator.ValidateName(name, false); err != nil {
		return err
	}

	if name != "" {
		staff.name = name
	}

	return nil
}

// Activate 激活员工
func (e *editor) Activate(staff *Staff) error {
	if staff.IsActive() {
		return nil // 已激活，幂等操作
	}

	staff.activate()

	// TODO: 发布领域事件
	// events.Publish(NewStaffActivatedEvent(staff.ID()))

	return nil
}

// Deactivate 停用员工
func (e *editor) Deactivate(staff *Staff, reason string) error {
	if !staff.IsActive() {
		return nil // 已停用，幂等操作
	}

	// 业务规则：停用时应清空所有角色
	staff.roles = make([]Role, 0)
	staff.deactivate()

	// TODO: 发布领域事件
	// events.Publish(NewStaffDeactivatedEvent(staff.ID(), reason))

	return nil
}
