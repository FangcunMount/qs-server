package answersheetattribution

import (
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetattribution"
	"testing"
	"time"
)

func TestPlanTaskAdmissionRejectsScopeStatusAndTimeBypasses(t *testing.T) {
	now := time.Now()
	opened, expires := now.Add(-time.Hour), now.Add(time.Hour)
	admission, err := sheet.NewAssessmentAdmission("question", "v1", "scale", "", "", "model", "v1", "测试")
	if err != nil {
		t.Fatal(err)
	}
	request := port.ResolveRequest{OrgID: 7, TesteeID: 31, QuestionnaireCode: "question", QuestionnaireVersion: "v1", Admission: admission}
	valid := planTaskRow{ID: 42, OrgID: 7, TesteeID: 31, ScaleCode: "model", TaskStatus: "opened", EnrollmentStatus: "active", OpenAt: &opened, ExpireAt: &expires}
	if err := validatePlanTask(valid, request, now); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*planTaskRow){
		"foreign org":          func(r *planTaskRow) { r.OrgID = 8 },
		"forged profile":       func(r *planTaskRow) { r.TesteeID = 32 },
		"pending":              func(r *planTaskRow) { r.TaskStatus = "pending" },
		"completed":            func(r *planTaskRow) { r.TaskStatus = "completed" },
		"canceled":             func(r *planTaskRow) { r.TaskStatus = "canceled" },
		"expired":              func(r *planTaskRow) { r.TaskStatus = "expired" },
		"inactive enrollment":  func(r *planTaskRow) { r.EnrollmentStatus = "terminated" },
		"future opening":       func(r *planTaskRow) { r.OpenAt = &expires },
		"hard expiry boundary": func(r *planTaskRow) { r.ExpireAt = &now },
		"missing expiry":       func(r *planTaskRow) { r.ExpireAt = nil },
		"already linked":       func(r *planTaskRow) { id := uint64(99); r.AssessmentID = &id },
		"different model":      func(r *planTaskRow) { r.ScaleCode = "different" },
	} {
		t.Run(name, func(t *testing.T) {
			row := valid
			change(&row)
			if err := validatePlanTask(row, request, now); err == nil {
				t.Fatal("invalid task admitted")
			}
		})
	}
}
