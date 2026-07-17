package redisadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
	redis "github.com/redis/go-redis/v9"
)

type Store struct {
	client  redis.UniversalClient
	builder *keyspace.Builder
}

func NewStore(client redis.UniversalClient, builder *keyspace.Builder) *Store {
	if builder == nil {
		builder = keyspace.NewBuilder()
	}
	return &Store{client: client, builder: builder}
}

func (s *Store) Load(ctx context.Context, name string) (resiliencecontrol.VersionedState, bool, error) {
	if s == nil || s.client == nil {
		return resiliencecontrol.VersionedState{}, false, resiliencecontrol.ErrUnavailable
	}
	raw, err := s.client.Get(ctx, s.builder.BuildResilienceStateKey(name)).Bytes()
	if errors.Is(err, redis.Nil) {
		return resiliencecontrol.VersionedState{}, false, nil
	}
	if err != nil {
		return resiliencecontrol.VersionedState{}, false, err
	}
	var state resiliencecontrol.VersionedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return resiliencecontrol.VersionedState{}, false, fmt.Errorf("decode resilience state %q: %w", name, err)
	}
	return state, true, nil
}

func (s *Store) CompareAndSwap(ctx context.Context, name string, expected uint64, candidate resiliencecontrol.VersionedState, ttl time.Duration) (resiliencecontrol.VersionedState, error) {
	if s == nil || s.client == nil {
		return resiliencecontrol.VersionedState{}, resiliencecontrol.ErrUnavailable
	}
	key := s.builder.BuildResilienceStateKey(name)
	var published resiliencecontrol.VersionedState
	err := s.client.Watch(ctx, func(tx *redis.Tx) error {
		current, exists, err := loadTx(ctx, tx, key)
		if err != nil {
			return err
		}
		currentVersion := uint64(0)
		if exists {
			currentVersion = current.Version
		}
		if currentVersion != expected {
			return resiliencecontrol.ErrVersionConflict
		}
		if candidate.Version <= expected {
			candidate.Version = expected + 1
		}
		candidate.UpdatedAt = time.Now().UTC()
		if ttl > 0 {
			candidate.ExpiresAt = candidate.UpdatedAt.Add(ttl)
		}
		raw, err := json.Marshal(candidate)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, raw, ttl)
			return nil
		})
		if err == nil {
			published = candidate
		}
		return err
	}, key)
	if errors.Is(err, redis.TxFailedErr) {
		err = resiliencecontrol.ErrVersionConflict
	}
	if err == nil {
		_ = s.client.Publish(ctx, s.builder.BuildResilienceSignalChannel(), name).Err()
	}
	return published, err
}

func (s *Store) Delete(ctx context.Context, name string, expected uint64) error {
	if s == nil || s.client == nil {
		return resiliencecontrol.ErrUnavailable
	}
	key := s.builder.BuildResilienceStateKey(name)
	err := s.client.Watch(ctx, func(tx *redis.Tx) error {
		current, exists, err := loadTx(ctx, tx, key)
		if err != nil {
			return err
		}
		if !exists || current.Version != expected {
			return resiliencecontrol.ErrVersionConflict
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Del(ctx, key)
			return nil
		})
		return err
	}, key)
	if errors.Is(err, redis.TxFailedErr) {
		return resiliencecontrol.ErrVersionConflict
	}
	if err == nil {
		_ = s.client.Publish(ctx, s.builder.BuildResilienceSignalChannel(), name).Err()
	}
	return err
}

func (s *Store) Claim(ctx context.Context, requestID, instanceID string, ttl time.Duration) (bool, error) {
	if s == nil || s.client == nil {
		return false, resiliencecontrol.ErrUnavailable
	}
	return s.client.SetNX(ctx, s.builder.BuildResilienceClaimKey(requestID, instanceID), "1", ttl).Result()
}

func (s *Store) PublishCommand(ctx context.Context, command resiliencecontrol.Command, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return resiliencecontrol.ErrUnavailable
	}
	raw, err := json.Marshal(command)
	if err != nil {
		return err
	}
	created, err := s.client.SetNX(ctx, s.builder.BuildResilienceCommandKey(command.Target.Component, resiliencecontrol.ScopedRequestID(command.Actor.OrgID, command.RequestID)), raw, ttl).Result()
	if err != nil {
		return err
	}
	if !created {
		return nil
	}
	_ = s.client.Publish(ctx, s.builder.BuildResilienceSignalChannel(), "command:"+command.Target.Component).Err()
	return nil
}

