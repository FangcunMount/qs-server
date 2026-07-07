package input

// SnapshotRef 标识已发布模型快照 作为 评估输入。
type SnapshotRef struct {
	ModelCode    string
	ModelVersion string
}

// IsZero 报告是否 快照引用 是 unset。
func (r SnapshotRef) IsZero() bool {
	return r.ModelCode == "" && r.ModelVersion == ""
}
