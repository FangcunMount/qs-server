package questionnaire

// Versioning 版本管理领域服务
// 负责问卷版本的自动化管理
//
// 版本规则:
// - 默认版本: 0.0.1 (新建问卷的起始版本)
// - 存草稿: 小版本递增 (例如: 0.0.1 -> 0.0.2, 1.0.1 -> 1.0.2)
// - 发布: 大版本递增 (例如: 0.0.x -> 1.0.1, 1.0.x -> 2.0.1)
// - 发布后再编辑: 保持当前版本不变,存草稿时小版本递增,再次发布时大版本递增
//
// 示例流程:
// 1. 新建问卷: 0.0.1
// 2. 存草稿: 0.0.2, 0.0.3...
// 3. 发布: 1.0.1
// 4. 编辑并存草稿: 1.0.2, 1.0.3...
// 5. 再次发布: 2.0.1
type Versioning struct{}

// InitializeVersion 初始化版本
// 用于新建问卷时设置初始版本为 0.0.1
func (Versioning) InitializeVersion(q *Questionnaire) error {
	// 如果已有版本，不重复初始化
	if !q.version.IsEmpty() {
		return nil
	}

	return q.updateVersion(NewVersion("0.0.1"))
}

// IncrementMinorVersion 递增小版本号
// 在存草稿时调用，自动将小版本号递增
// 例如：0.0.1 -> 0.0.2, 1.0.5 -> 1.0.6
func (Versioning) IncrementMinorVersion(q *Questionnaire) error {
	// 如果版本为空，初始化为 0.0.1
	if q.version.IsEmpty() {
		return q.updateVersion(NewVersion("0.0.1"))
	}

	// 递增小版本
	newVersion := q.version.IncrementMinor()

	// 更新版本
	return q.updateVersion(newVersion)
}

// IncrementMajorVersion 递增大版本号
// 在发布问卷时调用，自动将大版本号递增并重置为 x.0.1
// 例如：0.0.5 -> 1.0.1, 1.0.3 -> 2.0.1
func (Versioning) IncrementMajorVersion(q *Questionnaire) error {
	// 如果版本为空，初始化为 1.0.1 (首次发布)
	if q.version.IsEmpty() {
		return q.updateVersion(NewVersion("1.0.1"))
	}

	// 递增大版本
	newVersion := q.version.IncrementMajor()

	// 更新版本
	return q.updateVersion(newVersion)
}
