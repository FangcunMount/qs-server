package aibridge

import (
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

type Client struct{ RPC pb.CommandsClient }

func New(conn grpc.ClientConnInterface) *Client { return &Client{pb.NewCommandsClient(conn)} }
func evidenceMessages(items []app.EvidenceItem) []*pb.EvidenceItem {
	result := make([]*pb.EvidenceItem, 0, len(items))
	for _, item := range items {
		message := &pb.EvidenceItem{AssessmentId: item.AssessmentID, TesteeId: item.TesteeID, ReportId: item.ReportID, SourceVersion: item.SourceVersion}
		for _, fact := range item.Facts {
			message.Facts = append(message.Facts, &pb.Fact{Ref: fact.Ref, Value: fact.Value})
		}
		result = append(result, message)
	}
	return result
}
