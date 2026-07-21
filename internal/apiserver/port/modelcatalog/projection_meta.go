package modelcatalog

const (
	SourceDefinitionContentHash = "definition_content_hash"
	SourceDefinitionHashSchema  = "definition_hash_schema"
)

// AttachDefinitionHash records the canonical DefinitionV2 fingerprint.
func AttachDefinitionHash(snapshot *AssessmentSnapshot, definitionHash string) {
	if snapshot == nil {
		return
	}
	if snapshot.Source == nil {
		snapshot.Source = map[string]any{}
	}
	if definitionHash != "" {
		snapshot.Source[SourceDefinitionContentHash] = definitionHash
	}
	snapshot.Source[SourceDefinitionHashSchema] = "definition-v2/v1"
}

func DefinitionHashFromSource(source map[string]any) string {
	if value, ok := source[SourceDefinitionContentHash].(string); ok {
		return value
	}
	return ""
}
