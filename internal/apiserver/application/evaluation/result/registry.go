package result

import interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"

func NewScoreProjectorRegistry(projectors ...ScoreProjector) (ScoreProjectorRegistry, error) {
	return interpretationreporting.NewScoreProjectorRegistry(projectors...)
}

func NewReportBuilderRegistry(builders ...ReportBuilder) (ReportBuilderRegistry, error) {
	return interpretationreporting.NewReportBuilderRegistry(builders...)
}
