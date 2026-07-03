package eventpayload

import "time"

type FootprintEntryOpenedData struct {
	OrgID       int64     `json:"org_id"`
	ClinicianID uint64    `json:"clinician_id"`
	EntryID     uint64    `json:"entry_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type FootprintIntakeConfirmedData struct {
	OrgID       int64     `json:"org_id"`
	ClinicianID uint64    `json:"clinician_id"`
	EntryID     uint64    `json:"entry_id"`
	TesteeID    uint64    `json:"testee_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type FootprintTesteeProfileCreatedData struct {
	OrgID       int64     `json:"org_id"`
	ClinicianID uint64    `json:"clinician_id"`
	EntryID     uint64    `json:"entry_id"`
	TesteeID    uint64    `json:"testee_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type FootprintCareRelationshipEstablishedData struct {
	OrgID       int64     `json:"org_id"`
	ClinicianID uint64    `json:"clinician_id"`
	EntryID     uint64    `json:"entry_id"`
	TesteeID    uint64    `json:"testee_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type FootprintCareRelationshipTransferredData struct {
	OrgID           int64     `json:"org_id"`
	FromClinicianID uint64    `json:"from_clinician_id"`
	ToClinicianID   uint64    `json:"to_clinician_id"`
	TesteeID        uint64    `json:"testee_id"`
	OccurredAt      time.Time `json:"occurred_at"`
}

type FootprintAnswerSheetSubmittedData struct {
	OrgID         int64     `json:"org_id"`
	TesteeID      uint64    `json:"testee_id"`
	AnswerSheetID uint64    `json:"answersheet_id"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type FootprintAssessmentCreatedData struct {
	OrgID         int64     `json:"org_id"`
	TesteeID      uint64    `json:"testee_id"`
	AnswerSheetID uint64    `json:"answersheet_id"`
	AssessmentID  uint64    `json:"assessment_id"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type FootprintReportGeneratedData struct {
	OrgID        int64     `json:"org_id"`
	TesteeID     uint64    `json:"testee_id"`
	AssessmentID uint64    `json:"assessment_id"`
	ReportID     uint64    `json:"report_id"`
	OccurredAt   time.Time `json:"occurred_at"`
}
