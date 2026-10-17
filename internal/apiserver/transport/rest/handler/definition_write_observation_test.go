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
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type observingDefinitionService struct {
	assessmentModelDefinitionStub
	called bool
}

func (s *observingDefinitionService) SaveDefinition(_ context.Context, _ modelcatalog.ActorContext, _ string, value *domain.Definition) (*domain.Definition, error) {
	s.called = true
	modeldefinition.MaterializeLayers(value)
	return value, s.saveErr
}

func TestDefinitionObservationKeepsOriginalFieldPresenceAndBinding(t *testing.T) {
	for _, tc := range []struct {
		name, body, input, decision, assets, result string
		status                                      int
	}{
		{"canonical", "{}", "canonical", "absent", "absent", "accepted", 200},
		{"both null", `{"DecisionSpec":null,"InterpretationAssets":null}`, "derived", "present", "present", "accepted", 200},
		{"empty decision", `{"DecisionSpec":{}}`, "derived", "present", "absent", "accepted", 200},
		{"empty assets", `{"InterpretationAssets":{}}`, "derived", "absent", "present", "accepted", 200},
		{"case folded keys", `{"decisionspec":null,"INTERPRETATIONASSETS":{}}`, "derived", "present", "present", "accepted", 200},
		{"nested fields are not input", `{"unknown":{"DecisionSpec":{}}}`, "canonical", "absent", "absent", "accepted", 200},
		{"trailing bytes preserve binder contract", `{} trailing`, "canonical", "absent", "absent", "accepted", 200},
		{"invalid decision", `{"DecisionSpec":42}`, "derived", "present", "absent", "rejected", 500},
		{"malformed", `{`, "unreadable", "unreadable", "unreadable", "rejected", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			totalBefore := testutil.ToFloat64(definitionWriteTotal.WithLabelValues(tc.input, tc.result))
			decisionBefore := testutil.ToFloat64(definitionInputFieldTotal.WithLabelValues("DecisionSpec", tc.decision, tc.result))
			assetsBefore := testutil.ToFloat64(definitionInputFieldTotal.WithLabelValues("InterpretationAssets", tc.assets, tc.result))
			svc := &observingDefinitionService{}
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
			if svc.called != (tc.result == "accepted") {
				t.Fatalf("service called=%t", svc.called)
			}
			if c.Request.Body != original {
				t.Fatal("body ownership changed")
			}
			if delta := testutil.ToFloat64(definitionWriteTotal.WithLabelValues(tc.input, tc.result)) - totalBefore; delta != 1 {
				t.Fatalf("total delta=%g", delta)
			}
			if delta := testutil.ToFloat64(definitionInputFieldTotal.WithLabelValues("DecisionSpec", tc.decision, tc.result)) - decisionBefore; delta != 1 {
				t.Fatalf("decision delta=%g", delta)
			}
			if delta := testutil.ToFloat64(definitionInputFieldTotal.WithLabelValues("InterpretationAssets", tc.assets, tc.result)) - assetsBefore; delta != 1 {
				t.Fatalf("assets delta=%g", delta)
			}
		})
	}
}

func TestDefinitionObservationPreservesReadError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/definition", nil)
	original := &definitionErrorBody{}
	c.Request.Body = original
	setAssessmentModelActor(c)
	NewAssessmentModelHandler(nil, &observingDefinitionService{}, nil).UpdateDefinition(c)
	if recorder.Code != 500 || c.Request.Body != original || original.closed {
		t.Fatalf("status=%d closed=%t", recorder.Code, original.closed)
	}
}

type definitionErrorBody struct{ closed bool }

func (*definitionErrorBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (b *definitionErrorBody) Close() error           { b.closed = true; return nil }
