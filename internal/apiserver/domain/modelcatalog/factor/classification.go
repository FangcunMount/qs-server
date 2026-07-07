package factor

// ClassificationSpec 携带pole-composition 元数据 用于 类型学 models。
// Personality 类型学 still uses its own 运行时 graph; 这个type 是 共享 目录 seam。
type ClassificationSpec struct {
	PositivePole string
	NegativePole string
	DecisionRule string
	TieBreakRule string
}
