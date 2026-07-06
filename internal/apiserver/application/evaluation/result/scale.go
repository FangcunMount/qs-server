package result

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type ScaleScoreProjector = interpretationreporting.ScaleScoreProjector

type ScaleReportBuilder = interpretationreporting.ScaleReportBuilder

func NewScaleScoreProjector(scoreRepo assessment.ScoreRepository) ScaleScoreProjector {
	return interpretationreporting.NewScaleScoreProjector(scoreRepo)
}

func NewScaleReportBuilder(composer domainReport.ReportBuilder) ScaleReportBuilder {
	return interpretationreporting.NewScaleReportBuilder(composer)
}
