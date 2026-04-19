package assessment

import (
	"testing"
	"time"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
)

func TestToReportResultIncludesCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, time.April, 19, 18, 8, 30, 0, time.Local)
	rpt := domainReport.ReconstructInterpretReport(
		domainReport.NewID(615830360323797550),
		"SNAP-IV量表（18项）",
		"3adyDE",
		31,
		domainReport.RiskLevelMedium,
		"总体症状负担中度偏高，控制不理想。",
		nil,
		nil,
		createdAt,
		nil,
	)

	got := toReportResult(rpt)
	if got == nil {
		t.Fatal("expected report result")
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected createdAt %v, got %v", createdAt, got.CreatedAt)
	}
}
