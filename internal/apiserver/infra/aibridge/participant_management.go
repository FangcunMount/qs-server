package aibridge

import (
	"context"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"strconv"
	"time"
)

type ParticipantClient struct {
	RPC pb.ParticipantManagementClient
}

func NewParticipantClient(conn grpc.ClientConnInterface) *ParticipantClient {
	return &ParticipantClient{RPC: pb.NewParticipantManagementClient(conn)}
}
func participantUsage(r *pb.ParticipantCapacityUsage, identity string) (*app.ParticipantCapacityUsage, error) {
	if identity == "" {
		if r != nil {
			return nil, app.ErrConflict
		}
		return nil, nil
	}
	if r == nil || r.Identity != identity || r.DailyReserved < 0 || r.DailyRemaining < 0 || r.Active < 0 || r.ActiveRemaining < 0 {
		return nil, app.ErrConflict
	}
	return &app.ParticipantCapacityUsage{Identity: r.Identity, DailyReserved: r.DailyReserved, DailyRemaining: r.DailyRemaining, Active: r.Active, ActiveRemaining: r.ActiveRemaining}, nil
}
func participantReservations(records []*pb.ParticipantReservation) ([]app.ParticipantReservation, error) {
	if len(records) > 100 {
		return nil, app.ErrConflict
	}
	result := make([]app.ParticipantReservation, 0, len(records))
	seen := map[string]bool{}
	for _, r := range records {
		if r == nil || seen[r.RunId] || len(r.SubjectId) == 0 || len(r.SubjectId) > 128 || len(r.AssessmentIds) < 1 || len(r.AssessmentIds) > 10 {
			return nil, app.ErrConflict
		}
		for _, key := range []string{r.RunId, r.SessionId} {
			id, err := uuid.Parse(key)
			if err != nil || id == uuid.Nil || id.String() != key {
				return nil, app.ErrConflict
			}
		}
		for _, id := range r.AssessmentIds {
			parsed, err := strconv.ParseUint(id, 10, 64)
			if err != nil || parsed == 0 || strconv.FormatUint(parsed, 10) != id {
				return nil, app.ErrConflict
			}
		}
		at, err := time.Parse(time.RFC3339Nano, r.ReservedAt)
		if err != nil || at.UTC().Format("2006-01-02") != r.BudgetDay {
			return nil, app.ErrConflict
		}
		if r.Active {
			if _, err := time.Parse(time.RFC3339Nano, r.AcquiredAt); err != nil {
				return nil, app.ErrConflict
			}
		}
		seen[r.RunId] = true
		result = append(result, app.ParticipantReservation{RunID: r.RunId, SessionID: r.SessionId, SubjectID: r.SubjectId, AssessmentIDs: r.AssessmentIds, BudgetDay: r.BudgetDay, ReservedAt: r.ReservedAt, Active: r.Active, AcquiredAt: r.AcquiredAt})
	}
	return result, nil
}
func (c *ParticipantClient) GetParticipantCapacity(ctx context.Context, scope app.DraftScope, query app.ParticipantCapacityQuery) (app.ParticipantCapacity, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.GetCapacity(ctx, &pb.ParticipantCapacityQuery{Scope: draftScope(scope), SubjectId: query.SubjectID, AssessmentId: query.AssessmentID}, grpc.MaxCallRecvMsgSize(256*1024))
	if err != nil {
		return app.ParticipantCapacity{}, err
	}
	if r == nil || r.OrganizationId != scope.OrganizationID || r.Policy == nil {
		return app.ParticipantCapacity{}, app.ErrConflict
	}
	if _, err = time.Parse("2006-01-02", r.BudgetDay); err != nil {
		return app.ParticipantCapacity{}, app.ErrConflict
	}
	for _, limit := range []int64{r.Policy.DailyOrg, r.Policy.DailyUser, r.Policy.DailyAssessment, r.Policy.ActiveOrg, r.Policy.ActiveUser, r.Policy.ActiveAssessment} {
		if limit < 1 {
			return app.ParticipantCapacity{}, app.ErrConflict
		}
	}
	org, err := participantUsage(r.Organization, strconv.FormatInt(scope.OrganizationID, 10))
	if err != nil {
		return app.ParticipantCapacity{}, err
	}
	user, err := participantUsage(r.Subject, query.SubjectID)
	if err != nil {
		return app.ParticipantCapacity{}, err
	}
	assessment, err := participantUsage(r.Assessment, query.AssessmentID)
	if err != nil {
		return app.ParticipantCapacity{}, err
	}
	daily, err := participantReservations(r.DailyReservations)
	if err != nil {
		return app.ParticipantCapacity{}, err
	}
	active, err := participantReservations(r.ActiveReservations)
	if err != nil {
		return app.ParticipantCapacity{}, err
	}
	return app.ParticipantCapacity{OrganizationID: r.OrganizationId, BudgetDay: r.BudgetDay,
		Policy:       app.ParticipantCapacityPolicy{DailyOrg: r.Policy.DailyOrg, DailyUser: r.Policy.DailyUser, DailyAssessment: r.Policy.DailyAssessment, ActiveOrg: r.Policy.ActiveOrg, ActiveUser: r.Policy.ActiveUser, ActiveAssessment: r.Policy.ActiveAssessment},
		Organization: *org, Subject: user, Assessment: assessment, DailyReservations: daily, ActiveReservations: active, DailyTruncated: r.DailyTruncated, ActiveTruncated: r.ActiveTruncated}, nil
}
