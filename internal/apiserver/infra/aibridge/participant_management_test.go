package aibridge

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

type participantCapacityRPC struct {
	t        *testing.T
	request  *pb.ParticipantCapacityQuery
	response *pb.ParticipantCapacitySnapshot
	err      error
	calls    int
}

func (s *participantCapacityRPC) GetCapacity(ctx context.Context, query *pb.ParticipantCapacityQuery, _ ...grpc.CallOption) (*pb.ParticipantCapacitySnapshot, error) {
	s.calls++
	s.request = query
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("unbounded query")
	}
	return s.response, s.err
}
func participantSnapshot() *pb.ParticipantCapacitySnapshot {
	return &pb.ParticipantCapacitySnapshot{
		OrganizationId: 12, BudgetDay: "2026-09-13",
		Policy:       &pb.ParticipantCapacityPolicy{DailyOrg: 500, DailyUser: 5, DailyAssessment: 3, ActiveOrg: 10, ActiveUser: 2, ActiveAssessment: 1},
		Organization: &pb.ParticipantCapacityUsage{Identity: "12", DailyRemaining: 499, ActiveRemaining: 9},
		Subject:      &pb.ParticipantCapacityUsage{Identity: "user:42", DailyRemaining: 4},
		Assessment:   &pb.ParticipantCapacityUsage{Identity: "42", DailyRemaining: 2},
	}
}
func TestParticipantClientPreservesFiltersAndServerCapacity(t *testing.T) {
	rpc := &participantCapacityRPC{t: t, response: participantSnapshot()}
	client := &ParticipantClient{RPC: rpc}
	scope := app.DraftScope{OrganizationID: 12, OperatorUserID: 34}
	query := app.ParticipantCapacityQuery{SubjectID: "user:42", AssessmentID: "42"}
	value, err := client.GetParticipantCapacity(context.Background(), scope, query)
	if err != nil || value.Subject.DailyRemaining != 4 || value.Assessment.DailyRemaining != 2 || rpc.request.Scope.OrganizationId != 12 || rpc.request.Scope.OperatorUserId != 34 || rpc.request.SubjectId != "user:42" || rpc.request.AssessmentId != "42" {
		t.Fatal(value, err, rpc.request)
	}
	rpc.err = context.DeadlineExceeded
	if _, err = client.GetParticipantCapacity(context.Background(), scope, query); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal("read retried unexpectedly", err, rpc.calls)
	}
}
func TestParticipantClientRejectsUnboundOrMalformedCapacity(t *testing.T) {
	for _, name := range []string{"organization", "subject_missing", "subject_other", "assessment", "policy", "negative", "date", "receipt"} {
		t.Run(name, func(t *testing.T) {
			value := participantSnapshot()
			switch name {
			case "organization":
				value.OrganizationId = 99
			case "subject_missing":
				value.Subject = nil
			case "subject_other":
				value.Subject.Identity = "other"
			case "assessment":
				value.Assessment.Identity = "77"
			case "policy":
				value.Policy.ActiveOrg = 0
			case "negative":
				value.Organization.DailyReserved = -1
			case "date":
				value.BudgetDay = "not-a-date"
			case "receipt":
				value.ActiveReservations = []*pb.ParticipantReservation{{RunId: "invalid"}}
			}
			rpc := &participantCapacityRPC{t: t, response: value}
			_, err := (&ParticipantClient{RPC: rpc}).GetParticipantCapacity(context.Background(), app.DraftScope{OrganizationID: 12, OperatorUserID: 34}, app.ParticipantCapacityQuery{SubjectID: "user:42", AssessmentID: "42"})
			if !errors.Is(err, app.ErrConflict) {
				t.Fatal(err)
			}
		})
	}
}
