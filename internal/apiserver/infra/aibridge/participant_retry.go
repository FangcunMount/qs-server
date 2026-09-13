package aibridge

import (
	"context"
	"errors"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

func participantID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func (c *ParticipantClient) GetParticipantExecution(ctx context.Context, scope app.DraftScope, sessionID string) (app.ParticipantExecution, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.GetExecution(ctx, &pb.ParticipantExecutionQuery{Scope: draftScope(scope), SessionId: sessionID}, grpc.MaxCallRecvMsgSize(16384))
	if err != nil {
		return app.ParticipantExecution{}, err
	}
	if r == nil || r.OrganizationId != scope.OrganizationID || r.SessionId != sessionID || r.Version < 1 || !participantID(r.RunId) || !participantID(r.RequestId) || r.SubjectId == "" || len(r.SubjectId) > 128 || r.RetryProviderInvocations != 1 || len(r.AssessmentIds) < 1 || len(r.AssessmentIds) > 10 || len(r.FailureCode) > 64 {
		return app.ParticipantExecution{}, app.ErrConflict
	}
	for _, id := range append([]string{r.TesteeId}, r.AssessmentIds...) {
		if !participantNumber(id) {
			return app.ParticipantExecution{}, app.ErrConflict
		}
	}
	if (r.SourceRunId != "" && !participantID(r.SourceRunId)) || (r.InvocationId != "" && !participantID(r.InvocationId)) {
		return app.ParticipantExecution{}, app.ErrConflict
	}
	switch r.Status {
	case "queued", "running", "awaiting_answer", "blocked", "cancelled", "completed":
	default:
		return app.ParticipantExecution{}, app.ErrConflict
	}
	switch r.ModelCallStatus {
	case "", "dispatched", "unknown", "failed", "response_received":
	default:
		return app.ParticipantExecution{}, app.ErrConflict
	}
	if (r.CanRetry && r.Status != "blocked") || r.UnknownResultRisk != (r.ModelCallStatus == "dispatched" || r.ModelCallStatus == "unknown") || (r.ModelCallStatus == "") != (r.InvocationId == "") {
		return app.ParticipantExecution{}, app.ErrConflict
	}
	return app.ParticipantExecution{OrganizationID: r.OrganizationId, SessionID: r.SessionId, RequestID: r.RequestId, RunID: r.RunId, Version: r.Version, Status: r.Status, SubjectID: r.SubjectId, TesteeID: r.TesteeId, AssessmentIDs: r.AssessmentIds, FailureCode: r.FailureCode, ModelCallStatus: r.ModelCallStatus, InvocationID: r.InvocationId, SourceRunID: r.SourceRunId, CanRetry: r.CanRetry, UnknownResultRisk: r.UnknownResultRisk, RetryProviderInvocations: r.RetryProviderInvocations}, nil
}
func participantReceipt(r *pb.Receipt) (app.Receipt, error) {
	if r == nil || !participantID(r.SessionId) || !participantID(r.RunId) || r.Version < 2 || r.Status != "queued" {
		return app.Receipt{}, app.ErrConflict
	}
	return app.Receipt{SessionID: r.SessionId, RunID: r.RunId, Status: r.Status, Version: r.Version}, nil
}
func (c *ParticipantClient) RetryParticipant(ctx context.Context, scope app.DraftScope, sessionID string, command app.ParticipantRetry) (app.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.Retry(ctx, &pb.ParticipantRetryCommand{Scope: draftScope(scope), SessionId: sessionID, CommandId: command.CommandID, ExpectedRunId: command.ExpectedRunID, ExpectedVersion: command.ExpectedVersion, Reason: command.Reason, Confirm: command.Confirm, ExpectedProviderInvocations: command.ExpectedProviderInvocations, AcceptResultUnknownRisk: command.AcceptResultUnknownRisk}, grpc.MaxCallRecvMsgSize(4096))
	if err != nil {
		return app.Receipt{}, err
	}
	value, err := participantReceipt(r)
	if err != nil || value.SessionID != sessionID || value.RunID == command.ExpectedRunID || value.Version != command.ExpectedVersion+1 {
		return app.Receipt{}, errors.New("participant retry outcome unknown; query original command")
	}
	return value, nil
}
func (c *ParticipantClient) GetParticipantRetryReceipt(ctx context.Context, scope app.DraftScope, commandID string) (app.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.GetRetryReceipt(ctx, &pb.ParticipantRetryReceiptQuery{Scope: draftScope(scope), CommandId: commandID}, grpc.MaxCallRecvMsgSize(4096))
	if err != nil {
		return app.Receipt{}, err
	}
	return participantReceipt(r)
}

func participantNumber(value string) bool {
	n, err := strconv.ParseUint(value, 10, 64)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == value
}
