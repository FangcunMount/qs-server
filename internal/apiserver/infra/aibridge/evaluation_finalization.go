package aibridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

type finalizationReceipt struct {
	SchemaVersion      string `json:"schema_version"`
	RunID              string `json:"run_id"`
	SourceVersion      int64  `json:"source_version"`
	Version            int64  `json:"version"`
	ReleaseFingerprint string `json:"release_fingerprint"`
	Actor              string `json:"actor"`
	Reason             string `json:"reason"`
	FinalizedAt        string `json:"finalized_at"`
	Passed             *bool  `json:"passed"`
	Status             string `json:"status"`
	GateResult         struct {
		EvaluatedAt   string            `json:"evaluated_at"`
		GatePasses    map[string]*bool  `json:"gate_passes"`
		Metrics       []json.RawMessage `json:"metrics"`
		Reasons       []json.RawMessage `json:"reasons"`
		Adjudications []json.RawMessage `json:"semantic_adjudications"`
	} `json:"gate_result"`
}

// QS validates response bindings only; AI owns recomputation of the quality policy.
func finalization(response *pb.EvaluationState) (json.RawMessage, error) {
	terminal := response.Status == "approved" || response.Status == "rejected"
	if response.FinalizationJson == "" && !terminal {
		return nil, nil
	}
	if !terminal || len(response.FinalizationJson) > 256*1024 || !utf8.ValidString(response.FinalizationJson) {
		return nil, app.ErrConflict
	}
	var receipt finalizationReceipt
	if json.Unmarshal([]byte(response.FinalizationJson), &receipt) != nil ||
		receipt.SchemaVersion != "qs-ai-evaluation-finalization/v1" || receipt.RunID != response.RunId || receipt.Version != response.Version ||
		receipt.SourceVersion < 1 || receipt.SourceVersion != response.Version-1 || receipt.Passed == nil || receipt.Status != response.Status ||
		!gateReleaseFingerprint.MatchString(receipt.ReleaseFingerprint) || !strings.HasPrefix(receipt.Actor, "user:") ||
		strings.TrimSpace(receipt.Reason) == "" || len(receipt.Reason) > 1000 || response.UnresolvedResultUnknownCount != 0 {
		return nil, app.ErrConflict
	}
	if (*receipt.Passed) != (response.Status == "approved") || receipt.GateResult.EvaluatedAt != receipt.FinalizedAt {
		return nil, app.ErrConflict
	}
	actorID, err := strconv.ParseInt(strings.TrimPrefix(receipt.Actor, "user:"), 10, 64)
	if err != nil || actorID < 1 || receipt.Actor != fmt.Sprintf("user:%d", actorID) || strings.ContainsAny(receipt.Reason, "<>") {
		return nil, app.ErrConflict
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.FinalizedAt); err != nil {
		return nil, app.ErrConflict
	}
	gates := receipt.GateResult
	if len(gates.GatePasses) != 5 || gates.Metrics == nil || gates.Reasons == nil || gates.Adjudications == nil {
		return nil, app.ErrConflict
	}
	passed := true
	for _, gate := range []string{"G1", "G2", "G3", "G4", "G5"} {
		if gates.GatePasses[gate] == nil {
			return nil, app.ErrConflict
		}
		passed = passed && *gates.GatePasses[gate]
	}
	if passed != *receipt.Passed {
		return nil, app.ErrConflict
	}
	return json.RawMessage(response.FinalizationJson), nil
}

func (c *EvaluationClient) FinalizeEvaluation(ctx context.Context, scope app.EvaluationScope, command app.EvaluationFinalize) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Finalize(ctx, &pb.EvaluationFinalizeCommand{
		Scope: query(scope), ExpectedVersion: command.ExpectedVersion, ExpectedPassed: command.ExpectedPassed,
		Reason: command.Reason, Confirm: command.Confirm,
	})
	if err != nil {
		return app.EvaluationState{}, err
	}
	result, err := state(response, scope)
	if err != nil {
		return app.EvaluationState{}, err
	}
	var receipt finalizationReceipt
	if json.Unmarshal(result.Finalization, &receipt) != nil || command.ExpectedPassed == nil || receipt.Passed == nil ||
		*receipt.Passed != *command.ExpectedPassed || receipt.SourceVersion != command.ExpectedVersion ||
		receipt.Actor != fmt.Sprintf("user:%d", scope.OperatorUserID) || receipt.Reason != command.Reason {
		return app.EvaluationState{}, app.ErrConflict
	}
	return result, nil
}
