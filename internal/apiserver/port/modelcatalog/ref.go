package modelcatalog

// RefFromPublished projects an immutable published model to a runtime model
// reference. It intentionally does not inspect the wire payload.
func RefFromPublished(model *PublishedModel) Ref {
	if model == nil {
		return Ref{}
	}
	return Ref{
		Kind:      model.Kind,
		SubKind:   model.SubKind,
		Algorithm: model.Algorithm,
		Code:      model.Code,
		Version:   model.Version,
		Title:     model.Title,
	}
}
