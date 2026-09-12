package aibridge

import (
	"context"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

type PublicationClient struct {
	RPC pb.PublicationManagementClient
}

func NewPublicationClient(conn grpc.ClientConnInterface) *PublicationClient {
	return &PublicationClient{RPC: pb.NewPublicationManagementClient(conn)}
}
func publicationScope(s app.PublicationScope) *pb.PublicationScope {
	return &pb.PublicationScope{OrganizationId: s.OrganizationID, OperatorUserId: s.OperatorUserID}
}
func publicationSelector(s app.PublicationSelector) *pb.PublicationSelector {
	return &pb.PublicationSelector{Audience: s.Audience, ModelKind: s.ModelKind, DecisionKind: s.DecisionKind, ModelCode: s.ModelCode, ModelVersion: s.ModelVersion}
}
func expectation(command app.PublicationCommand) (*pb.PublicationExpectation, error) {
	if !command.Valid() {
		return nil, app.ErrInvalid
	}
	e := command.Expected
	return &pb.PublicationExpectation{Selector: publicationSelector(e.Selector), Version: *e.Version, ActivePublicationId: e.ActivePublicationID}, nil
}
func (c *PublicationClient) PublishConfiguration(ctx context.Context, scope app.PublicationScope, command app.PublishConfiguration) (app.PublicationReceipt, error) {
	expected, err := expectation(command.PublicationCommand)
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Publish(ctx, &pb.PublicationPublishCommand{Scope: publicationScope(scope), CommandId: command.CommandID, Expected: expected, Reason: command.Reason, Confirm: command.Confirm, RunId: command.RunID, RunVersion: command.RunVersion, ReleaseFingerprint: command.ReleaseFingerprint}, grpc.MaxCallRecvMsgSize(1024*1024))
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	result, err := publicationReceipt(response, scope, command.CommandID)
	if err != nil || !confirmedPublication(result, command.PublicationCommand, "publish") {
		return app.PublicationReceipt{}, app.ErrConflict
	}
	proof, err := publicationDocument(result.Current)
	if err != nil || proof.Evidence.RunID != command.RunID || proof.Evidence.RunVersion != command.RunVersion || releaseDigest(proof.Evidence.Release) != command.ReleaseFingerprint {
		return app.PublicationReceipt{}, app.ErrConflict
	}
	return result, nil
}
func (c *PublicationClient) RollbackPublication(ctx context.Context, scope app.PublicationScope, command app.RollbackPublication) (app.PublicationReceipt, error) {
	expected, err := expectation(command.PublicationCommand)
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Rollback(ctx, &pb.PublicationRollbackCommand{Scope: publicationScope(scope), CommandId: command.CommandID, Expected: expected, Reason: command.Reason, Confirm: command.Confirm, TargetPublicationId: command.TargetPublicationID}, grpc.MaxCallRecvMsgSize(1024*1024))
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	result, err := publicationReceipt(response, scope, command.CommandID)
	if err != nil || !confirmedPublication(result, command.PublicationCommand, "rollback") || result.Current.ActivePublicationID != command.TargetPublicationID {
		return app.PublicationReceipt{}, app.ErrConflict
	}
	return result, nil
}
func (c *PublicationClient) DisablePublication(ctx context.Context, scope app.PublicationScope, command app.PublicationCommand) (app.PublicationReceipt, error) {
	expected, err := expectation(command)
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Disable(ctx, &pb.PublicationDisableCommand{Scope: publicationScope(scope), CommandId: command.CommandID, Expected: expected, Reason: command.Reason, Confirm: command.Confirm}, grpc.MaxCallRecvMsgSize(1024*1024))
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	result, err := publicationReceipt(response, scope, command.CommandID)
	if err != nil || !confirmedPublication(result, command, "disable") {
		return app.PublicationReceipt{}, app.ErrConflict
	}
	return result, nil
}
func (c *PublicationClient) GetPublication(ctx context.Context, scope app.PublicationScope, selector app.PublicationSelector) (app.PublicationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Get(ctx, &pb.PublicationQuery{Scope: publicationScope(scope), Selector: publicationSelector(selector)}, grpc.MaxCallRecvMsgSize(512*1024))
	if err != nil {
		return app.PublicationState{}, err
	}
	result, err := publicationState(response)
	if err != nil || !result.Selector.Equal(selector) {
		return app.PublicationState{}, app.ErrConflict
	}
	return result, nil
}
func (c *PublicationClient) GetPublicationReceipt(ctx context.Context, scope app.PublicationScope, commandID string) (app.PublicationReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.GetReceipt(ctx, &pb.PublicationReceiptQuery{Scope: publicationScope(scope), CommandId: commandID}, grpc.MaxCallRecvMsgSize(1024*1024))
	if err != nil {
		return app.PublicationReceipt{}, err
	}
	return publicationReceipt(response, scope, commandID)
}
func confirmedPublication(result app.PublicationReceipt, command app.PublicationCommand, action string) bool {
	return command.Expected != nil && command.Expected.Version != nil && result.Action == action && result.Reason == command.Reason &&
		result.Previous.Version == *command.Expected.Version && result.Previous.ActivePublicationID == command.Expected.ActivePublicationID && result.Current.Selector.Equal(command.Expected.Selector)
}
