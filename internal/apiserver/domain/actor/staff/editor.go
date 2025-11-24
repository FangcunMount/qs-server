package staff

// Editor 员工编辑器领域服务
// 负责 Staff 基础信息的编辑，包含业务规则验证
type Editor interface {
	// UpdateBasicInfo 更新基本信息（姓名）
	// 参数使用指针表示可选更新
	UpdateBasicInfo(staff *Staff, name *string) error

	// UpdateContactInfo 更新联系信息（邮箱、手机号）
	// 参数使用指针表示可选更新
	UpdateContactInfo(staff *Staff, email *string, phone *string) error
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

// UpdateBasicInfo 更新基本信息
func (e *editor) UpdateBasicInfo(staff *Staff, name *string) error {
	// 验证并更新姓名
	if name != nil {
		if err := e.validator.ValidateName(*name, false); err != nil {
			return err
		}
		if *name != "" {
			staff.name = *name
		}
	}

	return nil
}

// UpdateContactInfo 更新联系信息
func (e *editor) UpdateContactInfo(staff *Staff, email *string, phone *string) error {
	// 验证并更新邮箱
	if email != nil {
		if err := e.validator.ValidateEmail(*email); err != nil {
			return err
		}
		if *email != "" {
			staff.email = *email
		}
	}

	// 验证并更新手机号
	if phone != nil {
		if err := e.validator.ValidatePhone(*phone); err != nil {
			return err
		}
		if *phone != "" {
			staff.phone = *phone
		}
	}

	return nil
}
