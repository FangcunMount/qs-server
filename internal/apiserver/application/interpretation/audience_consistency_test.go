package interpretation_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func TestRestrictedClinicianAdministrationMatchesClinicianModelExtra(t *testing.T) {
	t.Parallel()

	row := interpretationreadmodel.ReportRow{
		AssessmentID: 42,
		ModelExtra:   &interpretationreadmodel.ReportModelExtraRow{TypeCode: "secret", TypeName: "hidden"},
	}
	reader := &sharedReportReader{row: row}

	adminSvc := administration.NewService(reader, restrictedAdminAccess{})
	clinicianSvc := clinician.NewService(reader, clinicianAccess{})

	adminReport, err := adminSvc.GetReport(context.Background(), administration.Actor{OrgID: 1, OperatorUserID: 2}, administration.GetQuery{AssessmentID: 42})
	if err != nil {
		t.Fatal(err)
	}
	clinicianReport, err := clinicianSvc.GetParticipantReport(context.Background(), clinician.Actor{OrgID: 1, OperatorUserID: 2}, clinician.GetQuery{TesteeID: 7, AssessmentID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if adminReport.ModelExtra != nil {
		t.Fatal("administration exposed ModelExtra for restricted clinician")
	}
	if clinicianReport.ModelExtra != nil {
		t.Fatal("clinician exposed ModelExtra")
	}
}

func TestAdminAdministrationStillSeesModelExtra(t *testing.T) {
	t.Parallel()

	row := interpretationreadmodel.ReportRow{
		AssessmentID: 42,
		ModelExtra:   &interpretationreadmodel.ReportModelExtraRow{TypeCode: "secret", TypeName: "visible"},
	}
	reader := &sharedReportReader{row: row}
	adminSvc := administration.NewService(reader, adminActorAccess{})

	report, err := adminSvc.GetReport(context.Background(), administration.Actor{OrgID: 1, OperatorUserID: 2}, administration.GetQuery{AssessmentID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if report.ModelExtra == nil || report.ModelExtra.TypeCode != "secret" {
		t.Fatalf("admin lost ModelExtra: %#v", report.ModelExtra)
	}
}

type sharedReportReader struct {
	row interpretationreadmodel.ReportRow
}

func (r *sharedReportReader) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	row := r.row
	return &row, nil
}

func (r *sharedReportReader) ListReports(context.Context, interpretationreadmodel.ReportFilter, interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	return []interpretationreadmodel.ReportRow{r.row}, 1, nil
}

type restrictedAdminAccess struct{}

func (restrictedAdminAccess) AuthorizeAssessment(context.Context, administration.Actor, uint64) (administration.ReportAccessDecision, error) {
	return administration.ReportAccessDecision{
		Audience:       policy.AudienceClinician,
		IsAdmin:        false,
		Restricted:     true,
		DecisionSource: "test.restricted",
	}, nil
}

func (restrictedAdminAccess) ScopeReports(context.Context, administration.Actor, uint64) (administration.ListScope, error) {
	return administration.ListScope{
		TesteeID:       7,
		Restricted:     false,
		Audience:       policy.AudienceClinician,
		IsAdmin:        false,
		DecisionSource: "test.restricted",
	}, nil
}

type adminActorAccess struct{}

func (adminActorAccess) AuthorizeAssessment(context.Context, administration.Actor, uint64) (administration.ReportAccessDecision, error) {
	return administration.ReportAccessDecision{
		Audience:       policy.AudienceAdmin,
		IsAdmin:        true,
		Restricted:     false,
		DecisionSource: "test.admin",
	}, nil
}

func (adminActorAccess) ScopeReports(context.Context, administration.Actor, uint64) (administration.ListScope, error) {
	return administration.ListScope{
		OrgID:          1,
		Audience:       policy.AudienceAdmin,
		IsAdmin:        true,
		DecisionSource: "test.admin",
	}, nil
}

type clinicianAccess struct{}

func (clinicianAccess) AuthorizeParticipant(context.Context, clinician.Actor, uint64) error {
	return nil
}

func (clinicianAccess) AuthorizeParticipantAssessment(context.Context, clinician.Actor, uint64, uint64) error {
	return nil
}
