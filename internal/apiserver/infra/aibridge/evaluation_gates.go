package aibridge

import (
	"context"
	"encoding/json"
	"regexp"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

var gateReleaseFingerprint = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func (c *EvaluationClient) PreviewEvaluationGates(ctx context.Context, scope app.EvaluationScope, expectedVersion int64) (app.EvaluationGatePreview, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.PreviewGates(ctx, &pb.EvaluationGateQuery{Scope: query(scope), ExpectedVersion: expectedVersion})
	if err != nil {
		return app.EvaluationGatePreview{}, err
	}
	if response == nil || response.RunId != scope.RunID || response.Version != expectedVersion ||
		!gateReleaseFingerprint.MatchString(response.ReleaseFingerprint) || len(response.GateResultJson) > 256*1024 || !utf8.ValidString(response.GateResultJson) {
		return app.EvaluationGatePreview{}, app.ErrConflict
	}
	// Validate the transport contract only. QS never recalculates AI release policy.
	var result struct {
		SchemaVersion string            `json:"schema_version"`
		EvaluatedAt   string            `json:"evaluated_at"`
		GatePasses    map[string]*bool  `json:"gate_passes"`
		Metrics       []json.RawMessage `json:"metrics"`
		Reasons       []json.RawMessage `json:"reasons"`
		Adjudications []json.RawMessage `json:"semantic_adjudications"`
	}
	if json.Unmarshal([]byte(response.GateResultJson), &result) != nil || result.SchemaVersion != "qs-ai-evaluation-gate-preview/v1" ||
		len(result.GatePasses) != 5 || result.Metrics == nil || result.Reasons == nil || result.Adjudications == nil {
		return app.EvaluationGatePreview{}, app.ErrConflict
	}
	if _, err := time.Parse(time.RFC3339Nano, result.EvaluatedAt); err != nil {
		return app.EvaluationGatePreview{}, app.ErrConflict
	}
	for _, gate := range []string{"G1", "G2", "G3", "G4", "G5"} {
		if result.GatePasses[gate] == nil {
			return app.EvaluationGatePreview{}, app.ErrConflict
		}
	}
	return app.EvaluationGatePreview{RunID: response.RunId, Version: response.Version, ReleaseFingerprint: response.ReleaseFingerprint, GateResult: json.RawMessage(response.GateResultJson)}, nil
}
