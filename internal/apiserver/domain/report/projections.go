package report

// FinalizeInterpretReport synchronizes v2 outcome summary fields with legacy report fields.
func FinalizeInterpretReport(r *InterpretReport) {
	if r == nil {
		return
	}
	if r.primaryScore == nil && (r.totalScore != 0 || r.riskLevel != "") {
		r.primaryScore = NewRawTotalScore(r.totalScore, nil)
	}
	if r.level == nil && r.riskLevel != "" {
		r.level = LevelFromRisk(r.riskLevel)
	}
	if r.model.IsEmpty() {
		if r.modelCode != "" || r.modelName != "" {
			r.model = ModelIdentity{Code: r.modelCode, Title: r.modelName}
		}
	}
	if r.primaryScore != nil {
		r.totalScore = r.primaryScore.Value
	}
	if r.level != nil && r.level.Code != "" && IsRiskLevelCode(r.level.Code) {
		r.riskLevel = RiskLevel(r.level.Code)
	}
	if !r.model.IsEmpty() {
		if r.modelName == "" {
			r.modelName = r.model.Title
		}
		if r.modelCode == "" {
			r.modelCode = r.model.Code
		}
	}
}

// AttachOutcomeSummary binds v2 model identity and outcome summary onto a report.
func AttachOutcomeSummary(
	r *InterpretReport,
	model ModelIdentity,
	primary *ScoreValue,
	level *ResultLevel,
) *InterpretReport {
	if r == nil {
		return nil
	}
	if !model.IsEmpty() {
		r.model = model
	}
	if primary != nil {
		r.primaryScore = primary
	}
	if level != nil {
		r.level = level
	}
	FinalizeInterpretReport(r)
	return r
}
