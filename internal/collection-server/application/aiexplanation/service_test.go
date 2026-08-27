package aiexplanation

import (
	"context"
	"errors"
	"testing"

	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
)

type clientStub struct {
	capability *aiport.Output
	export     *aiport.ExportPage
	err        error
}

func (s clientStub) GetCapability(context.Context, uint64, uint64, string, []string) (*aiport.Output, error) {
	return s.capability, s.err
}
func (s clientStub) Request(context.Context, uint64, uint64, string, []string) (*aiport.Output, error) {
	return s.capability, s.err
}
func (s clientStub) Get(context.Context, uint64, uint64, string) (*aiport.Output, error) {
	return s.capability, s.err
}
func (s clientStub) Export(context.Context, uint64, int, string) (*aiport.ExportPage, error) {
	return s.export, s.err
}

func TestServiceMapsDisabledBackendToCapabilityState(t *testing.T) {
	service := NewService(clientStub{err: aiport.ErrDisabled})
	result, err := service.Capability(context.Background(), 7, 42, Request{Locale: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "not_applicable" || result.ReasonCode != "feature_disabled" {
		t.Fatalf("result = %#v", result)
	}
}

func TestServicePreservesBackendStateAndErrors(t *testing.T) {
	want := &aiport.Output{Status: "pending", GenerationID: "9001", SourceState: "current"}
	service := NewService(clientStub{capability: want})
	got, err := service.Request(context.Background(), 7, 42, Request{Locale: "zh-CN"})
	if err != nil || got != want {
		t.Fatalf("result = %#v, %v", got, err)
	}

	backendErr := errors.New("backend unavailable")
	service = NewService(clientStub{err: backendErr})
	if _, err := service.Get(context.Background(), 7, 42, "9001"); !errors.Is(err, backendErr) {
		t.Fatalf("error = %v", err)
	}
}

func TestServiceRejectsMalformedBackendLifecycleState(t *testing.T) {
	tests := []struct {
		name   string
		result *aiport.Output
	}{
		{name: "unknown status", result: &aiport.Output{Status: "done", SourceState: "current"}},
		{name: "unknown source state", result: &aiport.Output{Status: "pending", GenerationID: "9001", SourceState: "latest"}},
		{name: "pending without generation", result: &aiport.Output{Status: "pending", SourceState: "current"}},
		{name: "unknown reason", result: &aiport.Output{Status: "not_applicable", ReasonCode: "future_unbounded_reason", SourceState: "current"}},
		{name: "reason does not match status", result: &aiport.Output{Status: "not_ready", ReasonCode: aiport.ReasonCodeProfileUnresolved, SourceState: "unavailable"}},
		{name: "lifecycle state contains reason", result: &aiport.Output{Status: "pending", ReasonCode: aiport.ReasonCodeProfileUnresolved, GenerationID: "9001", SourceState: "current"}},
		{name: "generated without artifact", result: &aiport.Output{
			Status: "generated", GenerationID: "9001", SourceState: "current",
			Content: &aiport.Content{SchemaVersion: "ai-explanation-output/v1"},
		}},
		{name: "failed without receipt", result: &aiport.Output{Status: "failed", GenerationID: "9001", SourceState: "current"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewService(clientStub{capability: testCase.result})
			if _, err := service.Request(context.Background(), 7, 42, Request{Locale: "zh-CN"}); err == nil {
				t.Fatal("expected malformed upstream lifecycle state rejection")
			}
		})
	}
}

func TestServiceAcceptsBoundedReasonCodes(t *testing.T) {
	tests := []struct {
		name   string
		status string
		reason string
	}{
		{name: "standard report not ready", status: "not_ready", reason: aiport.ReasonCodeStandardReportNotReady},
		{name: "feature disabled", status: "not_applicable", reason: aiport.ReasonCodeFeatureDisabled},
		{name: "source not supported", status: "not_applicable", reason: aiport.ReasonCodeSourceNotSupported},
		{name: "profile unresolved", status: "not_applicable", reason: aiport.ReasonCodeProfileUnresolved},
		{name: "profile mismatch", status: "not_applicable", reason: aiport.ReasonCodeProfileMismatch},
		{name: "input not applicable", status: "not_applicable", reason: aiport.ReasonCodeNotApplicable},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewService(clientStub{capability: &aiport.Output{
				Status: testCase.status, ReasonCode: testCase.reason, SourceState: "current",
			}})
			if _, err := service.Capability(context.Background(), 7, 42, Request{Locale: "zh-CN"}); err != nil {
				t.Fatalf("bounded reason rejected: %v", err)
			}
		})
	}
}

func TestServiceClassifiesInvalidRequest(t *testing.T) {
	service := NewService(clientStub{})
	if _, err := service.Capability(context.Background(), 7, 42, Request{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("capability error = %v", err)
	}
	focusAreas := make([]string, 4)
	if _, err := service.Request(context.Background(), 7, 42, Request{Locale: "zh-CN", FocusAreas: focusAreas}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("request error = %v", err)
	}
	if _, err := service.Get(context.Background(), 7, 42, ""); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("get error = %v", err)
	}
	if _, err := service.Export(context.Background(), 0, 0, ""); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("export error = %v", err)
	}
}

func TestServiceExportFailsClosedAndValidatesSubject(t *testing.T) {
	service := NewService(nil)
	if _, err := service.Export(context.Background(), 7, 0, ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled export error = %v", err)
	}
	want := &aiport.ExportPage{SchemaVersion: "ai-explanation-subject-export/v1", OrgID: 9, TesteeID: 7, Items: []aiport.ExportItem{}}
	service = NewService(clientStub{export: want})
	got, err := service.Export(context.Background(), 7, 50, "")
	if err != nil || got != want {
		t.Fatalf("export result = %#v, %v", got, err)
	}
	service = NewService(clientStub{export: &aiport.ExportPage{SchemaVersion: want.SchemaVersion, TesteeID: 8, Items: []aiport.ExportItem{}}})
	if _, err := service.Export(context.Background(), 7, 50, ""); err == nil {
		t.Fatal("expected cross-subject export rejection")
	}
}
