package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func executionSummary(raw *pb.EvaluationExecutionSummary) (app.ExecutionSummary, error) {
	if raw == nil || !candidateIdentity.MatchString(raw.ExecutionId) || !candidateIdentity.MatchString(raw.InvocationId) || !candidateIdentity.MatchString(raw.CaseId) ||
		(raw.Kind != "generation" && raw.Kind != "semantic") || raw.SlotOrdinal < 1 || raw.ExecutionOrdinal < 1 ||
		raw.RawOutputBytes < 0 || raw.RawOutputBytes > 256*1024 || raw.NormalizedOutputBytes < 0 || raw.NormalizedOutputBytes > 256*1024 ||
		len(raw.EvidenceJson) > 16384 || !utf8.ValidString(raw.EvidenceJson) || !json.Valid([]byte(raw.EvidenceJson)) {
		return app.ExecutionSummary{}, app.ErrConflict
	}
	switch raw.Status {
	case "prepared", "dispatching", "succeeded", "failed", "result_unknown":
	default:
		return app.ExecutionSummary{}, app.ErrConflict
	}
	var evidence struct {
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(raw.EvidenceJson), &evidence) != nil || evidence.Status != raw.Status {
		return app.ExecutionSummary{}, app.ErrConflict
	}
	return app.ExecutionSummary{ExecutionID: raw.ExecutionId, InvocationID: raw.InvocationId, Kind: raw.Kind, CaseID: raw.CaseId,
		SlotOrdinal: raw.SlotOrdinal, ExecutionOrdinal: raw.ExecutionOrdinal, Status: raw.Status, RawOutputBytes: raw.RawOutputBytes,
		NormalizedOutputBytes: raw.NormalizedOutputBytes, Evidence: json.RawMessage(raw.EvidenceJson)}, nil
}

func (c *EvaluationClient) ListEvaluationExecutions(ctx context.Context, scope app.EvaluationScope, requested app.ExecutionQuery) (app.ExecutionPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.ListExecutions(ctx, &pb.EvaluationExecutionQuery{Scope: query(scope), ExpectedVersion: requested.ExpectedVersion, Cursor: requested.Cursor, Limit: int32(requested.Limit)}, grpc.MaxCallRecvMsgSize(1024*1024))
	if err != nil {
		return app.ExecutionPage{}, err
	}
	if raw == nil || proto.Size(raw) > 1024*1024 || raw.RunId != scope.RunID || raw.Version != requested.ExpectedVersion || len(raw.Executions) > requested.Limit {
		return app.ExecutionPage{}, app.ErrConflict
	}
	page := app.ExecutionPage{RunID: raw.RunId, Version: raw.Version, Executions: make([]app.ExecutionSummary, 0, len(raw.Executions)), NextCursor: raw.NextCursor}
	previous := requested.Cursor
	for _, row := range raw.Executions {
		item, err := executionSummary(row)
		if err != nil || item.ExecutionID <= previous {
			return app.ExecutionPage{}, app.ErrConflict
		}
		page.Executions = append(page.Executions, item)
		previous = item.ExecutionID
	}
	if raw.NextCursor != "" && (len(page.Executions) != requested.Limit || raw.NextCursor != previous) {
		return app.ExecutionPage{}, app.ErrConflict
	}
	return page, nil
}

func (c *EvaluationClient) GetEvaluationExecutionOutput(ctx context.Context, scope app.EvaluationScope, version int64, executionID string) (app.ExecutionOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.GetExecutionOutput(ctx, &pb.EvaluationExecutionQuery{Scope: query(scope), ExpectedVersion: version, ExecutionId: executionID}, grpc.MaxCallRecvMsgSize(1024*1024))
	if err != nil {
		return app.ExecutionOutput{}, err
	}
	if raw == nil || proto.Size(raw) > 1024*1024 || raw.RunId != scope.RunID || raw.Version != version {
		return app.ExecutionOutput{}, app.ErrConflict
	}
	item, err := executionSummary(raw.Execution)
	if err != nil || item.ExecutionID != executionID || int64(len(raw.RawOutput)) != item.RawOutputBytes || int64(len(raw.NormalizedOutput)) != item.NormalizedOutputBytes {
		return app.ExecutionOutput{}, app.ErrConflict
	}
	fingerprint := func(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
	if raw.RawSha256 != fingerprint(raw.RawOutput) || raw.NormalizedSha256 != fingerprint(raw.NormalizedOutput) {
		return app.ExecutionOutput{}, app.ErrConflict
	}
	return app.ExecutionOutput{RunID: raw.RunId, Version: raw.Version, Execution: item,
		RawOutput: append([]byte{}, raw.RawOutput...), NormalizedOutput: append([]byte{}, raw.NormalizedOutput...), RawSHA256: raw.RawSha256, NormalizedSHA256: raw.NormalizedSha256}, nil
}
