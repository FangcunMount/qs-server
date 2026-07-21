package plan

import (
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type PlanEnrollmentID = meta.ID

type EnrollmentStatus string

const (
	EnrollmentStatusActive     EnrollmentStatus = "active"
	EnrollmentStatusClosed     EnrollmentStatus = "closed"
	EnrollmentStatusTerminated EnrollmentStatus = "terminated"
)

type EnrollmentRecordOrigin string

const (
	EnrollmentRecordOriginNative        EnrollmentRecordOrigin = "native"
	EnrollmentRecordOriginDerivedLegacy EnrollmentRecordOrigin = "derived_legacy"
)

// Enrollment 是一个患者在一个时间段内持续履行同一种测评计划的业务事实。
// round 使同一患者终止后可以再次加入同一个 Plan，而不会覆盖历史任务。
type Enrollment struct {
	id               PlanEnrollmentID
	orgID            int64
	planID           AssessmentPlanID
	testeeID         testee.ID
	round            uint32
	startDate        time.Time
	status           EnrollmentStatus
	joinedAt         time.Time
	closedAt         *time.Time
	terminatedAt     *time.Time
	terminatedReason string
	recordOrigin     EnrollmentRecordOrigin
}

func NewEnrollment(orgID int64, planID AssessmentPlanID, testeeID testee.ID, round uint32, startDate, joinedAt time.Time) *Enrollment {
	return &Enrollment{
		id:           meta.New(),
		orgID:        orgID,
		planID:       planID,
		testeeID:     testeeID,
		round:        round,
		startDate:    startDate,
		status:       EnrollmentStatusActive,
		joinedAt:     joinedAt,
		recordOrigin: EnrollmentRecordOriginNative,
	}
}

func (e *Enrollment) ID() PlanEnrollmentID                 { return e.id }
func (e *Enrollment) OrgID() int64                         { return e.orgID }
func (e *Enrollment) PlanID() AssessmentPlanID             { return e.planID }
func (e *Enrollment) TesteeID() testee.ID                  { return e.testeeID }
func (e *Enrollment) Round() uint32                        { return e.round }
func (e *Enrollment) StartDate() time.Time                 { return e.startDate }
func (e *Enrollment) Status() EnrollmentStatus             { return e.status }
func (e *Enrollment) JoinedAt() time.Time                  { return e.joinedAt }
func (e *Enrollment) ClosedAt() *time.Time                 { return e.closedAt }
func (e *Enrollment) TerminatedAt() *time.Time             { return e.terminatedAt }
func (e *Enrollment) TerminatedReason() string             { return e.terminatedReason }
func (e *Enrollment) RecordOrigin() EnrollmentRecordOrigin { return e.recordOrigin }
func (e *Enrollment) IsActive() bool                       { return e.status == EnrollmentStatusActive }

func (e *Enrollment) Close(at time.Time) {
	if !e.IsActive() {
		return
	}
	e.status = EnrollmentStatusClosed
	e.closedAt = &at
}

func (e *Enrollment) Terminate(at time.Time, reason string) {
	if !e.IsActive() {
		return
	}
	e.status = EnrollmentStatusTerminated
	e.terminatedAt = &at
	e.terminatedReason = strings.TrimSpace(reason)
}

func RestoreEnrollment(
	id PlanEnrollmentID,
	orgID int64,
	planID AssessmentPlanID,
	testeeID testee.ID,
	round uint32,
	startDate time.Time,
	status EnrollmentStatus,
	joinedAt time.Time,
	closedAt, terminatedAt *time.Time,
	terminatedReason string,
	recordOrigin EnrollmentRecordOrigin,
) *Enrollment {
	return &Enrollment{
		id: id, orgID: orgID, planID: planID, testeeID: testeeID, round: round,
		startDate: startDate, status: status, joinedAt: joinedAt, closedAt: closedAt,
		terminatedAt: terminatedAt, terminatedReason: terminatedReason, recordOrigin: recordOrigin,
	}
}
