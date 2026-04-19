package chain

import (
	"context"
	"testing"
)

func TestRunStopsOnHandlerDecision(t *testing.T) {
	type state struct {
		visited []string
	}

	current := &state{}
	decision, err := Run(context.Background(), "test_chain", current,
		FuncHandler[state]{
			HandlerName: "first",
			HandlerFunc: func(ctx context.Context, state *state) (Decision, error) {
				state.visited = append(state.visited, "first")
				return Next(), nil
			},
		},
		FuncHandler[state]{
			HandlerName: "second",
			HandlerFunc: func(ctx context.Context, state *state) (Decision, error) {
				state.visited = append(state.visited, "second")
				return Stop("stop_here"), nil
			},
		},
		FuncHandler[state]{
			HandlerName: "third",
			HandlerFunc: func(ctx context.Context, state *state) (Decision, error) {
				state.visited = append(state.visited, "third")
				return Next(), nil
			},
		},
	)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if decision.Continue {
		t.Fatal("expected chain to stop")
	}
	if decision.StopReason != "stop_here" {
		t.Fatalf("unexpected stop reason: %q", decision.StopReason)
	}
	if len(current.visited) != 2 {
		t.Fatalf("expected exactly 2 handlers to run, got %v", current.visited)
	}
}
