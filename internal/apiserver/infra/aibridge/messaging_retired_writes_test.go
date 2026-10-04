package aibridge

import (
	"reflect"
	"testing"
)

// A missing MQ dependency must never make a reusable gateway expose the retired
// runtime RPC path. Query and governance methods remain on these clients.
func TestMQRuntimeGatewaysCannotExposeRetiredGRPCWrites(t *testing.T) {
	for _, candidate := range []struct {
		client any
		method string
	}{
		{(*EvaluationClient)(nil), "StartEvaluation"},
		{(*EvaluationClient)(nil), "CancelEvaluation"},
		{(*ParticipantClient)(nil), "RetryParticipant"},
	} {
		t.Run(candidate.method, func(t *testing.T) {
			if _, exists := reflect.TypeOf(candidate.client).MethodByName(candidate.method); exists {
				t.Fatalf("retired runtime gRPC write still exposed: %s", candidate.method)
			}
		})
	}
}
