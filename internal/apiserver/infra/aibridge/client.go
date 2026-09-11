package aibridge

import (
	"context"
	"encoding/json"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"time"
)

type Client struct{ RPC pb.CommandsClient }

func New(conn grpc.ClientConnInterface) *Client { return &Client{pb.NewCommandsClient(conn)} }
func (c *Client) Send(ctx context.Context, command app.Command) (app.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var response *pb.Receipt
	var err error
	if command.Kind == "start" {
		var r app.Start
		if err = json.Unmarshal(command.Payload, &r); err != nil {
			return app.Receipt{}, err
		}
		response, err = c.RPC.Start(ctx, &pb.StartCommand{RequestId: r.RequestID, Actor: &pb.Actor{OrgId: r.Actor.OrgID, SubjectId: r.Actor.SubjectID}, TesteeId: r.TesteeID, AssessmentIds: r.AssessmentIDs, Goal: r.Goal})
	} else {
		var r app.Change
		if err = json.Unmarshal(command.Payload, &r); err != nil {
			return app.Receipt{}, err
		}
		response, err = c.RPC.Change(ctx, &pb.ChangeCommand{CommandId: r.CommandID, SessionId: r.SessionID, Actor: &pb.Actor{OrgId: r.Actor.OrgID, SubjectId: r.Actor.SubjectID}, Action: r.Action, ExpectedVersion: r.ExpectedVersion, QuestionId: r.QuestionID, Answer: r.Answer, Skip: r.Skip})
	}
	if err != nil {
		return app.Receipt{}, err
	}
	return app.Receipt{SessionID: response.SessionId, RunID: response.RunId, Version: response.Version, Status: response.Status}, nil
}
