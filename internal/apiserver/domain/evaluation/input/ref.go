package input

// SnapshotRef identifies a published model snapshot used as evaluation input.
type SnapshotRef struct {
	ModelCode    string
	ModelVersion string
}

// IsZero reports whether the snapshot reference is unset.
func (r SnapshotRef) IsZero() bool {
	return r.ModelCode == "" && r.ModelVersion == ""
}