func (s *Store) ListCommands(ctx context.Context, component, instanceID string) ([]resiliencecontrol.Command, error) {
	if s == nil || s.client == nil {
		return nil, resiliencecontrol.ErrUnavailable
	}
	pattern := s.builder.BuildResilienceCommandKey(component, "*")
	commands := []resiliencecontrol.Command{}
	iter := s.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		raw, err := s.client.Get(ctx, iter.Val()).Bytes()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var command resiliencecontrol.Command
		if json.Unmarshal(raw, &command) != nil {
			continue
		}
		if command.Target.InstanceID == "" || command.Target.InstanceID == "all" || command.Target.InstanceID == instanceID {
			commands = append(commands, command)
		}
	}
	return commands, iter.Err()
}

func (s *Store) PutCommandResult(ctx context.Context, result resiliencecontrol.CommandResult, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return resiliencecontrol.ErrUnavailable
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.builder.BuildResilienceCommandResultKey(resiliencecontrol.ScopedRequestID(result.OrgID, result.RequestID), result.InstanceID), raw, ttl).Err()
}

func (s *Store) ListCommandResults(ctx context.Context, orgID int64, requestID string) ([]resiliencecontrol.CommandResult, error) {
	if s == nil || s.client == nil {
		return nil, resiliencecontrol.ErrUnavailable
	}
	pattern := s.builder.BuildResilienceCommandResultKey(resiliencecontrol.ScopedRequestID(orgID, requestID), "*")
	results := []resiliencecontrol.CommandResult{}
	iter := s.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		raw, err := s.client.Get(ctx, iter.Val()).Bytes()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var result resiliencecontrol.CommandResult
		if json.Unmarshal(raw, &result) == nil {
			results = append(results, result)
		}
	}
	return results, iter.Err()
}

func (s *Store) ListInstances(ctx context.Context, component string) ([]resiliencecontrol.InstanceIdentity, error) {
	if s == nil || s.client == nil {
		return nil, resiliencecontrol.ErrUnavailable
	}
	pattern := s.builder.BuildResilienceInstanceKey(component, "*")
	instances := []resiliencecontrol.InstanceIdentity{}
	iter := s.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		raw, err := s.client.Get(ctx, iter.Val()).Bytes()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var identity resiliencecontrol.InstanceIdentity
		if json.Unmarshal(raw, &identity) == nil {
			instances = append(instances, identity)
		}
	}
	return instances, iter.Err()
}

func (s *Store) Heartbeat(ctx context.Context, identity resiliencecontrol.InstanceIdentity, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return resiliencecontrol.ErrUnavailable
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.builder.BuildResilienceInstanceKey(identity.Component, identity.InstanceID), raw, ttl).Err()
}

func (s *Store) WatchStateSignals(ctx context.Context) (<-chan string, error) {
	if s == nil || s.client == nil {
		return nil, resiliencecontrol.ErrUnavailable
	}
	subscription := s.client.Subscribe(ctx, s.builder.BuildResilienceSignalChannel())
	if _, err := subscription.Receive(ctx); err != nil {
		_ = subscription.Close()
		return nil, err
	}
	out := make(chan string, 1)
	go func() {
		defer close(out)
		defer subscription.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case message, ok := <-subscription.Channel():
				if !ok {
					return
				}
				select {
				case out <- message.Payload:
				default:
				}
			}
		}
	}()
	return out, nil
}

func loadTx(ctx context.Context, tx *redis.Tx, key string) (resiliencecontrol.VersionedState, bool, error) {
	raw, err := tx.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return resiliencecontrol.VersionedState{}, false, nil
	}
	if err != nil {
		return resiliencecontrol.VersionedState{}, false, err
	}
	var state resiliencecontrol.VersionedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return resiliencecontrol.VersionedState{}, false, err
	}
	return state, true, nil
}

var _ resiliencecontrol.StateStore = (*Store)(nil)
var _ resiliencecontrol.InstanceHeartbeater = (*Store)(nil)
var _ resiliencecontrol.StateSignalWatcher = (*Store)(nil)
var _ resiliencecontrol.CommandStore = (*Store)(nil)
