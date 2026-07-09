package conclusion

// Conclusion 是测量结果到解释、诊断、类型或能力等级的配置抽象。
type Conclusion interface {
	conclusionKind() Kind
}

type Kind string

const (
	KindRisk    Kind = "risk"
	KindType    Kind = "type"
	KindNorm    Kind = "norm"
	KindAbility Kind = "ability"
)

type Outcome struct {
	Code        string
	Title       string
	Summary     string
	Description string
}

type RiskConclusion struct {
	FactorCode string
	Outcomes   []Outcome
}

func (RiskConclusion) conclusionKind() Kind { return KindRisk }

type TypeConclusion struct {
	FactorCodes []string
	Outcomes    []Outcome
}

func (TypeConclusion) conclusionKind() Kind { return KindType }

type NormConclusion struct {
	FactorCode string
	Outcomes   []Outcome
}

func (NormConclusion) conclusionKind() Kind { return KindNorm }

type AbilityConclusion struct {
	FactorCode string
	Outcomes   []Outcome
}

func (AbilityConclusion) conclusionKind() Kind { return KindAbility }
