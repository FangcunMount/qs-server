package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

func (s *evaluationRPCStub) ReopenReview(ctx context.Context, command *pb.EvaluationReopenCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.calls++
	s.reopenCommand = command
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("bounded reopening timeout required")
	}
	return s.reopenState, s.fail
}

func reopenedState() *pb.EvaluationState {
	old := finalState()
	var reviews []json.RawMessage
	_ = json.Unmarshal([]byte(old.ReviewsJson), &reviews)
	entry := reviewReopeningReceipt{SourceVersion: 8, Version: 9, TransitionCount: 3, PreviousFinalization: json.RawMessage(old.FinalizationJson), PreviousReviews: reviews, CandidateIDs: []string{"candidate:1"}, Actor: "user:42", Reason: "复核", ReopenedAt: "2026-09-13T01:00:01Z"}
	raw, _ := json.Marshal([]reviewReopeningReceipt{entry})
	return &pb.EvaluationState{RunId: old.RunId, Version: 9, Status: "awaiting_review", ReviewsJson: "[]", ResolutionsJson: "[]", ReopeningsJson: string(raw)}
}

func TestReopenForwardsOnceAndBindsAcceptedRound(t *testing.T) {
	rpc := &evaluationRPCStub{t: t, reopenState: reopenedState()}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	command := app.EvaluationReopen{ExpectedVersion: 8, Reason: "复核", Confirm: true}
	result, err := client.ReopenEvaluationReview(context.Background(), scope, command)
	if err != nil || result.Version != 9 || len(result.ReviewReopenings) == 0 || rpc.calls != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	if rpc.reopenCommand.Scope.OperatorUserId != 42 || rpc.reopenCommand.ExpectedVersion != 8 || !rpc.reopenCommand.Confirm {
		t.Fatal("trusted command drift")
	}
	rpc.fail = context.DeadlineExceeded
	if _, err := client.ReopenEvaluationReview(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal("uncertain write retried", err)
	}
	rpc.fail = nil
	for _, mutate := range []func(){
		func() { scope.OperatorUserID = 43 },
		func() { command.Reason = "另一个理由" },
		func() { command.ExpectedVersion-- },
		func() { rpc.reopenState.ReopeningsJson = "[]" },
	} {
		scope.OperatorUserID = 42
		command = app.EvaluationReopen{ExpectedVersion: 8, Reason: "复核", Confirm: true}
		mutate()
		if _, err := client.ReopenEvaluationReview(context.Background(), scope, command); !errors.Is(err, app.ErrConflict) {
			t.Fatal("mismatched reopening accepted", err)
		}
	}
}

func TestEveryStateReadChecksBoundedHistoricalReceipts(t *testing.T) {
	for _, mutate := range []func(*pb.EvaluationState){
		func(r *pb.EvaluationState) { r.ReopeningsJson = "" },
		func(r *pb.EvaluationState) { r.ReopeningsJson = "null" },
		func(r *pb.EvaluationState) { r.ReopeningsJson = strings.Repeat("x", 2*1024*1024+1) },
		func(r *pb.EvaluationState) { r.Version = 8 },
		func(r *pb.EvaluationState) {
			r.ReopeningsJson = strings.ReplaceAll(r.ReopeningsJson, `"source_version":8`, `"source_version":7`)
		},
		func(r *pb.EvaluationState) {
			r.ReopeningsJson = strings.ReplaceAll(r.ReopeningsJson, `"reopened_at":"2026-09-13T01:00:01Z"`, `"reopened_at":"2026-09-13T00:00:01Z"`)
		},
		func(r *pb.EvaluationState) {
			r.ReopeningsJson = strings.ReplaceAll(r.ReopeningsJson, `"candidate:1"`, `"candidate:1","candidate:1"`)
		},
	} {
		response := reopenedState()
		mutate(response)
		if _, err := state(response, app.EvaluationScope{RunID: "run:1"}); !errors.Is(err, app.ErrConflict) {
			t.Fatal("invalid history accepted", err)
		}
	}
	old := &pb.EvaluationState{ReviewsJson: "[]", ReopeningsJson: "[]", RunId: "run:1", Version: 1, Status: "requested", ResolutionsJson: "[]"}
	if result, err := state(old, app.EvaluationScope{RunID: "run:1"}); err != nil || string(result.ReviewReopenings) != "[]" {
		t.Fatal("explicit empty reopening history rejected", err)
	}
}

