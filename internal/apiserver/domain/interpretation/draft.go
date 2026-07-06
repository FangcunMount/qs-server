package report

type interpretReportDraft struct {
	assessmentID ID
	model        ModelIdentity
	primaryScore *ScoreValue
	level        *ResultLevel
	modelName    string
	modelCode    string
	totalScore   float64
	riskLevel    RiskLevel
	conclusion   string
	dimensions   []DimensionInterpret
	suggestions  []Suggestion
	modelExtra   *ModelExtra
}

func (d interpretReportDraft) build() *InterpretReport {
	r := NewInterpretReport(
		d.assessmentID,
		d.modelName,
		d.modelCode,
		d.totalScore,
		d.riskLevel,
		d.conclusion,
		d.dimensions,
		d.suggestions,
		d.modelExtra,
	)
	return AttachOutcomeSummary(r, d.model, d.primaryScore, d.level)
}
