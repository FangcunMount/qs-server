package factor

// FactorsFromDefinitionDimensions materializes Factor identity nodes from
// shared legacy payload dimensions.
func FactorsFromDefinitionDimensions(dimensions []DimensionRule) []Factor {
	if dimensions == nil {
		return nil
	}
	out := make([]Factor, 0, len(dimensions))
	for _, item := range dimensions {
		role := FactorRole(item.Role)
		if role != "" && !role.IsValid() {
			role = ""
		}
		out = append(out, Factor{
			Code:  item.Code,
			Title: item.Title,
			Role:  resolveRole(role, item.IsTotalScore),
		})
	}
	return out
}

// FactorGraphFromDefinitionDimensions extracts hierarchy and ordering metadata
// from shared legacy payload dimensions.
func FactorGraphFromDefinitionDimensions(dimensions []DimensionRule) FactorGraph {
	if len(dimensions) == 0 {
		return FactorGraph{}
	}
	graph := FactorGraph{
		Roots:      make([]string, 0, len(dimensions)),
		Edges:      make([]FactorEdge, 0, len(dimensions)),
		SortOrders: make(map[string]int),
	}
	seenEdges := make(map[FactorEdge]struct{})
	hasParent := make(map[string]bool, len(dimensions))
	for _, item := range dimensions {
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
	for _, item := range dimensions {
		if !hasParent[item.Code] {
			graph.Roots = append(graph.Roots, item.Code)
		}
	}
	if len(graph.SortOrders) == 0 {
		graph.SortOrders = nil
	}
	return graph
}

// ScoringFromDefinitionDimensions extracts scoring rules from shared legacy
// payload dimensions.
func ScoringFromDefinitionDimensions(dimensions []DimensionRule) []Scoring {
	if len(dimensions) == 0 {
		return nil
	}
	out := make([]Scoring, 0, len(dimensions))
	for _, item := range dimensions {
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
				Params:     scoringParamsFromPayload(item.ScoringParams),
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
				Params:     scoringParamsFromPayload(item.ScoringParams),
				MaxScore:   cloneFloat64(item.MaxScore),
			})
		}
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