func TestReviewHistoryDoesNotReimplementAIPolicy(t *testing.T) {
	// These snapshots model a policy change in the authoritative AI service.
	// The proxy preserves the evidence without prescribing its review/gate counts.
	for _, change := range []struct {
		name  string
		apply func(*reviewReopeningReceipt)
	}{
		{"review count", func(r *reviewReopeningReceipt) { r.PreviousReviews = r.PreviousReviews[:69] }},
		{"candidate count", func(r *reviewReopeningReceipt) {
			for i := 2; i <= 36; i++ {
				r.CandidateIDs = append(r.CandidateIDs, fmt.Sprintf("candidate:%d", i))
			}
		}},
		{"gate policy", func(r *reviewReopeningReceipt) {
			var final map[string]any
			_ = json.Unmarshal(r.PreviousFinalization, &final)
			final["gate_result"] = map[string]any{"policy": "future-policy"}
			r.PreviousFinalization, _ = json.Marshal(final)
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			response := reopenedState()
			var history []reviewReopeningReceipt
			_ = json.Unmarshal([]byte(response.ReopeningsJson), &history)
			change.apply(&history[0])
			raw, _ := json.Marshal(history)
			response.ReopeningsJson = string(raw)
			result, err := state(response, app.EvaluationScope{RunID: "run:1"})
			if err != nil || string(result.ReviewReopenings) != string(raw) {
				t.Fatal("QS reinterpreted AI review policy", err)
			}
		})
	}
}

func TestStatePreservesReopeningEligibilityPresence(t *testing.T) {
	yes, no := true, false
	for _, allowed := range []*bool{nil, &no, &yes} {
		response := reopenedState()
		response.CanReopenReview = allowed
		result, err := state(response, app.EvaluationScope{RunID: "run:1"})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		value, present := wire["can_reopen_review"]
		if present != (allowed != nil) || (allowed != nil && value != *allowed) {
			t.Fatalf("eligibility presence/value lost: %s", raw)
		}
	}
}

func TestReviewHistoryRoundLimitBelongsToAI(t *testing.T) {
	response := reopenedState()
	var original []reviewReopeningReceipt
	_ = json.Unmarshal([]byte(response.ReopeningsJson), &original)
	history := make([]reviewReopeningReceipt, 4)
	for i := range history {
		entry := original[0]
		entry.SourceVersion += int64(i * 3)
		entry.Version = entry.SourceVersion + 1
		entry.TransitionCount += int64(i * 3)
		var final finalizationReceipt
		_ = json.Unmarshal(entry.PreviousFinalization, &final)
		final.Version, final.SourceVersion = entry.SourceVersion, entry.SourceVersion-1
		final.FinalizedAt = time.Date(2026, 9, 13, i+1, 0, 0, 0, time.UTC).Format(time.RFC3339)
		entry.ReopenedAt = time.Date(2026, 9, 13, i+1, 0, 1, 0, time.UTC).Format(time.RFC3339)
		entry.PreviousFinalization, _ = json.Marshal(final)
		history[i] = entry
	}
	raw, _ := json.Marshal(history)
	response.Version, response.ReopeningsJson = history[3].Version, string(raw)
	if _, err := state(response, app.EvaluationScope{RunID: response.RunId}); err != nil {
		t.Fatal("proxy imposed its own round/transition limit", err)
	}
	// Moving ownership must not accept an archive belonging to another Run.
	history[0].PreviousFinalization = json.RawMessage(strings.ReplaceAll(string(history[0].PreviousFinalization), `"run:1"`, `"run:other"`))
	raw, _ = json.Marshal(history)
	response.ReopeningsJson = string(raw)
	if _, err := state(response, app.EvaluationScope{RunID: response.RunId}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("archive identity mismatch accepted", err)
	}
}
