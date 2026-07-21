package answersheet

import (
	"errors"
	"strings"
	"time"
)

type OriginType string

const (
	OriginTypeAssessmentEntry OriginType = "assessment_entry"
	OriginTypePlanTask        OriginType = "plan_task"
	OriginTypeClinicianDirect OriginType = "clinician_direct"
	OriginTypeSelfService     OriginType = "self_service"
)

type OriginRef struct {
	Type OriginType
	ID   string
}

func (r OriginRef) Validate() error {
	switch r.Type {
	case OriginTypeAssessmentEntry, OriginTypePlanTask, OriginTypeClinicianDirect:
		if strings.TrimSpace(r.ID) == "" {
			return errors.New("origin id is required")
		}
	case OriginTypeSelfService:
		if strings.TrimSpace(r.ID) != "" {
			return errors.New("self_service origin must not carry id")
		}
	default:
		return errors.New("unsupported origin type")
	}
	return nil
}

type AttributionMode string

const (
	AttributionModeFrozen        AttributionMode = "frozen"
	AttributionModeDerivedLegacy AttributionMode = "derived_legacy"
	AttributionModeUnknown       AttributionMode = "unknown"
)

// AttributionSnapshot freezes the organizational ownership used at durable
// acceptance time. Later Actor, Entry or Plan changes must not rewrite it.
type AttributionSnapshot struct {
	originType   OriginType
	originID     string
	clinicianID  string
	entryID      string
	planID       string
	enrollmentID string
	taskID       string
	capturedAt   time.Time
	version      uint32
	mode         AttributionMode
}

func NewAttributionSnapshot(ref OriginRef, clinicianID, entryID, planID, enrollmentID, taskID string, capturedAt time.Time) (AttributionSnapshot, error) {
	if err := ref.Validate(); err != nil {
		return AttributionSnapshot{}, err
	}
	if capturedAt.IsZero() {
		return AttributionSnapshot{}, errors.New("attribution captured_at is required")
	}
	snapshot := AttributionSnapshot{
		originType: ref.Type, originID: strings.TrimSpace(ref.ID), clinicianID: strings.TrimSpace(clinicianID),
		entryID: strings.TrimSpace(entryID), planID: strings.TrimSpace(planID), enrollmentID: strings.TrimSpace(enrollmentID),
		taskID: strings.TrimSpace(taskID), capturedAt: capturedAt, version: 1, mode: AttributionModeFrozen,
	}
	return snapshot, nil
}

func ReconstructAttributionSnapshot(originType OriginType, originID, clinicianID, entryID, planID, enrollmentID, taskID string, capturedAt time.Time, version uint32, mode AttributionMode) AttributionSnapshot {
	return AttributionSnapshot{originType: originType, originID: originID, clinicianID: clinicianID, entryID: entryID, planID: planID, enrollmentID: enrollmentID, taskID: taskID, capturedAt: capturedAt, version: version, mode: mode}
}

func (s AttributionSnapshot) IsZero() bool           { return s.originType == "" }
func (s AttributionSnapshot) OriginType() OriginType { return s.originType }
func (s AttributionSnapshot) OriginID() string       { return s.originID }
func (s AttributionSnapshot) ClinicianID() string    { return s.clinicianID }
func (s AttributionSnapshot) EntryID() string        { return s.entryID }
func (s AttributionSnapshot) PlanID() string         { return s.planID }
func (s AttributionSnapshot) EnrollmentID() string   { return s.enrollmentID }
func (s AttributionSnapshot) TaskID() string         { return s.taskID }
func (s AttributionSnapshot) CapturedAt() time.Time  { return s.capturedAt }
func (s AttributionSnapshot) Version() uint32        { return s.version }
func (s AttributionSnapshot) Mode() AttributionMode  { return s.mode }
