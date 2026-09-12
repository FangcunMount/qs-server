package operatorretirement

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Recovery authorizes a new operator lifecycle, without restoring retired roles or doctor bindings.
// The completed exit must be archived before its mutation block can be removed.
type Recovery struct {
	RequestID              string
	OperatorID             uint64
	OrgID, UserID, ActorID int64
	ExpectedVersion        uint32
	PolicyVersion          int64
	RetirementRequestID    string
	Reason                 string
	CreatedAt              time.Time
}

func NewRecovery(task Task, actor int64, version uint32, policyVersion int64, requestID, reason string, now time.Time) (Recovery, error) {
	if err := task.Validate(); err != nil {
		return Recovery{}, err
	}
	if task.Stage != Completed || task.ExpectedVersion > math.MaxUint32-3 || version != task.ExpectedVersion+2 || policyVersion < task.PolicyVersion {
		return Recovery{}, fmt.Errorf("completed retirement and converged authorization required")
	}
	r := Recovery{RequestID: strings.TrimSpace(requestID), OperatorID: task.OperatorID, OrgID: task.OrgID, UserID: task.UserID, ActorID: actor, ExpectedVersion: version, PolicyVersion: policyVersion, RetirementRequestID: task.RequestID, Reason: strings.TrimSpace(reason), CreatedAt: now}
	return r, r.Validate()
}

func (r Recovery) Validate() error {
	if r.OperatorID == 0 || r.OrgID <= 0 || r.UserID <= 0 || r.ActorID <= 0 || r.ActorID == r.UserID || r.ExpectedVersion == 0 || r.ExpectedVersion == math.MaxUint32 || r.PolicyVersion <= 0 || r.CreatedAt.IsZero() {
		return fmt.Errorf("invalid operator recovery identity or version")
	}
	if strings.TrimSpace(r.RequestID) != r.RequestID || r.RequestID == "" || len(r.RequestID) > 64 || r.RetirementRequestID == "" || len(r.RetirementRequestID) > 64 || strings.TrimSpace(r.Reason) != r.Reason || r.Reason == "" || len([]rune(r.Reason)) > 500 {
		return fmt.Errorf("recovery request and reason required")
	}
	return nil
}
