package classification

import (
	"fmt"
)

// FactorGraph 是directed acyclic 层级 of personality 因子。
type FactorGraph struct {
	Factors   map[FactorID]PersonalityFactor
	LeafSpecs map[FactorID]LeafScoringSpec
	Roots     []FactorID
}

// Validate 检查graph invariants: known 子节点, 叶子 specs, 和 acyclicity。
func (g FactorGraph) Validate() error {
	if len(g.Factors) == 0 {
		return fmt.Errorf("factor graph is empty")
	}
	for id, factor := range g.Factors {
		switch factor.Kind {
		case FactorKindLeaf:
			if len(factor.Children) > 0 {
				return fmt.Errorf("leaf factor %s must not have children", id)
			}
			if _, ok := g.LeafSpecs[id]; !ok {
				return fmt.Errorf("leaf factor %s is missing scoring spec", id)
			}
		case FactorKindComposite:
			if len(factor.Children) == 0 {
				return fmt.Errorf("composite factor %s requires children", id)
			}
			if _, ok := g.LeafSpecs[id]; ok {
				return fmt.Errorf("composite factor %s must not have leaf spec", id)
			}
		default:
			return fmt.Errorf("factor %s has unsupported kind %s", id, factor.Kind)
		}
		for _, childID := range factor.Children {
			if _, ok := g.Factors[childID]; !ok {
				return fmt.Errorf("factor %s references unknown child %s", id, childID)
			}
		}
		if factor.Aggregation == AggregationWeightedAvg {
			if len(factor.Weights) == 0 {
				return fmt.Errorf("weighted factor %s requires weights", id)
			}
			children := make(map[FactorID]struct{}, len(factor.Children))
			for _, childID := range factor.Children {
				children[childID] = struct{}{}
			}
			for childID, weight := range factor.Weights {
				if _, ok := children[childID]; !ok {
					return fmt.Errorf("factor %s weight %s is not a child", id, childID)
				}
				if weight <= 0 {
					return fmt.Errorf("factor %s child %s weight must be > 0", id, childID)
				}
			}
			for _, childID := range factor.Children {
				if _, ok := factor.Weights[childID]; !ok {
					return fmt.Errorf("factor %s child %s is missing weight", id, childID)
				}
			}
		}
	}
	for leafID := range g.LeafSpecs {
		if _, ok := g.Factors[leafID]; !ok {
			return fmt.Errorf("leaf spec %s has no factor definition", leafID)
		}
	}
	if err := detectCycle(g); err != nil {
		return err
	}
	if len(g.Roots) == 0 {
		return fmt.Errorf("factor graph roots are required")
	}
	for _, rootID := range g.Roots {
		if _, ok := g.Factors[rootID]; !ok {
			return fmt.Errorf("unknown root factor %s", rootID)
		}
	}
	return nil
}

func detectCycle(g FactorGraph) error {
	visiting := make(map[FactorID]bool, len(g.Factors))
	visited := make(map[FactorID]bool, len(g.Factors))
	var visit func(FactorID) error
	visit = func(id FactorID) error {
		if visiting[id] {
			return fmt.Errorf("factor graph cycle detected at %s", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		factor := g.Factors[id]
		for _, childID := range factor.Children {
			if err := visit(childID); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range g.Factors {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// TopologicalOrder 返回子节点-在之前-父节点s 评估 order。
func (g FactorGraph) TopologicalOrder() ([]FactorID, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	visited := make(map[FactorID]bool, len(g.Factors))
	order := make([]FactorID, 0, len(g.Factors))
	var visit func(FactorID) error
	visit = func(id FactorID) error {
		if visited[id] {
			return nil
		}
		factor := g.Factors[id]
		for _, childID := range factor.Children {
			if err := visit(childID); err != nil {
				return err
			}
		}
		visited[id] = true
		order = append(order, id)
		return nil
	}
	for id := range g.Factors {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}
