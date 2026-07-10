package factor

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
