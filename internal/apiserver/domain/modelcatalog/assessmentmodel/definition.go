package assessmentmodel

// DefinitionPayload 是评估模型的定义
type DefinitionPayload struct {
	Format string
	Data   []byte
}

func (p DefinitionPayload) IsEmpty() bool {
	return p.Format == "" && len(p.Data) == 0
}
