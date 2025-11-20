package actor

// 设计说明：
// FillerRef 用于在 AnswerSheet 中记录"谁操作填写"的元数据。
// 设计动机：区分"被测者"（Testee）和"填写人"（Filler）的不同场景：
//   - 儿童测评：家长/老师代填
//   - 认知障碍：护理人员代填
//   - 自测场景：受试者本人填写
// 当前状态：已设计完成，等待 AnswerSheet 聚合根重构后使用。
// 参考文档：docs/v2/11-03-Testee和Staff用户模型设计-v2.md 第8章

// FillerType 填写动作的角色类型
type FillerType string

const (
	// FillerTypeSelf 受试者本人填写
	FillerTypeSelf FillerType = "self"
	// FillerTypeGuardian 监护人/家长/老师代填
	FillerTypeGuardian FillerType = "guardian"
	// FillerTypeStaff 内部员工代填
	FillerTypeStaff FillerType = "staff"
)

// String 返回字符串表示
func (f FillerType) String() string {
	return string(f)
}

// FillerRef 填写人引用（值对象）
// 用于记录"谁操作填写"的元数据
type FillerRef struct {
	userID     int64      // IAM.UserID
	fillerType FillerType // 填写类型
}

// NewFillerRef 创建填写人引用
func NewFillerRef(userID int64, fillerType FillerType) *FillerRef {
	return &FillerRef{
		userID:     userID,
		fillerType: fillerType,
	}
}

// UserID 获取用户ID
func (f *FillerRef) UserID() int64 {
	return f.userID
}

// FillerType 获取填写类型
func (f *FillerRef) FillerType() FillerType {
	return f.fillerType
}

// IsSelf 是否本人填写
func (f *FillerRef) IsSelf() bool {
	return f.fillerType == FillerTypeSelf
}

// IsGuardian 是否监护人代填
func (f *FillerRef) IsGuardian() bool {
	return f.fillerType == FillerTypeGuardian
}

// IsStaff 是否员工代填
func (f *FillerRef) IsStaff() bool {
	return f.fillerType == FillerTypeStaff
}
