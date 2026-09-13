package aiexplanation

import (
	"context"
	"errors"
	"testing"

	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
)

type workflowReadClient struct {
	aiport.Client
	err    error
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
			client := &workflowReadClient{err: tc.err, result: tc.result}
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

type workflowSourceClient struct {
	aiport.Client
	err    error
	result *aiport.WorkflowSource
}

func (c *workflowSourceClient) GetWorkflowSource(_ context.Context, testee, assessment uint64) (*aiport.WorkflowSource, error) {
	if testee != 7 || assessment != 42 {
		return nil, ErrInvalidRequest
	}
	return c.result, c.err
}
func TestWorkflowSourceRejectsMalformedOrInapplicableProvenance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *aiport.WorkflowSource
		valid  bool
	}{
		{"ready", &aiport.WorkflowSource{Status: "ready", ReportID: "18446744073709551615", SourceVersion: "standard-v1:101"}, true},
		{"not ready", &aiport.WorkflowSource{Status: "not_ready"}, true},
		{"not applicable", &aiport.WorkflowSource{Status: "not_applicable"}, true},
		{"missing", nil, false},
		{"zero ID", &aiport.WorkflowSource{Status: "ready", ReportID: "0", SourceVersion: "v1"}, false},
		{"missing version", &aiport.WorkflowSource{Status: "ready", ReportID: "99"}, false},
		{"unexpected state", &aiport.WorkflowSource{Status: "published"}, false},
		{"inapplicable identity", &aiport.WorkflowSource{Status: "not_applicable", ReportID: "99"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &workflowSourceClient{result: tc.result}
			got, err := NewService(client).GetWorkflowSource(context.Background(), 7, 42)
			if (err == nil) != tc.valid || tc.valid && got != tc.result {
				t.Fatalf("got=%#v err=%v", got, err)
			}
		})
	}
	denied := errors.New("denied")
	if _, err := NewService(&workflowSourceClient{err: denied}).GetWorkflowSource(context.Background(), 7, 42); !errors.Is(err, denied) {
		t.Fatal("authorization error hidden")
	}
}
