package typology

type BigFiveTraitReport struct {
	Code     string
	Name     string
	RawScore float64
}

type BigFiveReportDetail struct {
	Traits []BigFiveTraitReport
	Source BigFiveSourceReport
}

type BigFiveSourceReport struct {
	QuestionsRepo string
	SourceSite    string
	License       string
	Attribution   string
	NonCommercial bool
}
