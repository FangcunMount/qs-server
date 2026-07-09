package factor

// SlimFactorsFromLegacy materializes Factor identity nodes from transitional flat factors.
func SlimFactorsFromLegacy(factors []LegacyFactor) []Factor {
	if factors == nil {
		return nil
	}
	out := make([]Factor, 0, len(factors))
	for _, item := range factors {
		out = append(out, Factor{
			Code:  item.Code,
			Title: item.Title,
			Role:  item.ResolvedRole(),
		})
	}
	return out
}

// FactorGraphFromLegacy extracts hierarchy and ordering metadata from transitional flat factors.
func FactorGraphFromLegacy(factors []LegacyFactor) FactorGraph {
	if len(factors) == 0 {
		return FactorGraph{}
	}
	graph := FactorGraph{
		Roots:      make([]string, 0, len(factors)),
		Edges:      make([]FactorEdge, 0, len(factors)),
		SortOrders: make(map[string]int),
	}
	seenEdges := make(map[FactorEdge]struct{})
	hasParent := make(map[string]bool, len(factors))
	for _, item := range factors {
		if item.SortOrder != 0 {
			graph.SortOrders[item.Code] = item.SortOrder
		}
		if item.ParentCode != "" {
			edge := FactorEdge{ParentCode: item.ParentCode, ChildCode: item.Code}
			if _, ok := seenEdges[edge]; !ok {
				graph.Edges = append(graph.Edges, edge)
				seenEdges[edge] = struct{}{}
			}
			hasParent[item.Code] = true
		}
		if item.ChildrenPolicy == nil {
			continue
		}
		for _, childCode := range item.ChildrenPolicy.Children {
			edge := FactorEdge{ParentCode: item.Code, ChildCode: childCode}
			if _, ok := seenEdges[edge]; ok {
				continue
			}
			graph.Edges = append(graph.Edges, edge)
			seenEdges[edge] = struct{}{}
			hasParent[childCode] = true
		}
	}
	for _, item := range factors {
		if !hasParent[item.Code] {
			graph.Roots = append(graph.Roots, item.Code)
		}
	}
	if len(graph.SortOrders) == 0 {
		graph.SortOrders = nil
	}
	return graph
}

// ScoringFromLegacy extracts scoring rules from transitional flat factors.
func ScoringFromLegacy(factors []LegacyFactor) []Scoring {
	if len(factors) == 0 {
		return nil
	}
	out := make([]Scoring, 0, len(factors))
	for _, item := range factors {
		switch {
		case item.ChildrenPolicy != nil && len(item.ChildrenPolicy.Children) > 0:
			sources := make([]ScoringSource, 0, len(item.ChildrenPolicy.Children))
			for _, childCode := range item.ChildrenPolicy.Children {
				sources = append(sources, ScoringSource{Kind: ScoringSourceFactor, Code: childCode})
			}
			out = append(out, Scoring{
				FactorCode: item.Code,
				Sources:    sources,
				Strategy:   ScoringStrategy(item.ChildrenPolicy.Strategy),
				Params:     cloneScoringParams(item.ScoringParams),
				MaxScore:   cloneFloat64(item.MaxScore),
				Weights:    cloneWeights(item.ChildrenPolicy.Weights),
			})
		case len(item.QuestionCodes) > 0 || item.ScoringStrategy != "" || item.ScoringParams != nil || item.MaxScore != nil:
			sources := make([]ScoringSource, 0, len(item.QuestionCodes))
			for _, questionCode := range item.QuestionCodes {
				sources = append(sources, ScoringSource{Kind: ScoringSourceQuestion, Code: questionCode})
			}
			out = append(out, Scoring{
				FactorCode: item.Code,
				Sources:    sources,
				Strategy:   ScoringStrategy(item.ScoringStrategy),
				Params:     cloneScoringParams(item.ScoringParams),
				MaxScore:   cloneFloat64(item.MaxScore),
			})
		}
	}
	return out
}

// NormRefsFromLegacy extracts norm references from transitional flat factors.
func NormRefsFromLegacy(factors []LegacyFactor) []NormRef {
	if len(factors) == 0 {
		return nil
	}
	out := make([]NormRef, 0)
	for _, item := range factors {
		if item.Norm == nil {
			continue
		}
		ref := *item.Norm
		if ref.FactorCode == "" {
			ref.FactorCode = item.Code
		}
		out = append(out, ref)
	}
	return out
}

// ParentCode returns the parent code for a child factor, if present.
func (g FactorGraph) ParentCode(childCode string) string {
	for _, edge := range g.Edges {
		if edge.ChildCode == childCode {
			return edge.ParentCode
		}
	}
	return ""
}

// Children returns child factor codes for a parent factor.
func (g FactorGraph) Children(parentCode string) []string {
	children := make([]string, 0)
	for _, edge := range g.Edges {
		if edge.ParentCode == parentCode {
			children = append(children, edge.ChildCode)
		}
	}
	if len(children) == 0 {
		return nil
	}
	return children
}

// Levels derives one-based levels from graph roots and edges.
func (g FactorGraph) Levels() map[string]int {
	levels := make(map[string]int)
	childrenByParent := make(map[string][]string)
	for _, edge := range g.Edges {
		childrenByParent[edge.ParentCode] = append(childrenByParent[edge.ParentCode], edge.ChildCode)
	}
	var walk func(code string, level int)
	walk = func(code string, level int) {
		if current, ok := levels[code]; ok && current <= level {
			return
		}
		levels[code] = level
		for _, childCode := range childrenByParent[code] {
			walk(childCode, level+1)
		}
	}
	for _, root := range g.Roots {
		walk(root, 1)
	}
	return levels
}
