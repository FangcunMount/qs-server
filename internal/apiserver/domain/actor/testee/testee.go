package testee

import "time"

// Testee 受试者聚合根
// 表示"被测评的人"在问卷&量表 BC 内的领域视图，是统计和趋势分析的核心主体
// 设计原则：
// 1. 只包含通用属性，特定场景（如筛查）的属性应在对应子域中维护
// 2. 以行为为中心，而非数据中心，避免成为贫血模型
// 3. 审计字段由基础设施层（PO）处理，领域层不关心
type Testee struct {
	// === 核心标识 ===
	id    ID
	orgID int64 // 所属机构（医院、训练中心、学校等）

	// === 与 IAM 的映射 ===
	iamUserID  *int64 // 可选：绑定 IAM.User（成人患者）
	iamChildID *int64 // 可选：绑定 IAM.Child（儿童档案）

	// === 基本属性 ===
	name     string     // 姓名（可脱敏）
	gender   Gender     // 性别
	birthday *time.Time // 出生日期

	// === 业务标签与关注 ===
	tags       []string // 业务标签：["high_risk", "adhd_suspect", "vip"]
	source     string   // 数据来源：online_form / clinic_import / screening_campaign
	isKeyFocus bool     // 是否重点关注对象

	// === 测评统计快照（读模型优化）===
	// 注意：这些快照数据通过领域事件异步更新，不应直接修改
	assessmentStats *AssessmentStats
}

// NewTestee 创建新的受试者
func NewTestee(
	orgID int64,
	name string,
	gender Gender,
	birthday *time.Time,
) *Testee {
	return &Testee{
		orgID:    orgID,
		name:     name,
		gender:   gender,
		birthday: birthday,
		source:   "unknown",
		tags:     make([]string, 0),
	}
}

// === 核心标识方法 ===

// ID 获取受试者ID
func (t *Testee) ID() ID {
	return t.id
}

// OrgID 获取机构ID
func (t *Testee) OrgID() int64 {
	return t.orgID
}

// === 身份绑定方法（包内可见，通过 Binder 服务使用）===

// bindIAMUser 绑定IAM用户（包内方法，应通过 Binder 调用）
func (t *Testee) bindIAMUser(userID int64) {
	t.iamUserID = &userID
}

// bindIAMChild 绑定IAM儿童（包内方法，应通过 Binder 调用）
func (t *Testee) bindIAMChild(childID int64) {
	t.iamChildID = &childID
}

// IAMUserID 获取绑定的IAM用户ID（用于验证身份）
func (t *Testee) IAMUserID() *int64 {
	return t.iamUserID
}

// IAMChildID 获取绑定的IAM儿童ID（用于验证身份）
func (t *Testee) IAMChildID() *int64 {
	return t.iamChildID
}

// IsBoundToIAM 是否已绑定IAM账号
func (t *Testee) IsBoundToIAM() bool {
	return t.iamUserID != nil || t.iamChildID != nil
}

// === 基本信息方法 ===

// Name 获取姓名（用于显示）
func (t *Testee) Name() string {
	return t.name
}

// Gender 获取性别
func (t *Testee) Gender() Gender {
	return t.gender
}

// Birthday 获取出生日期
func (t *Testee) Birthday() *time.Time {
	return t.birthday
}

// GetAge 计算当前年龄
func (t *Testee) GetAge() int {
	if t.birthday == nil || t.birthday.IsZero() {
		return 0
	}

	now := time.Now()
	age := now.Year() - t.birthday.Year()

	// 如果生日还没过，年龄减1
	if now.Month() < t.birthday.Month() ||
		(now.Month() == t.birthday.Month() && now.Day() < t.birthday.Day()) {
		age--
	}

	if age < 0 {
		return 0
	}

	return age
}

// updateBasicInfo 更新基本信息（包内方法，应通过 Editor 调用）
func (t *Testee) updateBasicInfo(name string, gender Gender, birthday *time.Time) {
	if name != "" {
		t.name = name
	}
	t.gender = gender
	if birthday != nil {
		t.birthday = birthday
	}
}

// === 标签管理方法 ===

// Tags 获取标签列表（返回副本，防止外部修改）
func (t *Testee) Tags() []string {
	tags := make([]string, len(t.tags))
	copy(tags, t.tags)
	return tags
}

// HasTag 检查是否有某个标签
func (t *Testee) HasTag(tag string) bool {
	for _, existing := range t.tags {
		if existing == tag {
			return true
		}
	}
	return false
}

// addTag 添加标签（包内方法，应通过 Editor 调用）
func (t *Testee) addTag(tag string) {
	if tag == "" {
		return
	}
	// 防重复
	if t.HasTag(tag) {
		return
	}
	t.tags = append(t.tags, tag)
}

// removeTag 移除标签（包内方法，应通过 Editor 调用）
func (t *Testee) removeTag(tag string) {
	for i, existing := range t.tags {
		if existing == tag {
			t.tags = append(t.tags[:i], t.tags[i+1:]...)
			return
		}
	}
}

// clearTags 清空所有标签（包内方法，应通过 Editor 调用）
func (t *Testee) clearTags() {
	t.tags = make([]string, 0)
}

// === 关注度管理 ===

// IsKeyFocus 是否重点关注对象
func (t *Testee) IsKeyFocus() bool {
	return t.isKeyFocus
}

// markAsKeyFocus 标记为重点关注（包内方法，应通过 Editor 调用）
func (t *Testee) markAsKeyFocus() {
	t.isKeyFocus = true
}

// unmarkAsKeyFocus 取消重点关注（包内方法，应通过 Editor 调用）
func (t *Testee) unmarkAsKeyFocus() {
	t.isKeyFocus = false
}

// === 数据来源 ===

// Source 获取数据来源
func (t *Testee) Source() string {
	return t.source
}

// === 测评统计查询（只读）===

// AssessmentStats 获取测评统计快照
func (t *Testee) AssessmentStats() *AssessmentStats {
	return t.assessmentStats
}

// HasAssessmentHistory 是否有测评历史
func (t *Testee) HasAssessmentHistory() bool {
	return t.assessmentStats != nil && t.assessmentStats.TotalCount() > 0
}

// updateAssessmentStats 更新测评统计快照（包内方法，应通过 StatsUpdater 调用）
func (t *Testee) updateAssessmentStats(stats *AssessmentStats) {
	t.assessmentStats = stats
}

// === 仓储层需要的方法（用于重建聚合根）===

// SetID 设置ID（仅用于从数据库加载）
func (t *Testee) SetID(id ID) {
	t.id = id
}

// SetSource 设置数据来源（仅用于从数据库加载）
func (t *Testee) SetSource(source string) {
	t.source = source
}

// SetKeyFocus 设置重点关注状态（仅用于从数据库加载）
func (t *Testee) SetKeyFocus(isKeyFocus bool) {
	t.isKeyFocus = isKeyFocus
}

// SetTags 设置标签列表（仅用于从数据库加载）
func (t *Testee) SetTags(tags []string) {
	if tags == nil {
		t.tags = make([]string, 0)
	} else {
		t.tags = tags
	}
}
