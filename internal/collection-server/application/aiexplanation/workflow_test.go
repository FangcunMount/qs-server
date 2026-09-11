package aiexplanation

import (
	"context"
	"errors"
	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
	"testing"
)

type workflowReadClient struct {
	clientStub
	result *aiport.WorkflowResult
	called bool
}

func (c *workflowReadClient) GetWorkflow(_ context.Context, testee, assessment uint64, request string) (*aiport.WorkflowResult, error) {
	c.called = true
	if testee != 7 || assessment != 42 {
		return nil, ErrInvalidRequest
	}
	return c.result, c.err
}
func TestWorkflowReadPreservesCorrelationAndAccessErrors(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	denied := errors.New("access denied")
	for _, tc := range []struct {
		name, request string
		result        *aiport.WorkflowResult
		err, want     error
		called        bool
	}{
		{name: "pending", request: id, result: &aiport.WorkflowResult{RequestID: id, Status: "accepted"}, called: true},
		{name: "revoked", request: id, err: denied, want: denied, called: true},
		{name: "other request", request: id, result: &aiport.WorkflowResult{RequestID: "other"}, want: ErrUnavailable, called: true},
		{name: "missing response", request: id, want: ErrUnavailable, called: true},
		{name: "invalid id", request: "123", want: ErrInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &workflowReadClient{clientStub: clientStub{err: tc.err}, result: tc.result}
			got, err := NewService(client).GetWorkflow(context.Background(), 7, 42, tc.request)
			if !errors.Is(err, tc.want) || client.called != tc.called {
				t.Fatalf("error=%v called=%v", err, client.called)
			}
			if err == nil && got != tc.result {
				t.Fatal("response changed")
			}
		})
	}
}
