package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/gin-gonic/gin"
)

type definitionContractService struct {
	assessmentModelDefinitionStub
	called bool
}

func (s *definitionContractService) SaveDefinition(_ context.Context, _ modelcatalog.ActorContext, _ string, value *domain.Definition) (*domain.Definition, error) {
	s.called = true
	return value, s.saveErr
}

func TestDefinitionWritesRejectRetiredDerivedInputs(t *testing.T) {
	for _, tc := range []struct {
		name, body, input, decision, assets, result string
		status                                      int
	}{
		{"canonical", "{}", "canonical", "absent", "absent", "accepted", 200},
		{"both null", `{"DecisionSpec":null,"InterpretationAssets":null}`, "derived", "present", "present", "rejected", 400},
		{"empty decision", `{"DecisionSpec":{}}`, "derived", "present", "absent", "rejected", 400},
		{"empty assets", `{"InterpretationAssets":{}}`, "derived", "absent", "present", "rejected", 400},
		{"case folded keys", `{"decisionspec":null,"INTERPRETATIONASSETS":{}}`, "derived", "present", "present", "rejected", 400},
		{"nested fields are not input", `{"unknown":{"DecisionSpec":{}}}`, "canonical", "absent", "absent", "accepted", 200},
		{"trailing bytes preserve binder contract", `{} trailing`, "canonical", "absent", "absent", "accepted", 200},
		{"invalid decision", `{"DecisionSpec":42}`, "derived", "present", "absent", "rejected", 400},
		{"malformed", `{`, "unreadable", "unreadable", "unreadable", "rejected", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &definitionContractService{}
			h := NewAssessmentModelHandler(nil, svc, nil)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/assessment-models/model/definition", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			original := c.Request.Body
			setAssessmentModelActor(c)
			h.UpdateDefinition(c)
			if recorder.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.status, recorder.Body)
			}
			if tc.input == "derived" && !strings.Contains(recorder.Body.String(), "is derived; submit only") {
				t.Fatalf("missing actionable retired-field error: %s", recorder.Body)
			}
			if svc.called != (tc.result == "accepted") {
				t.Fatalf("service called=%t", svc.called)
			}
			if c.Request.Body != original {
				t.Fatal("body ownership changed")
			}
		})
	}
}

func TestDefinitionInputReadFailurePreservesBodyOwnership(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/definition", nil)
	original := &definitionErrorBody{}
	c.Request.Body = original
	setAssessmentModelActor(c)
	NewAssessmentModelHandler(nil, &definitionContractService{}, nil).UpdateDefinition(c)
	if recorder.Code != 500 || c.Request.Body != original || original.closed {
		t.Fatalf("status=%d closed=%t", recorder.Code, original.closed)
	}
}

type definitionErrorBody struct{ closed bool }

func (*definitionErrorBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (b *definitionErrorBody) Close() error           { b.closed = true; return nil }
