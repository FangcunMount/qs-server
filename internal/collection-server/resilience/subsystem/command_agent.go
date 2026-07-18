package subsystem

import (
	"context"
	"encoding/json"
	"errors"
	"time"

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
	s.queueMu.RLock()
	queue, ok := s.queues[change.Queue]
	s.queueMu.RUnlock()
	if !ok {
		result.Message = "queue controller is not registered"
		s.finishQueueCommand(ctx, store, result)
		return
	}
	switch command.ActionID {
	case "resilience.drain_queue":
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
		if err := queue.controller.Resume(ctx); err != nil {
			result.Message = err.Error()
		} else {
			result.Status = control.CommandStatusOK
		}
	}
	s.finishQueueCommand(ctx, store, result)
}

func (s *Subsystem) finishQueueCommand(ctx context.Context, store control.CommandStore, result control.CommandResult) {
	result.FinishedAt = time.Now()
	_ = store.PutCommandResult(context.WithoutCancel(ctx), result, 10*time.Minute)
}
