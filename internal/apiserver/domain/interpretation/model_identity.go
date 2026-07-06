package interpretation

// ModelIdentity is the canonical published-model reference on a report.
type ModelIdentity struct {
	Kind      string
	SubKind   string
	Algorithm string
	Code      string
	Version   string
	Title     string
}

func (m ModelIdentity) IsEmpty() bool {
	return m.Kind == "" && m.SubKind == "" && m.Algorithm == "" && m.Code == "" && m.Version == "" && m.Title == ""
}
