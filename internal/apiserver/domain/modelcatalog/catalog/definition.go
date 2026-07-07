package catalog

// DefinitionPayload is the persisted draft definition envelope.
type DefinitionPayload struct {
	Format string
	Data   []byte
}

func (p DefinitionPayload) IsEmpty() bool {
	return p.Format == "" && len(p.Data) == 0
}
