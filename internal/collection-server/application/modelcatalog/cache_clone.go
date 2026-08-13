package modelcatalog

func cloneModelResponse(src *ModelResponse) *ModelResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Stages = cloneSlice(src.Stages)
	dst.ApplicableAges = cloneSlice(src.ApplicableAges)
	dst.Reporters = cloneSlice(src.Reporters)
	dst.Tags = cloneSlice(src.Tags)
	dst.Definition = cloneSlice(src.Definition)
	return &dst
}

func cloneListResponse(src *ListResponse) *ListResponse {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Models != nil {
		dst.Models = make([]ModelResponse, len(src.Models))
		for index := range src.Models {
			dst.Models[index] = *cloneModelResponse(&src.Models[index])
		}
	}
	return &dst
}

func cloneOptionsResponse(src *OptionsResponse) *OptionsResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Kinds = cloneSlice(src.Kinds)
	dst.Algorithms = cloneSlice(src.Algorithms)
	dst.Categories = cloneSlice(src.Categories)
	dst.Stages = cloneSlice(src.Stages)
	dst.ApplicableAges = cloneSlice(src.ApplicableAges)
	dst.Reporters = cloneSlice(src.Reporters)
	return &dst
}

func cloneSlice[T any](src []T) []T {
	if src == nil {
		return nil
	}
	dst := make([]T, len(src))
	copy(dst, src)
	return dst
}
