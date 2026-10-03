package handler

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/aicommand"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMQPublicMaintenanceIsDefinitiveWithoutCapacityReset(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		err     error
		code    int
		message string
	}{
		{"maintenance", status.Error(codes.ResourceExhausted, aicommand.AdmissionClosedReason), 429, aicommand.AdmissionClosedReason},
		{"storage_unknown", status.Error(codes.Unavailable, "secret storage detail"), 503, "temporarily unavailable"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			w, c := newAIExplanationTestContext("POST", "/", "{}")
			NewAIExplanationHandler(nil).respondError(c, scenario.err)
			if w.Code != scenario.code || !strings.Contains(w.Body.String(), scenario.message) || strings.Contains(w.Body.String(), "secret") || w.Header().Get("Retry-After") != "" {
				t.Fatal("admission refusal/storage uncertainty changed", w.Code, w.Body.String(), w.Header())
			}
		})
	}
}
