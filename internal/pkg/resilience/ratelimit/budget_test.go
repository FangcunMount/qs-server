package ratelimit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBudgetPublishesVersionedPairAndResets(t *testing.T) {
	now := time.Unix(100, 0)
	baseline := BudgetPolicy{Global: testPolicy("local"), User: testPolicy("local_key")}
	budget := NewBudget("query", baseline, BudgetOptions{Now: func() time.Time { return now }})
	next := baseline
	next.Global.RatePerSecond = 2
	next.User.RatePerSecond = 3
	published, err := budget.Apply(1, next, "governance", time.Minute)
	if err != nil || published.Version != 2 || published.Policy.Global.RatePerSecond != 2 || published.Policy.User.RatePerSecond != 3 {
		t.Fatalf("Apply() = %+v, %v", published, err)
	}
	if _, err := budget.Apply(1, next, "stale", time.Minute); !errors.Is(err, ErrBudgetVersionConflict) {
		t.Fatalf("stale Apply() error = %v", err)
	}
	reset, err := budget.Reset(2)
	if err != nil || reset.Source != "config" || reset.Version != 3 || reset.Policy.Global.RatePerSecond != baseline.Global.RatePerSecond {
		t.Fatalf("Reset() = %+v, %v", reset, err)
	}
}

func TestBudgetOverrideExpiresToBaseline(t *testing.T) {
	now := time.Unix(100, 0)
	baseline := BudgetPolicy{Global: testPolicy("local"), User: testPolicy("local_key")}
	budget := NewBudget("query", baseline, BudgetOptions{Now: func() time.Time { return now }})
	override := baseline
	override.Global.Burst = 4
	if _, err := budget.Apply(1, override, "governance", time.Second); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	snapshot := budget.Snapshot()
	if snapshot.Version != 3 || snapshot.Source != "config" || snapshot.Policy.Global.Burst != baseline.Global.Burst {
		t.Fatalf("expired snapshot = %+v", snapshot)
	}
}

func TestBudgetConcurrentDecisionsDuringPublish(t *testing.T) {
	baseline := BudgetPolicy{Global: testPolicy("local"), User: testPolicy("local_key")}
	budget := NewBudget("query", baseline, BudgetOptions{})
	limiters := budget.Limiters()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = limiters.Global.Decide(context.Background(), "global")
				_ = limiters.User.Decide(context.Background(), "user")
			}
		}()
	}
	next := baseline
	next.Global.Burst = 20
	next.User.Burst = 20
	if _, err := budget.Apply(1, next, "governance", time.Minute); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}
