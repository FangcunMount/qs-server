package scale

func cloneScaleResponse(src *ScaleResponse) *ScaleResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Stages = append([]string(nil), src.Stages...)
	dst.ApplicableAges = append([]string(nil), src.ApplicableAges...)
	dst.Reporters = append([]string(nil), src.Reporters...)
	dst.Tags = append([]string(nil), src.Tags...)
	if len(src.Factors) > 0 {
		dst.Factors = make([]FactorResponse, len(src.Factors))
		for i := range src.Factors {
			dst.Factors[i] = cloneFactorResponse(src.Factors[i])
		}
	}
	return &dst
}

func cloneFactorResponse(src FactorResponse) FactorResponse {
	dst := src
	dst.QuestionCodes = append([]string(nil), src.QuestionCodes...)
	if src.ScoringParams != nil {
		dst.ScoringParams = make(map[string]string, len(src.ScoringParams))
		for k, v := range src.ScoringParams {
			dst.ScoringParams[k] = v
		}
	}
	if src.MaxScore != nil {
		v := *src.MaxScore
		dst.MaxScore = &v
	}
	if len(src.InterpretRules) > 0 {
		dst.InterpretRules = append([]InterpretRuleResponse(nil), src.InterpretRules...)
	}
	return dst
}

func cloneScaleSummaryResponse(src ScaleSummaryResponse) ScaleSummaryResponse {
	dst := src
	dst.Stages = append([]string(nil), src.Stages...)
	dst.ApplicableAges = append([]string(nil), src.ApplicableAges...)
	dst.Reporters = append([]string(nil), src.Reporters...)
	dst.Tags = append([]string(nil), src.Tags...)
	return dst
}

func cloneListScalesResponse(src *ListScalesResponse) *ListScalesResponse {
	if src == nil {
		return nil
	}
	dst := *src
	if len(src.Scales) > 0 {
		dst.Scales = make([]ScaleSummaryResponse, len(src.Scales))
		for i := range src.Scales {
			dst.Scales[i] = cloneScaleSummaryResponse(src.Scales[i])
		}
	}
	return &dst
}

func cloneHotScaleSummaryResponse(src HotScaleSummaryResponse) HotScaleSummaryResponse {
	dst := src
	dst.ScaleSummaryResponse = cloneScaleSummaryResponse(src.ScaleSummaryResponse)
	return dst
}

func cloneListHotScalesResponse(src *ListHotScalesResponse) *ListHotScalesResponse {
	if src == nil {
		return nil
	}
	dst := *src
	if len(src.Scales) > 0 {
		dst.Scales = make([]HotScaleSummaryResponse, len(src.Scales))
		for i := range src.Scales {
			dst.Scales[i] = cloneHotScaleSummaryResponse(src.Scales[i])
		}
	}
	return &dst
}

func cloneScaleCategoriesResponse(src *ScaleCategoriesResponse) *ScaleCategoriesResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Categories = append([]CategoryResponse(nil), src.Categories...)
	dst.Stages = append([]StageResponse(nil), src.Stages...)
	dst.ApplicableAges = append([]ApplicableAgeResponse(nil), src.ApplicableAges...)
	dst.Reporters = append([]ReporterResponse(nil), src.Reporters...)
	dst.Tags = append([]TagResponse(nil), src.Tags...)
	return &dst
}
