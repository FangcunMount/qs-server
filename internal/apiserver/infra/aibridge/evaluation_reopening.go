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

type reviewReopeningReceipt struct {
	SourceVersion        int64             `json:"source_version"`
	Version              int64             `json:"version"`
	TransitionCount      int64             `json:"transition_count"`
	PreviousFinalization json.RawMessage   `json:"previous_finalization"`
	PreviousReviews      []json.RawMessage `json:"previous_reviews"`
	CandidateIDs         []string          `json:"candidate_ids"`
	Actor                string            `json:"actor"`
	Reason               string            `json:"reason"`
	ReopenedAt           string            `json:"reopened_at"`
}

// Verify transport bindings. AI remains responsible for rebuilding every old gate.
func reviewReopenings(response *pb.EvaluationState) (json.RawMessage, error) {
	raw := response.ReopeningsJson
	if raw == "" { // Compatibility with AI versions predating review reopening.
		raw = "[]"
	}
	var history []reviewReopeningReceipt
	if len(raw) > 2*1024*1024 || !utf8.ValidString(raw) || json.Unmarshal([]byte(raw), &history) != nil || history == nil || len(history) > 3 {
		return nil, app.ErrConflict
	}
	var previousVersion, boundary int64
	var previousTime time.Time
	var fingerprint string
	for i, entry := range history {
		if entry.SourceVersion < 1 || entry.Version-1 != entry.SourceVersion || entry.Version > response.Version || entry.SourceVersion <= previousVersion ||
			entry.TransitionCount < 1 || (i > 0 && entry.TransitionCount != boundary+2) || len(entry.PreviousReviews) != 70 || len(entry.CandidateIDs) < 1 || len(entry.CandidateIDs) > 35 ||
			strings.TrimSpace(entry.Reason) == "" || len(entry.Reason) > 1000 || strings.ContainsAny(entry.Reason, "<>") {
			return nil, app.ErrConflict
		}
		actor, err := strconv.ParseInt(strings.TrimPrefix(entry.Actor, "user:"), 10, 64)
		if err != nil || actor < 1 || entry.Actor != fmt.Sprintf("user:%d", actor) {
			return nil, app.ErrConflict
		}
		seen := make(map[string]bool)
		for _, id := range entry.CandidateIDs {
			if strings.TrimSpace(id) == "" || len(id) > 128 || seen[id] {
				return nil, app.ErrConflict
			}
			seen[id] = true
		}
		archived := &pb.EvaluationState{RunId: response.RunId, Version: entry.SourceVersion, Status: "rejected", FinalizationJson: string(entry.PreviousFinalization)}
		if _, err := finalization(archived); err != nil {
			return nil, err
		}
		var final finalizationReceipt
		if json.Unmarshal(entry.PreviousFinalization, &final) != nil {
			return nil, app.ErrConflict
		}
		openedAt, err := time.Parse(time.RFC3339Nano, entry.ReopenedAt)
		closedAt, closeErr := time.Parse(time.RFC3339Nano, final.FinalizedAt)
		if err != nil || closeErr != nil || openedAt.Before(closedAt) || closedAt.Before(previousTime) || (i > 0 && final.ReleaseFingerprint != fingerprint) {
			return nil, app.ErrConflict
		}
		previousVersion, boundary, previousTime, fingerprint = entry.Version, entry.TransitionCount, openedAt, final.ReleaseFingerprint
	}
	if len(history) > 0 {
		if response.UnresolvedResultUnknownCount != 0 || (response.Status != "awaiting_review" && response.Status != "approved" && response.Status != "rejected") {
			return nil, app.ErrConflict
		}
		if response.FinalizationJson != "" {
			var final finalizationReceipt
			if json.Unmarshal([]byte(response.FinalizationJson), &final) != nil || final.ReleaseFingerprint != fingerprint {
				return nil, app.ErrConflict
			}
			at, err := time.Parse(time.RFC3339Nano, final.FinalizedAt)
			if err != nil || at.Before(previousTime) {
				return nil, app.ErrConflict
			}
		}
	}
	return json.RawMessage(raw), nil
}

func (c *EvaluationClient) ReopenEvaluationReview(ctx context.Context, scope app.EvaluationScope, command app.EvaluationReopen) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.ReopenReview(ctx, &pb.EvaluationReopenCommand{Scope: query(scope), ExpectedVersion: command.ExpectedVersion, Reason: command.Reason, Confirm: command.Confirm})
	if err != nil {
		return app.EvaluationState{}, err
	}
	result, err := state(response, scope)
	if err != nil {
		return app.EvaluationState{}, err
	}
	var history []reviewReopeningReceipt
	if result.Status != "awaiting_review" || result.Version-1 != command.ExpectedVersion || json.Unmarshal(result.ReviewReopenings, &history) != nil || len(history) == 0 {
		return app.EvaluationState{}, app.ErrConflict
	}
	last := history[len(history)-1]
	if last.Version != result.Version || last.Actor != fmt.Sprintf("user:%d", scope.OperatorUserID) || last.Reason != command.Reason {
		return app.EvaluationState{}, app.ErrConflict
	}
	return result, nil
}
