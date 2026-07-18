package subsystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
)

func (s *Subsystem) processCommands(ctx context.Context) bool {
	store, ok := s.stateStore.(control.CommandStore)
	if !ok {
		return false
	}
	commands, err := store.ListCommands(ctx, "collection-server", s.identity.InstanceID)
	if err != nil {
		return false
	}
	found := s.hasActiveCommands()
	for _, command := range commands {
		if command.ActionID != "resilience.drain_queue" && command.ActionID != "resilience.resume_queue" {
			continue
		}
		results, _ := store.ListCommandResults(ctx, command.Actor.OrgID, command.RequestID)
		finished := false
		for _, result := range results {
			finished = finished || result.InstanceID == s.identity.InstanceID
		}
		if finished {
			continue
		}
		commandKey := control.ScopedRequestID(command.Actor.OrgID, command.RequestID)
		if s.commandActive(commandKey) {
			found = true
			continue
		}
		claimed, err := store.Claim(ctx, control.ScopedRequestID(command.Actor.OrgID, command.RequestID), s.identity.InstanceID, time.Until(command.ExpiresAt)+time.Minute)
		if err != nil || !claimed {
			continue
		}
		s.markCommandActive(commandKey)
		found = true
		go func() {
			defer s.clearCommandActive(commandKey)
			s.executeQueueCommand(ctx, store, command)
		}()
	}
	return found
}

func (s *Subsystem) commandActive(key string) bool {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	_, ok := s.activeCommands[key]
	return ok
}

func (s *Subsystem) hasActiveCommands() bool {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	return len(s.activeCommands) > 0
}

func (s *Subsystem) markCommandActive(key string) {
	s.commandMu.Lock()
	s.activeCommands[key] = struct{}{}
	s.commandMu.Unlock()
}

func (s *Subsystem) clearCommandActive(key string) {
	s.commandMu.Lock()
	delete(s.activeCommands, key)
	s.commandMu.Unlock()
}

func (s *Subsystem) executeQueueCommand(ctx context.Context, store control.CommandStore, command control.Command) {
	result := control.CommandResult{RequestID: command.RequestID, ActionID: command.ActionID,
		OrgID: command.Actor.OrgID, Component: "collection-server", InstanceID: s.identity.InstanceID, Status: control.CommandStatusFailed}
	var change control.QueueChange
	if err := json.Unmarshal(command.Payload, &change); err != nil {
		result.Message = err.Error()
		s.finishQueueCommand(ctx, store, result)
		return
	}
	if err := s.waitForQueueCommandCommit(ctx, command, change); err != nil {
		result.Message = err.Error()
		s.finishQueueCommand(ctx, store, result)
		return
	}
	s.queueMu.RLock()
	queue, ok := s.queues[change.Queue]
	s.queueMu.RUnlock()
	if !ok {
		result.Message = "queue controller is not registered"
		s.finishQueueCommand(ctx, store, result)
		return
	}
	snapshot := queue.snapshot(time.Now())
	switch command.ActionID {
	case "resilience.drain_queue":
		if snapshot.State == string(control.QueueStatePaused) {
			result.Status = control.CommandStatusNoop
			result.Payload, _ = json.Marshal(snapshot)
			break
		}
		timeout := time.Duration(change.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = time.Minute
		}
		drained, err := queue.controller.Drain(ctx, control.DrainOptions{Timeout: timeout})
		result.Payload, _ = json.Marshal(drained)
		if err == nil {
			result.Status = control.CommandStatusOK
		} else {
			result.Message = err.Error()
			if errors.Is(err, context.DeadlineExceeded) {
				result.Status = control.CommandStatusTimeout
			}
		}
	case "resilience.resume_queue":
		if snapshot.State == string(control.QueueStateActive) {
			result.Status = control.CommandStatusNoop
			result.Payload, _ = json.Marshal(snapshot)
			break
		}
		if err := queue.controller.Resume(ctx); err != nil {
			result.Message = err.Error()
		} else {
			result.Status = control.CommandStatusOK
		}
	}
	s.finishQueueCommand(ctx, store, result)
}

func (s *Subsystem) waitForQueueCommandCommit(ctx context.Context, command control.Command, change control.QueueChange) error {
	deadline := command.ExpiresAt
	if deadline.IsZero() || time.Until(deadline) > time.Minute {
		deadline = time.Now().Add(time.Minute)
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	name := "queue:" + change.Component + ":" + change.Queue
	for {
		state, exists, err := s.stateStore.Load(ctx, name)
		if err == nil && exists {
			var committed control.QueueChange
			if json.Unmarshal(state.Payload, &committed) == nil {
				if queueCommandCommitted(state, committed, change) {
					return nil
				}
				if change.StateVersion > 0 && state.Version > change.StateVersion {
					return fmt.Errorf("%w: queue command state version %d was not committed", control.ErrInvalidState, change.StateVersion)
				}
			}
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: queue command was not committed before expiry", control.ErrInvalidState)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func queueCommandCommitted(state control.VersionedState, committed, command control.QueueChange) bool {
	if committed.DesiredState != command.DesiredState || committed.Component != command.Component || committed.Queue != command.Queue {
		return false
	}
	committedTarget, commandTarget := committed.Target, command.Target
	if committedTarget == "" {
		committedTarget = "all"
	}
	if commandTarget == "" {
		commandTarget = "all"
	}
	if committedTarget != commandTarget {
		return false
	}
	if command.StateVersion == 0 {
		return true
	}
	return state.Version == command.StateVersion && committed.StateVersion == command.StateVersion &&
		committed.RequestID == command.RequestID
}

func (s *Subsystem) finishQueueCommand(ctx context.Context, store control.CommandStore, result control.CommandResult) {
	result.FinishedAt = time.Now()
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	delay := 25 * time.Millisecond
	for {
		err := store.PutCommandResult(writeCtx, result, 10*time.Minute)
		if err == nil {
			resilience.ObserveControlOperation("collection-server", "command_result_write", "ok")
			return
		}
		select {
		case <-writeCtx.Done():
			resilience.ObserveControlOperation("collection-server", "command_result_write", "failed")
			logger.L(ctx).Errorw("failed to persist resilience command result",
				"request_id", result.RequestID, "action_id", result.ActionID, "error", err)
			return
		case <-time.After(delay):
			if delay < 500*time.Millisecond {
				delay *= 2
			}
		}
	}
}
