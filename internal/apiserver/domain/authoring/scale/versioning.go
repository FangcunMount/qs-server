package scale

// Versioning 管理量表解释模型版本。
type Versioning struct{}

// InitializeVersion 初始化新建量表版本。
func (Versioning) InitializeVersion(m *MedicalScale) error {
	if m == nil || m.version != "" {
		return nil
	}
	return m.updateScaleVersion(NewScaleVersion(DefaultScaleVersion))
}

// ForkDraftFromPublished 将已发布 head 派生为草稿工作版本。
// 对外可答状态由 published snapshot 承载，因此该操作不触发下架事件。
func (Versioning) ForkDraftFromPublished(m *MedicalScale) error {
	if m == nil || !m.IsPublished() {
		return nil
	}
	current := NewScaleVersion(m.GetScaleVersion())
	if err := m.updateScaleVersion(current.IncrementPatch()); err != nil {
		return err
	}
	return m.updateStatus(StatusDraft)
}
