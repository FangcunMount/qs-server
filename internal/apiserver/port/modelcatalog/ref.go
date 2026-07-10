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

// RefMatchesPublished reports whether a runtime reference selects this exact
// immutable published model.
func RefMatchesPublished(ref Ref, model *PublishedModel) bool {
	if model == nil || ref.Code == "" || ref.Version == "" {
		return false
	}
	got := RefFromPublished(model)
	return ref.Kind == got.Kind &&
		ref.SubKind == got.SubKind &&
		ref.Algorithm == got.Algorithm &&
		ref.Code == got.Code &&
		ref.Version == got.Version
}
