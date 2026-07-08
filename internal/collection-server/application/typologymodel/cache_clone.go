package typologymodel

func cloneTypologyModelResponse(src *TypologyModelResponse) *TypologyModelResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.DimensionOrder = append([]string(nil), src.DimensionOrder...)
	if len(src.Dimensions) > 0 {
		dst.Dimensions = append([]TypologyDimensionResponse(nil), src.Dimensions...)
	}
	if len(src.Outcomes) > 0 {
		dst.Outcomes = append([]TypologyOutcomeResponse(nil), src.Outcomes...)
	}
	return &dst
}

func cloneTypologyModelSummaryResponse(src TypologyModelSummaryResponse) TypologyModelSummaryResponse {
	return src
}

func cloneListTypologyModelsResponse(src *ListTypologyModelsResponse) *ListTypologyModelsResponse {
	if src == nil {
		return nil
	}
	dst := *src
	if len(src.Models) > 0 {
		dst.Models = make([]TypologyModelSummaryResponse, len(src.Models))
		for i := range src.Models {
			dst.Models[i] = cloneTypologyModelSummaryResponse(src.Models[i])
		}
	}
	return &dst
}

func cloneTypologyModelCategoriesResponse(src *TypologyModelCategoriesResponse) *TypologyModelCategoriesResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Categories = append([]TypologyCategoryResponse(nil), src.Categories...)
	return &dst
}
