// Package eventevidencebinding defines versioned business-field projections.
// It never canonicalizes or replaces original event/body/checksum bytes.
package eventevidencebinding

import (
	"fmt"
	"math"
	"strconv"
	"time"

	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

func text(value string) *string    { return &value }
func integer(value uint64) *string { return text(strconv.FormatUint(value, 10)) }
func signed(value int64) *string   { return text(strconv.FormatInt(value, 10)) }

// Mongo stores business clocks as BSON milliseconds. SDK message clocks remain
// untouched and are independently checked at their original precision.
func clock(value time.Time) *string {
	return text(value.UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano))
}

func AnswerSheet(value eventpayload.AnswerSheetSubmittedData) (string, error) {
	if value.AnswerSheetID == "" || value.OrgID == 0 || value.TesteeID == 0 || value.SubmittedAt.IsZero() {
		return "", fmt.Errorf("answersheet evidence business identity is incomplete")
	}
	fields := []*string{text(value.AnswerSheetID), integer(value.OrgID), integer(value.TesteeID), text(value.QuestionnaireCode), text(value.QuestionnaireVersion), integer(value.FillerID), text(value.FillerType), text(value.TaskID), text(value.RequestID), clock(value.SubmittedAt)}
	if v := value.Admission; v == nil {
		fields = append(fields, nil)
	} else {
		fields = append(fields, text(string(v.Purpose)), text(v.QuestionnaireCode), text(v.QuestionnaireVersion), text(v.ModelKind), text(v.ModelAlgorithm), text(v.ModelCode), text(v.ModelVersion), text(v.ModelTitle))
	}
	if v := value.Attribution; v == nil {
		fields = append(fields, nil)
	} else {
		fields = append(fields, text(v.OriginType), text(v.OriginID), text(v.ClinicianID), text(v.EntryID), text(v.PlanID), text(v.EnrollmentID), text(v.TaskID), clock(v.CapturedAt), integer(uint64(v.Version)), text(v.Mode))
	}
	return evidence.BindingDigest(eventcatalog.AnswerSheetSubmitted, fields...), nil
}

func Generated(value eventoutcome.ReportGeneratedPayload) (string, error) {
	if value.OrgID <= 0 || value.GenerationID == "" || value.RunID == "" || value.ReportID == "" || value.AssessmentID == "" || value.OutcomeID == "" || value.TesteeID == 0 || value.GeneratedAt.IsZero() {
		return "", fmt.Errorf("generated evidence business identity is incomplete")
	}
	fields := []*string{signed(value.OrgID), text(value.GenerationID), text(value.RunID), text(value.ReportID), text(value.AssessmentID), text(value.OutcomeID), integer(value.TesteeID), integer(uint64(value.Attempt)), text(value.ReportType), text(value.TemplateVersion), text(value.BuilderIdentity), text(value.ContentSchemaVersion), text(value.Model.Kind), text(value.Model.Algorithm), text(value.Model.Code), text(value.Model.Version), text(value.Model.Title), clock(value.GeneratedAt)}
	if v := value.PrimaryScore; v == nil {
		fields = append(fields, nil)
	} else {
		score, err := number(v.Value)
		if err != nil {
			return "", err
		}
		fields = append(fields, text(v.Kind), score, text(v.Label))
		if v.Max == nil {
			fields = append(fields, nil)
		} else {
			max, err := number(*v.Max)
			if err != nil {
				return "", err
			}
			fields = append(fields, max)
		}
	}
	if v := value.Level; v == nil {
		fields = append(fields, nil)
	} else {
		fields = append(fields, text(v.Code), text(v.Label), text(v.Severity))
	}
	return evidence.BindingDigest(eventcatalog.InterpretationReportGenerated, fields...), nil
}

func Retry(value eventoutcome.InterpretationRetryRequestedPayload) (string, error) {
	if value.OrgID <= 0 || value.GenerationID == "" || value.RunID == "" || value.AssessmentID == "" || value.OutcomeID == "" || value.TesteeID == 0 || value.ExpectedAttempt < 1 || value.RequestedAt.IsZero() {
		return "", fmt.Errorf("retry evidence business identity is incomplete")
	}
	fields := []*string{signed(value.OrgID), text(value.GenerationID), text(value.RunID), text(value.AssessmentID), text(value.OutcomeID), integer(value.TesteeID), signed(int64(value.ExpectedAttempt)), text(value.AttemptOrigin), text(value.ActionRequestID), text(value.Mode), clock(value.RequestedAt)}
	return evidence.BindingDigest(eventcatalog.InterpretationRetryRequested, fields...), nil
}

func number(value float64) (*string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("non-finite evidence score")
	}
	return text(strconv.FormatFloat(value, 'g', -1, 64)), nil
}
