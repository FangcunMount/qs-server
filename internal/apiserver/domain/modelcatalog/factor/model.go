package factor

// Factor 是 ModelCatalog 的通用测量节点。
//
// 当前与 FactorSnapshot 同形，后续会逐步从 snapshot 命名迁到配置语义。
type Factor = FactorSnapshot

// Scoring 描述一个 Factor 的题目分聚合策略。
type Scoring struct {
	FactorCode string
	Strategy   ScoringStrategy
	Params     *ScoringParams
}

// FactorGraph 描述 Factor 之间的层级和展示顺序。
type FactorGraph struct {
	Roots []string
	Edges map[string][]string
}
