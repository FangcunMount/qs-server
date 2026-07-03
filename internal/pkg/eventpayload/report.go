package eventpayload

import "time"

// ReportGeneratedData is the legacy report generated event body.
type ReportGeneratedData struct {
	ReportID     string    `json:"report_id"`
	AssessmentID string    `json:"assessment_id"`
	TesteeID     uint64    `json:"testee_id"`
	ScaleCode    string    `json:"scale_code"`
	ScaleVersion string    `json:"scale_version"`
	TotalScore   float64   `json:"total_score"`
	RiskLevel    string    `json:"risk_level"`
	GeneratedAt  time.Time `json:"generated_at"`
}
