package subsystem

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/ratelimit"
)

const maxRateOverrideTTL = 24 * time.Hour

func (s *Subsystem) TuneRateLimit(ctx context.Context, actor control.ActionActor, change control.RateLimitChange) (control.RateLimitChangeResult, error) {
	result := control.RateLimitChangeResult{Status: control.CommandStatusOK, Component: change.Component, Budget: change.Budget}
	if s == nil || s.stateStore == nil {
		return result, control.ErrUnavailable
	}
	if change.Component != "apiserver" && change.Component != "collection-server" {
		return result, invalidArgument("unsupported rate limit component %q", change.Component)
	}
	if change.Component == "collection-server" && !validCollectionBudget(change.Budget) {
		return result, invalidArgument("unknown collection-server rate limit budget %q", change.Budget)
	}
	if change.ExpectedVersion == 0 {
		return result, invalidArgument("expected_version must be positive")
	}
	var local *ratelimit.Budget
	if change.Component == "apiserver" {
		var ok bool
		local, ok = s.RateBudget(ratelimit.BudgetID(change.Budget))
		if !ok {
			return result, invalidArgument("unknown apiserver rate limit budget %q", change.Budget)
		}
		if current := local.Snapshot().Version; current != change.ExpectedVersion {
			return result, control.ErrVersionConflict
		}
	}
	name := rateStateName(change.Component, change.Budget)
	if change.Mode == "reset" {
		state, exists, err := s.stateStore.Load(ctx, name)
		if err != nil {
			return result, err
		}
		if exists {
			if state.Version != change.ExpectedVersion {
				return result, control.ErrVersionConflict
			}
			payload, _ := json.Marshal(control.RateLimitChange{Mode: "reset", Component: change.Component, Budget: change.Budget})
			published, err := s.stateStore.CompareAndSwap(ctx, name, change.ExpectedVersion, control.VersionedState{Payload: payload, Actor: actor}, 0)
			if err != nil {
				return result, err
			}
			result.Version = published.Version
		}
		if !exists {
			if local == nil || local.Snapshot().Source == "config" {
				result.Status, result.Version, result.Source = control.CommandStatusNoop, change.ExpectedVersion, "config"
				return result, nil
			}
		}
		if local != nil {
			snapshot, err := local.ReconcileBaseline(result.Version)
			if err != nil {
				return result, err
			}
			result.Version, result.Source = snapshot.Version, snapshot.Source
		} else {
			result.Source = "config"
		}
		return result, nil
	}
	if change.Mode != "override" || !change.Global.Valid() || !change.User.Valid() {
		return result, invalidArgument("override requires valid global and user policies")
	}
	ttl := time.Duration(change.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if ttl > maxRateOverrideTTL {
		return result, invalidArgument("ttl_seconds exceeds %s", maxRateOverrideTTL)
	}
	if err := s.ensureRateBaseline(ctx, name, change.ExpectedVersion, actor); err != nil {
		return result, err
	}
	payload, err := json.Marshal(change)
	if err != nil {
		return result, err
	}
	published, err := s.stateStore.CompareAndSwap(ctx, name, change.ExpectedVersion, control.VersionedState{Payload: payload, Actor: actor}, ttl)
	if err != nil {
		return result, err
	}
	if local != nil {
		snapshot, applyErr := local.Apply(change.ExpectedVersion, ratelimit.BudgetPolicy{
			Global: overridePolicy(local.Snapshot().Policy.Global, change.Global),
			User:   overridePolicy(local.Snapshot().Policy.User, change.User),
		}, "governance", ttl)
		if applyErr != nil {
			return result, applyErr
		}
		result.Version, result.Source, result.ExpiresAt = snapshot.Version, snapshot.Source, snapshot.ExpiresAt
	} else {
		result.Version, result.Source, result.ExpiresAt = published.Version, "governance", published.ExpiresAt
	}
	return result, nil
}

func (s *Subsystem) ensureRateBaseline(ctx context.Context, name string, version uint64, actor control.ActionActor) error {
	state, exists, err := s.stateStore.Load(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		if state.Version != version {
			return control.ErrVersionConflict
		}
		return nil
	}
	_, err = s.stateStore.CompareAndSwap(ctx, name, 0, control.VersionedState{Version: version, Payload: []byte(`{"mode":"config"}`), Actor: actor}, 0)
	return err
}

func validCollectionBudget(budget string) bool {
	switch ratelimit.BudgetID(budget) {
	case "query", "submit", "wait_report", "report_events":
		return true
	default:
		return false
	}
}

func invalidArgument(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", control.ErrInvalidArgument, fmt.Sprintf(format, args...))
}

func overridePolicy(current ratelimit.RateLimitPolicy, change control.RatePolicy) ratelimit.RateLimitPolicy {
	current.RatePerSecond = change.RatePerSecond
	current.Burst = change.Burst
	return current
}

func rateStateName(component, budget string) string { return "rate:" + component + ":" + budget }

func queueStateName(component, queue string) string { return "queue:" + component + ":" + queue }

func leaderStateName(component, instanceID, workload string) string {
	return "leader:" + component + ":" + instanceID + ":" + workload
}

func (s *Subsystem) SetQueueState(ctx context.Context, actor control.ActionActor, change control.QueueChange) (control.QueueChangeResult, error) {
	result := control.QueueChangeResult{Status: control.CommandStatusOK, Component: change.Component, Queue: change.Queue, State: change.DesiredState}
	if s == nil || s.stateStore == nil {
		return result, control.ErrUnavailable
	}
	commands, ok := s.stateStore.(control.CommandStore)
	if !ok || change.RequestID == "" {
		return result, control.ErrUnavailable
	}
	if change.Component != "collection-server" || change.Queue != "answersheet_submit" {
		return result, invalidArgument("unsupported queue target %s/%s", change.Component, change.Queue)
	}
	if change.Target == "" {
		change.Target = "all"
	}
	if change.DesiredState != control.QueueStatePaused && change.DesiredState != control.QueueStateActive {
		return result, invalidArgument("queue desired state must be paused or active")
	}
	name := queueStateName(change.Component, change.Queue)
	current, exists, err := s.stateStore.Load(ctx, name)
	if err != nil {
		return result, err
	}
	expected := uint64(0)
	if exists {
		expected = current.Version
	}
	expectedInstances, err := commandTargetInstances(ctx, commands, change.Component, change.Target)
	if err != nil {
		return result, err
	}
	if exists && queueChangeMatches(current.Payload, change) {
		result.Status, result.Version = control.CommandStatusNoop, current.Version
		return result, nil
	}
	change.StateVersion = expected + 1
	payload, err := json.Marshal(change)
	if err != nil {
		return result, err
	}
	command := control.Command{
		RequestID: change.RequestID, ActionID: queueActionID(change.DesiredState),
		Target:  control.Target{Component: change.Component, InstanceID: change.Target},
		Payload: payload, Actor: actor, IssuedAt: time.Now(),
	}
	timeout := time.Duration(change.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Minute
	}
	command.ExpiresAt = time.Now().Add(timeout + time.Minute)
	if len(expectedInstances) == 0 {
		published, publishErr := s.stateStore.CompareAndSwap(ctx, name, expected, control.VersionedState{Payload: payload, Actor: actor}, 0)
		if publishErr != nil {
			resilience.ObserveControlOperation("apiserver", "queue_state_commit", "failed")
			return result, publishErr
		}
		resilience.ObserveControlOperation("apiserver", "queue_state_commit", "ok")
		result.Version = published.Version
		result.Status = control.CommandStatusNoop
		return result, nil
	}
	if err := commands.PublishCommand(ctx, command, timeout+time.Minute); err != nil {
		resilience.ObserveControlOperation("apiserver", "queue_command_publish", "failed")
		return result, err
	}
	resilience.ObserveControlOperation("apiserver", "queue_command_publish", "ok")
	published, err := s.stateStore.CompareAndSwap(ctx, name, expected, control.VersionedState{Payload: payload, Actor: actor}, 0)
	if err != nil {
		resilience.ObserveControlOperation("apiserver", "queue_state_commit", "failed")
		return result, err
	}
	resilience.ObserveControlOperation("apiserver", "queue_state_commit", "ok")
	result.Version = published.Version
	results, status, err := waitCommandResults(ctx, commands, actor.OrgID, change.RequestID, expectedInstances, timeout)
	result.Instances, result.Status = results, status
	if change.DesiredState == control.QueueStateActive && status != control.CommandStatusOK && status != control.CommandStatusNoop {
		rollback := change
		rollback.DesiredState = control.QueueStatePaused
		rollback.StateVersion = published.Version + 1
		rollbackPayload, _ := json.Marshal(rollback)
		if restored, restoreErr := s.stateStore.CompareAndSwap(ctx, name, published.Version, control.VersionedState{Payload: rollbackPayload, Actor: actor}, 0); restoreErr == nil {
			result.Version = restored.Version
		}
	}
	return result, err
}

func queueChangeMatches(payload []byte, expected control.QueueChange) bool {
	var current control.QueueChange
	if json.Unmarshal(payload, &current) != nil {
		return false
	}
	currentTarget, expectedTarget := current.Target, expected.Target
	if currentTarget == "" {
		currentTarget = "all"
	}
	if expectedTarget == "" {
		expectedTarget = "all"
	}
	return current.Component == expected.Component && current.Queue == expected.Queue &&
		currentTarget == expectedTarget && current.DesiredState == expected.DesiredState
}

func queueActionID(state control.QueueState) string {
	if state == control.QueueStateActive {
		return "resilience.resume_queue"
	}
	return "resilience.drain_queue"
}

func (s *Subsystem) RelinquishLeader(ctx context.Context, actor control.ActionActor, change control.LeaderChange) (any, error) {
	if s == nil || s.stateStore == nil || change.RequestID == "" {
		return nil, control.ErrUnavailable
	}
	commands, ok := s.stateStore.(control.CommandStore)
	if !ok {
		return nil, control.ErrUnavailable
	}
	if change.Component != "apiserver" || change.InstanceID == "" {
		return nil, invalidArgument("release_lock requires component=apiserver and a target instance_id")
	}
	workload := locklease.WorkloadID(change.Workload)
	capability, ok := locklease.Lookup(workload)
	if !ok || capability.Component != "apiserver" || capability.Kind != locklease.KindLeader {
		return nil, invalidArgument("workload %q is not a releasable apiserver leader", change.Workload)
	}
	cooldown := time.Duration(change.CooldownSeconds) * time.Second
	if cooldown <= 0 {
		cooldown = capability.Spec.DefaultTTL
	}
	payload, err := json.Marshal(change)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(change.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	instances, err := commandTargetInstances(ctx, commands, change.Component, change.InstanceID)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return map[string]interface{}{"status": control.CommandStatusNoop, "instances": []control.CommandResult{}}, nil
	}
	command := control.Command{RequestID: change.RequestID, ActionID: "resilience.release_lock",
		Target: control.Target{Component: change.Component, InstanceID: change.InstanceID}, Payload: payload,
		Actor: actor, IssuedAt: time.Now(), ExpiresAt: time.Now().Add(timeout + time.Minute)}
	if err := commands.PublishCommand(ctx, command, timeout+time.Minute); err != nil {
		return nil, err
	}
	results, status, waitErr := waitCommandResults(ctx, commands, actor.OrgID, change.RequestID, instances, timeout)
	return map[string]interface{}{"status": status, "instances": results, "cooldown_seconds": int(cooldown.Seconds())}, waitErr
}

func commandTargetInstances(ctx context.Context, store control.CommandStore, component, target string) ([]string, error) {
	instances, err := store.ListInstances(ctx, component)
	if err != nil {
		return nil, err
	}
	result := []string{}
	seen := make(map[string]struct{})
	for _, identity := range instances {
		if target == "" || target == "all" || target == identity.InstanceID {
			if _, exists := seen[identity.InstanceID]; exists {
				continue
			}
			seen[identity.InstanceID] = struct{}{}
			result = append(result, identity.InstanceID)
		}
	}
	return result, nil
}

func waitCommandResults(ctx context.Context, store control.CommandStore, orgID int64, requestID string, expected []string, timeout time.Duration) ([]control.CommandResult, control.CommandStatus, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		results, err := store.ListCommandResults(waitCtx, orgID, requestID)
		if err != nil {
			return nil, control.CommandStatusFailed, err
		}
		if len(results) >= len(expected) {
			status := control.CommandStatusOK
			allNoop, allTimeout := len(results) > 0, len(results) > 0
			failed, succeeded := false, false
			for _, result := range results {
				allNoop = allNoop && result.Status == control.CommandStatusNoop
				allTimeout = allTimeout && result.Status == control.CommandStatusTimeout
				failed = failed || result.Status == control.CommandStatusFailed || result.Status == control.CommandStatusTimeout
				succeeded = succeeded || result.Status == control.CommandStatusOK || result.Status == control.CommandStatusNoop
			}
			switch {
			case allNoop:
				status = control.CommandStatusNoop
			case allTimeout:
				status = control.CommandStatusTimeout
			case failed && succeeded:
				status = control.CommandStatusPartial
			case failed:
				status = control.CommandStatusFailed
			}
			return results, status, nil
		}
		select {
		case <-waitCtx.Done():
			if len(results) > 0 {
				return results, control.CommandStatusPartial, nil
			}
			return results, control.CommandStatusTimeout, nil
		case <-ticker.C:
		}
	}
}

var _ control.Governor = (*Subsystem)(nil)
