package subsystem

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
)

func (s *Subsystem) processCommands(ctx context.Context) {
	store, ok := s.stateStore.(resiliencecontrol.CommandStore)
	if !ok {
		return
	}
	commands, err := store.ListCommands(ctx, "apiserver", s.identity.InstanceID)
	if err != nil {
		return
	}
	for _, command := range commands {
		if command.ActionID != "resilience.release_lock" {
			continue
		}
		claimed, err := store.Claim(ctx, resiliencecontrol.ScopedRequestID(command.Actor.OrgID, command.RequestID), s.identity.InstanceID, time.Until(command.ExpiresAt)+time.Minute)
		if err != nil || !claimed {
			continue
		}
		go s.executeLeaderCommand(ctx, store, command)
	}
}

func (s *Subsystem) executeLeaderCommand(ctx context.Context, store resiliencecontrol.CommandStore, command resiliencecontrol.Command) {
	result := resiliencecontrol.CommandResult{RequestID: command.RequestID, ActionID: command.ActionID,
		OrgID: command.Actor.OrgID, Component: "apiserver", InstanceID: s.identity.InstanceID, Status: resiliencecontrol.CommandStatusFailed}
	var change resiliencecontrol.LeaderChange
	if err := json.Unmarshal(command.Payload, &change); err != nil {
		result.Message = err.Error()
		result.FinishedAt = time.Now()
		_ = store.PutCommandResult(context.WithoutCancel(ctx), result, 10*time.Minute)
		return
	}
	capability, ok := locklease.Lookup(locklease.WorkloadID(change.Workload))
	if !ok || capability.Component != "apiserver" || capability.Kind != locklease.KindLeader || s.locks == nil {
		result.Message = "workload is not a releasable apiserver leader"
		result.FinishedAt = time.Now()
		_ = store.PutCommandResult(context.WithoutCancel(ctx), result, 10*time.Minute)
		return
	}
	cooldown := time.Duration(change.CooldownSeconds) * time.Second
	if cooldown <= 0 {
		cooldown = capability.Spec.DefaultTTL
	}
	name := leaderStateName("apiserver", s.identity.InstanceID, change.Workload)
	current, exists, err := s.stateStore.Load(ctx, name)
	if err == nil {
		expected := uint64(0)
		if exists {
			expected = current.Version
		}
		_, err = s.stateStore.CompareAndSwap(ctx, name, expected, resiliencecontrol.VersionedState{Payload: command.Payload, Actor: command.Actor}, cooldown)
	}
	if err == nil {
		leaseResult, relinquishErr := s.locks.RelinquishLeader(ctx, capability.ID, locklease.RelinquishOptions{
			Cooldown: cooldown, Timeout: time.Duration(change.TimeoutSeconds) * time.Second,
		})
		result.Payload, _ = json.Marshal(leaseResult)
		if relinquishErr == nil {
			result.Status = resiliencecontrol.CommandStatusOK
			if leaseResult.ActiveCount == 0 {
				result.Status = resiliencecontrol.CommandStatusNoop
			}
		} else {
			result.Message = relinquishErr.Error()
			if errors.Is(relinquishErr, context.DeadlineExceeded) {
				result.Status = resiliencecontrol.CommandStatusTimeout
			}
		}
	} else {
		result.Message = err.Error()
	}
	result.FinishedAt = time.Now()
	_ = store.PutCommandResult(context.WithoutCancel(ctx), result, 10*time.Minute)
}
