package aibridge

import (
	"context"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

func (c *Client) CheckEligibility(ctx context.Context, actor app.Actor, testee string, assessments []string, evidence []app.EvidenceItem) (*app.Eligibility, error) {
	if c == nil || c.RPC == nil {
		return nil, app.ErrAccessUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := c.RPC.CheckEligibility(ctx, &pb.EligibilityQuery{
		Actor: &pb.Actor{OrgId: actor.OrgID, SubjectId: actor.SubjectID}, TesteeId: testee,
		AssessmentIds: assessments, Evidence: evidenceMessages(evidence),
	})
	if status.Code(err) == codes.PermissionDenied {
		return nil, app.ErrAccessDenied
	}
	if err != nil || result == nil {
		return nil, app.ErrAccessUnavailable
	}
	value := &app.Eligibility{Status: result.Status, ReasonCode: result.ReasonCode}
	if !value.Valid() {
		return nil, app.ErrAccessUnavailable
	}
	return value, nil
}
