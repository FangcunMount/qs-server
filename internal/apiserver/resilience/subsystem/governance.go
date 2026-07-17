package subsystem

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
)

const maxRateOverrideTTL = 24 * time.Hour

func (s *Subsystem) TuneRateLimit(ctx context.Context, actor resiliencecontrol.ActionActor, change resiliencecontrol.RateLimitChange) (resiliencecontrol.RateLimitChangeResult, error) {
	result := resiliencecontrol.RateLimitChangeResult{Status: resiliencecontrol.CommandStatusOK, Component: change.Component, Budget: change.Budget}
	if s == nil || s.stateStore == nil {
		return result, resiliencecontrol.ErrUnavailable
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
			return result, resiliencecontrol.ErrVersionConflict
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
				return result, resiliencecontrol.ErrVersionConflict
			}
			payload, _ := json.Marshal(resiliencecontrol.RateLimitChange{Mode: "reset", Component: change.Component, Budget: change.Budget})
			published, err := s.stateStore.CompareAndSwap(ctx, name, change.ExpectedVersion, resiliencecontrol.VersionedState{Payload: payload, Actor: actor}, 0)
			if err != nil {
				return result, err
			}
			result.Version = published.Version
		}
		if !exists {
			if local == nil || local.Snapshot().Source == "config" {
				result.Status, result.Version, result.Source = resiliencecontrol.CommandStatusNoop, change.ExpectedVersion, "config"
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
	published, err := s.stateStore.CompareAndSwap(ctx, name, change.ExpectedVersion, resiliencecontrol.VersionedState{Payload: payload, Actor: actor}, ttl)
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

func (s *Subsystem) ensureRateBaseline(ctx context.Context, name string, version uint64, actor resiliencecontrol.ActionActor) error {
	state, exists, err := s.stateStore.Load(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		if state.Version != version {
			return resiliencecontrol.ErrVersionConflict
		}
		return nil
	}
	_, err = s.stateStore.CompareAndSwap(ctx, name, 0, resiliencecontrol.VersionedState{Version: version, Payload: []byte(`{"mode":"config"}`), Actor: actor}, 0)
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
	return fmt.Errorf("%w: %s", resiliencecontrol.ErrInvalidArgument, fmt.Sprintf(format, args...))
}

func overridePolicy(current ratelimit.RateLimitPolicy, change resiliencecontrol.RatePolicy) ratelimit.RateLimitPolicy {
	current.RatePerSecond = change.RatePerSecond
	current.Burst = change.Burst
	return current
}

func rateStateName(component, budget string) string { return "rate:" + component + ":" + budget }

func queueStateName(component, queue string) string { return "queue:" + component + ":" + queue }

func leaderStateName(component, instanceID, workload string) string {
	return "leader:" + component + ":" + instanceID + ":" + workload
}

func (s *Subsystem) SetQueueState(ctx context.Context, actor resiliencecontrol.ActionActor, change resiliencecontrol.QueueChange) (resiliencecontrol.QueueChangeResult, error) {
	result := resiliencecontrol.QueueChangeResult{Status: resiliencecontrol.CommandStatusOK, Component: change.Component, Queue: change.Queue, State: change.DesiredState}
	if s == nil || s.stateStore == nil {
		return result, resiliencecontrol.ErrUnavailable
	}
	commands, ok := s.stateStore.(resiliencecontrol.CommandStore)
	if !ok || change.RequestID == "" {
		return result, resiliencecontrol.ErrUnavailable
	}
	if change.Component != "collection-server" || change.Queue != "answersheet_submit" {
		return result, invalidArgument("unsupported queue target %s/%s", change.Component, change.Queue)
	}
	if change.Target == "" {
		change.Target = "all"
	}
	if change.DesiredState != resiliencecontrol.QueueStatePaused && change.DesiredState != resiliencecontrol.QueueStateActive {
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
	payload, err := json.Marshal(change)
	if err != nil {
		return result, err
	}
	published, err := s.stateStore.CompareAndSwap(ctx, name, expected, resiliencecontrol.VersionedState{Payload: payload, Actor: actor}, 0)
	if err != nil {
		return result, err
	}
	result.Version = published.Version
	command := resiliencecontrol.Command{
		RequestID: change.RequestID, ActionID: queueActionID(change.DesiredState),
		Target:  resiliencecontrol.Target{Component: change.Component, InstanceID: change.Target},
		Payload: payload, Actor: actor, IssuedAt: time.Now(),
	}
	timeout := time.Duration(change.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Minute
	}
	command.ExpiresAt = time.Now().Add(timeout + time.Minute)
	expectedInstances, err := commandTargetInstances(ctx, commands, change.Component, change.Target)
	if err != nil {
		return result, err
	}
	if len(expectedInstances) == 0 {
		result.Status = resiliencecontrol.CommandStatusNoop
		return result, nil
	}
	if err := commands.PublishCommand(ctx, command, timeout+time.Minute); err != nil {
		return result, err
	}
	results, status, err := waitCommandResults(ctx, commands, actor.OrgID, change.RequestID, expectedInstances, timeout)
	result.Instances, result.Status = results, status
	if change.DesiredState == resiliencecontrol.QueueStateActive && status != resiliencecontrol.CommandStatusOK && status != resiliencecontrol.CommandStatusNoop {
		rollback := change
		rollback.DesiredState = resiliencecontrol.QueueStatePaused
		rollbackPayload, _ := json.Marshal(rollback)
		if restored, restoreErr := s.stateStore.CompareAndSwap(ctx, name, published.Version, resiliencecontrol.VersionedState{Payload: rollbackPayload, Actor: actor}, 0); restoreErr == nil {
			result.Version = restored.Version
		}
	}
	return result, err
}

func queueActionID(state resiliencecontrol.QueueState) string {
	if state == resiliencecontrol.QueueStateActive {
		return "resilience.resume_queue"
	}
	return "resilience.drain_queue"
}

func (s *Subsystem) RelinquishLeader(ctx context.Context, actor resiliencecontrol.ActionActor, change resiliencecontrol.LeaderChange) (any, error) {
	if s == nil || s.stateStore == nil || change.RequestID == "" {
		return nil, resiliencecontrol.ErrUnavailable
	}
	commands, ok := s.stateStore.(resiliencecontrol.CommandStore)
	if !ok {
		return nil, resiliencecontrol.ErrUnavailable
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
		return map[string]interface{}{"status": resiliencecontrol.CommandStatusNoop, "instances": []resiliencecontrol.CommandResult{}}, nil
	}
	command := resiliencecontrol.Command{RequestID: change.RequestID, ActionID: "resilience.release_lock",
		Target: resiliencecontrol.Target{Component: change.Component, InstanceID: change.InstanceID}, Payload: payload,
		Actor: actor, IssuedAt: time.Now(), ExpiresAt: time.Now().Add(timeout + time.Minute)}
	if err := commands.PublishCommand(ctx, command, timeout+time.Minute); err != nil {
		return nil, err
	}
	results, status, waitErr := waitCommandResults(ctx, commands, actor.OrgID, change.RequestID, instances, timeout)
	return map[string]interface{}{"status": status, "instances": results, "cooldown_seconds": int(cooldown.Seconds())}, waitErr
}

func commandTargetInstances(ctx context.Context, store resiliencecontrol.CommandStore, component, target string) ([]string, error) {
	instances, err := store.ListInstances(ctx, component)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, identity := range instances {
		if target == "" || target == "all" || target == identity.InstanceID {
			result = append(result, identity.InstanceID)
		}
	}
	return result, nil
}

func waitCommandResults(ctx context.Context, store resiliencecontrol.CommandStore, orgID int64, requestID string, expected []string, timeout time.Duration) ([]resiliencecontrol.CommandResult, resiliencecontrol.CommandStatus, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		results, err := store.ListCommandResults(waitCtx, orgID, requestID)
		if err != nil {
			return nil, resiliencecontrol.CommandStatusFailed, err
		}
		if len(results) >= len(expected) {
			status := resiliencecontrol.CommandStatusOK
			allNoop, allTimeout := len(results) > 0, len(results) > 0
			failed, succeeded := false, false
			for _, result := range results {
				allNoop = allNoop && result.Status == resiliencecontrol.CommandStatusNoop
				allTimeout = allTimeout && result.Status == resiliencecontrol.CommandStatusTimeout
				failed = failed || result.Status == resiliencecontrol.CommandStatusFailed || result.Status == resiliencecontrol.CommandStatusTimeout
				succeeded = succeeded || result.Status == resiliencecontrol.CommandStatusOK || result.Status == resiliencecontrol.CommandStatusNoop
			}
			switch {
			case allNoop:
				status = resiliencecontrol.CommandStatusNoop
			case allTimeout:
				status = resiliencecontrol.CommandStatusTimeout
			case failed && succeeded:
				status = resiliencecontrol.CommandStatusPartial
			case failed:
				status = resiliencecontrol.CommandStatusFailed
			}
			return results, status, nil
		}
		select {
		case <-waitCtx.Done():
			if len(results) > 0 {
				return results, resiliencecontrol.CommandStatusPartial, nil
			}
			return results, resiliencecontrol.CommandStatusTimeout, nil
		case <-ticker.C:
		}
	}
}

var _ resiliencecontrol.Governor = (*Subsystem)(nil)
