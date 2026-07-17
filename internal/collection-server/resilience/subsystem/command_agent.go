package subsystem

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
)

func (s *Subsystem) processCommands(ctx context.Context) bool {
	store, ok := s.stateStore.(resiliencecontrol.CommandStore)
	if !ok {
		return false
	}
	commands, err := store.ListCommands(ctx, "collection-server", s.identity.InstanceID)
	if err != nil {
		return false
	}
	found := false
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
		found = true
		claimed, err := store.Claim(ctx, resiliencecontrol.ScopedRequestID(command.Actor.OrgID, command.RequestID), s.identity.InstanceID, time.Until(command.ExpiresAt)+time.Minute)
		if err != nil || !claimed {
			continue
		}
		go s.executeQueueCommand(ctx, store, command)
	}
	return found
}

func (s *Subsystem) executeQueueCommand(ctx context.Context, store resiliencecontrol.CommandStore, command resiliencecontrol.Command) {
	result := resiliencecontrol.CommandResult{RequestID: command.RequestID, ActionID: command.ActionID,
		OrgID: command.Actor.OrgID, Component: "collection-server", InstanceID: s.identity.InstanceID, Status: resiliencecontrol.CommandStatusFailed}
	var change resiliencecontrol.QueueChange
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
		drained, err := queue.controller.Drain(ctx, resiliencecontrol.DrainOptions{Timeout: time.Duration(change.TimeoutSeconds) * time.Second})
		result.Payload, _ = json.Marshal(drained)
		if err == nil {
			result.Status = resiliencecontrol.CommandStatusOK
		} else {
			result.Message = err.Error()
			if errors.Is(err, context.DeadlineExceeded) {
				result.Status = resiliencecontrol.CommandStatusTimeout
			}
		}
	case "resilience.resume_queue":
		if err := queue.controller.Resume(ctx); err != nil {
			result.Message = err.Error()
		} else {
			result.Status = resiliencecontrol.CommandStatusOK
		}
	}
	s.finishQueueCommand(ctx, store, result)
}

func (s *Subsystem) finishQueueCommand(ctx context.Context, store resiliencecontrol.CommandStore, result resiliencecontrol.CommandResult) {
	result.FinishedAt = time.Now()
	_ = store.PutCommandResult(context.WithoutCancel(ctx), result, 10*time.Minute)
}
