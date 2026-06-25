package report

type interpretReportDraft struct {
	assessmentID ID
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
	return NewInterpretReport(
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
}
