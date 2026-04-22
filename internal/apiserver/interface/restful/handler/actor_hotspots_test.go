package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/gin-gonic/gin"
)

func TestBuildScaleAnalysisResponseFiltersAndSorts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/testees/1/scale-analysis", nil)

	scaleID := uint64(9)
	scaleCode := "SAS"
	scaleName := "Sleep"
	earlier := time.Date(2026, 4, 10, 9, 0, 0, 0, time.Local)
	later := time.Date(2026, 4, 12, 9, 0, 0, 0, time.Local)
	pendingTime := time.Date(2026, 4, 9, 9, 0, 0, 0, time.Local)
	riskLevel := "low"
	totalScore := 10.0

	handler := &ActorHandler{}
	result := handler.buildScaleAnalysisResponse(c, []*assessmentApp.AssessmentResult{
		{
			ID:               2,
			Status:           "interpreted",
			MedicalScaleID:   &scaleID,
			MedicalScaleCode: &scaleCode,
			MedicalScaleName: &scaleName,
			InterpretedAt:    &later,
			RiskLevel:        &riskLevel,
			TotalScore:       &totalScore,
		},
		{
			ID:               1,
			Status:           "interpreted",
			MedicalScaleID:   &scaleID,
			MedicalScaleCode: &scaleCode,
			MedicalScaleName: &scaleName,
			SubmittedAt:      &earlier,
			RiskLevel:        &riskLevel,
			TotalScore:       &totalScore,
		},
		{
			ID:               3,
			Status:           "pending",
			MedicalScaleID:   &scaleID,
			MedicalScaleCode: &scaleCode,
			MedicalScaleName: &scaleName,
			SubmittedAt:      &pendingTime,
		},
	})

	if len(result.Scales) != 1 {
		t.Fatalf("scales len = %d, want 1", len(result.Scales))
	}
	if got := result.Scales[0].ScaleID; got != "9" {
		t.Fatalf("scale_id = %q, want 9", got)
	}
	if len(result.Scales[0].Tests) != 2 {
		t.Fatalf("tests len = %d, want 2", len(result.Scales[0].Tests))
	}
	if got := result.Scales[0].Tests[0].AssessmentID; got != "1" {
		t.Fatalf("first assessment_id = %q, want 1", got)
	}
	if got := result.Scales[0].Tests[1].AssessmentID; got != "2" {
		t.Fatalf("second assessment_id = %q, want 2", got)
	}
}

func TestMergeAccessibleTesteeIDs(t *testing.T) {
	tests := []struct {
		name             string
		existing         []uint64
		restrictExisting bool
		allowed          []uint64
		want             []uint64
		wantRestricted   bool
	}{
		{
			name:             "adopts allowed set when no existing restriction",
			existing:         nil,
			restrictExisting: false,
			allowed:          []uint64{2, 3},
			want:             []uint64{2, 3},
			wantRestricted:   true,
		},
		{
			name:             "intersects when clinician scope already restricts",
			existing:         []uint64{1, 2, 3},
			restrictExisting: true,
			allowed:          []uint64{2, 4},
			want:             []uint64{2},
			wantRestricted:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, restricted := mergeAccessibleTesteeIDs(tt.existing, tt.restrictExisting, tt.allowed)
			if restricted != tt.wantRestricted {
				t.Fatalf("restricted = %v, want %v", restricted, tt.wantRestricted)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len(got) = %d, want %d", len(got), len(tt.want))
			}
			for index := range tt.want {
				if got[index] != tt.want[index] {
					t.Fatalf("got[%d] = %d, want %d", index, got[index], tt.want[index])
				}
			}
		})
	}
}

func TestDiffStringSet(t *testing.T) {
	toAssign, toRemove := diffStringSet([]string{"admin", "viewer"}, []string{"viewer", "editor"})

	if len(toAssign) != 1 || toAssign[0] != "editor" {
		t.Fatalf("toAssign = %v, want [editor]", toAssign)
	}
	if len(toRemove) != 1 || toRemove[0] != "admin" {
		t.Fatalf("toRemove = %v, want [admin]", toRemove)
	}
}
