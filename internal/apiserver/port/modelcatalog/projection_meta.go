package modelcatalog

const (
	// SourceDefinitionContentHash stores CanonicalContentHash of DefinitionV2 authoring layers.
	SourceDefinitionContentHash = "definition_content_hash"
	// SourcePayloadProjectionHash stores SHA256 of published compatibility payload bytes.
	SourcePayloadProjectionHash = "payload_projection_hash"
	// SourceProjectionHashSchema identifies the hash contract version.
	SourceProjectionHashSchema = "projection_hash_schema"
)

// AttachProjectionHashes writes publish-time projection fingerprints into snapshot Source.
func AttachProjectionHashes(snapshot *AssessmentSnapshot, definitionHash, payloadHash string) {
	if snapshot == nil {
		return
	}
	if snapshot.Source == nil {
		snapshot.Source = map[string]any{}
	}
	if definitionHash != "" {
		snapshot.Source[SourceDefinitionContentHash] = definitionHash
	}
	if payloadHash != "" {
		snapshot.Source[SourcePayloadProjectionHash] = payloadHash
	}
	snapshot.Source[SourceProjectionHashSchema] = "definition-projection/v1"
}

// ProjectionHashesFromSource reads publish-time projection fingerprints from snapshot Source.
func ProjectionHashesFromSource(source map[string]any) (definitionHash, payloadHash string) {
	if source == nil {
		return "", ""
	}
	if value, ok := source[SourceDefinitionContentHash].(string); ok {
		definitionHash = value
	}
	if value, ok := source[SourcePayloadProjectionHash].(string); ok {
		payloadHash = value
	}
	return definitionHash, payloadHash
}
