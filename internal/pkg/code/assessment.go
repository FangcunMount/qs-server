package code

// ==================== Assessment 错误码定义 ====================
// Code must start with 112xxx.

const (
// ErrAssessmentNotFound - 404: Assessment not found.
ErrAssessmentNotFound int = iota + 112001

// ErrAssessmentInvalidStatus - 400: Invalid assessment status for this operation.
ErrAssessmentInvalidStatus

// ErrAssessmentNoScale - 400: Assessment has no medical scale bound.
ErrAssessmentNoScale

// ErrAssessmentInvalidArgument - 400: Invalid argument for assessment.
ErrAssessmentInvalidArgument

// ErrAssessmentDuplicate - 409: Assessment already exists.
ErrAssessmentDuplicate

// ErrAssessmentTesteeNotFound - 404: Testee not found for assessment.
ErrAssessmentTesteeNotFound

// ErrAssessmentQuestionnaireNotFound - 404: Questionnaire not found for assessment.
ErrAssessmentQuestionnaireNotFound

// ErrAssessmentQuestionnaireNotPublished - 400: Questionnaire is not published.
ErrAssessmentQuestionnaireNotPublished

// ErrAssessmentAnswerSheetNotFound - 404: Answer sheet not found for assessment.
ErrAssessmentAnswerSheetNotFound

// ErrAssessmentAnswerSheetMismatch - 400: Answer sheet does not belong to questionnaire.
ErrAssessmentAnswerSheetMismatch

// ErrAssessmentScaleNotFound - 404: Medical scale not found for assessment.
ErrAssessmentScaleNotFound

// ErrAssessmentScaleNotLinked - 400: Medical scale is not linked to questionnaire.
ErrAssessmentScaleNotLinked

// ErrAssessmentReportNotFound - 404: Interpret report not found.
ErrAssessmentReportNotFound

// ErrAssessmentScoreNotFound - 404: Assessment score not found.
ErrAssessmentScoreNotFound

// ErrAssessmentScoreSaveFailed - 500: Failed to save assessment score.
ErrAssessmentScoreSaveFailed

// ErrAssessmentCreateFailed - 500: Failed to create assessment.
ErrAssessmentCreateFailed

// ErrAssessmentSubmitFailed - 500: Failed to submit assessment.
ErrAssessmentSubmitFailed

// ErrAssessmentInterpretFailed - 500: Failed to interpret assessment.
ErrAssessmentInterpretFailed

// ErrCalculationFailed - 500: Failed to calculate score.
ErrCalculationFailed

// ErrForbidden - 403: Access denied.
ErrForbidden

// ErrAssessmentListFailed - 500: Failed to list assessments.
ErrAssessmentListFailed

// ErrAssessmentStatisticsFailed - 500: Failed to get assessment statistics.
ErrAssessmentStatisticsFailed

// ErrAssessmentEvaluateFailed - 500: Failed to evaluate assessment.
ErrAssessmentEvaluateFailed

// ErrAssessmentRetryFailed - 500: Failed to retry assessment.
ErrAssessmentRetryFailed

// ErrReportNotFound - 404: Report not found.
ErrReportNotFound

// ErrReportListFailed - 500: Failed to list reports.
ErrReportListFailed

// ErrScoreNotFound - 404: Score not found.
ErrScoreNotFound

// ErrScoreTrendFailed - 500: Failed to get score trend.
ErrScoreTrendFailed

// ErrScoreHighRiskFailed - 500: Failed to get high risk factors.
ErrScoreHighRiskFailed
)

func init() {
	register(ErrAssessmentNotFound, 404, "Assessment not found")
	register(ErrAssessmentInvalidStatus, 400, "Invalid assessment status for this operation")
	register(ErrAssessmentNoScale, 400, "Assessment has no medical scale bound")
	register(ErrAssessmentInvalidArgument, 400, "Invalid argument for assessment")
	register(ErrAssessmentDuplicate, 409, "Assessment already exists")
	register(ErrAssessmentTesteeNotFound, 404, "Testee not found for assessment")
	register(ErrAssessmentQuestionnaireNotFound, 404, "Questionnaire not found for assessment")
	register(ErrAssessmentQuestionnaireNotPublished, 400, "Questionnaire is not published")
	register(ErrAssessmentAnswerSheetNotFound, 404, "Answer sheet not found for assessment")
	register(ErrAssessmentAnswerSheetMismatch, 400, "Answer sheet does not belong to questionnaire")
	register(ErrAssessmentScaleNotFound, 404, "Medical scale not found for assessment")
	register(ErrAssessmentScaleNotLinked, 400, "Medical scale is not linked to questionnaire")
	register(ErrAssessmentReportNotFound, 404, "Interpret report not found")
	register(ErrAssessmentScoreNotFound, 404, "Assessment score not found")
	register(ErrAssessmentScoreSaveFailed, 500, "Failed to save assessment score")
	register(ErrAssessmentCreateFailed, 500, "Failed to create assessment")
	register(ErrAssessmentSubmitFailed, 500, "Failed to submit assessment")
	register(ErrAssessmentInterpretFailed, 500, "Failed to interpret assessment")
	register(ErrCalculationFailed, 500, "Failed to calculate score")
	register(ErrForbidden, 403, "Access denied")
	register(ErrAssessmentListFailed, 500, "Failed to list assessments")
	register(ErrAssessmentStatisticsFailed, 500, "Failed to get assessment statistics")
	register(ErrAssessmentEvaluateFailed, 500, "Failed to evaluate assessment")
	register(ErrAssessmentRetryFailed, 500, "Failed to retry assessment")
	register(ErrReportNotFound, 404, "Report not found")
	register(ErrReportListFailed, 500, "Failed to list reports")
	register(ErrScoreNotFound, 404, "Score not found")
	register(ErrScoreTrendFailed, 500, "Failed to get score trend")
	register(ErrScoreHighRiskFailed, 500, "Failed to get high risk factors")
}
