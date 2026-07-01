package personalitymodel

func clonePersonalityModelResponse(src *PersonalityModelResponse) *PersonalityModelResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.DimensionOrder = append([]string(nil), src.DimensionOrder...)
	if len(src.Dimensions) > 0 {
		dst.Dimensions = append([]PersonalityDimensionResponse(nil), src.Dimensions...)
	}
	if len(src.Outcomes) > 0 {
		dst.Outcomes = append([]PersonalityOutcomeResponse(nil), src.Outcomes...)
	}
	return &dst
}

func clonePersonalityModelSummaryResponse(src PersonalityModelSummaryResponse) PersonalityModelSummaryResponse {
	return src
}

func cloneListPersonalityModelsResponse(src *ListPersonalityModelsResponse) *ListPersonalityModelsResponse {
	if src == nil {
		return nil
	}
	dst := *src
	if len(src.Models) > 0 {
		dst.Models = make([]PersonalityModelSummaryResponse, len(src.Models))
		for i := range src.Models {
			dst.Models[i] = clonePersonalityModelSummaryResponse(src.Models[i])
		}
	}
	return &dst
}

func clonePersonalityModelCategoriesResponse(src *PersonalityModelCategoriesResponse) *PersonalityModelCategoriesResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Categories = append([]CategoryResponse(nil), src.Categories...)
	return &dst
}
