package redisadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/control"
	redis "github.com/redis/go-redis/v9"
)

const persistentIndexScore int64 = 9007199254740991

var setIndexedValueScript = redis.NewScript(`
local ttl = tonumber(ARGV[2])
if ttl > 0 then
	redis.call('SET', KEYS[1], ARGV[1], 'PX', ttl)
else
	redis.call('SET', KEYS[1], ARGV[1])
end
redis.call('ZADD', KEYS[2], ARGV[3], KEYS[1])
return 1
`)

var setNXIndexedValueScript = redis.NewScript(`
local ttl = tonumber(ARGV[2])
local created
if ttl > 0 then
	created = redis.call('SET', KEYS[1], ARGV[1], 'PX', ttl, 'NX')
else
	created = redis.call('SET', KEYS[1], ARGV[1], 'NX')
end
if not created then
	local remaining = redis.call('PTTL', KEYS[1])
	local score = ARGV[3]
	if remaining > 0 then
		score = tonumber(ARGV[4]) + remaining
	elseif remaining == -1 then
		score = ARGV[5]
	end
	redis.call('ZADD', KEYS[2], score, KEYS[1])
	return 0
end
redis.call('ZADD', KEYS[2], ARGV[3], KEYS[1])
return 1
`)

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

func (s *Store) Load(ctx context.Context, name string) (control.VersionedState, bool, error) {
	if s == nil || s.client == nil {
		return control.VersionedState{}, false, control.ErrUnavailable
	}
	raw, err := s.client.Get(ctx, s.builder.BuildResilienceStateKey(name)).Bytes()
	if errors.Is(err, redis.Nil) {
		return control.VersionedState{}, false, nil
	}
	if err != nil {
		return control.VersionedState{}, false, err
	}
	var state control.VersionedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return control.VersionedState{}, false, fmt.Errorf("decode resilience state %q: %w", name, err)
	}
	return state, true, nil
}

func (s *Store) CompareAndSwap(ctx context.Context, name string, expected uint64, candidate control.VersionedState, ttl time.Duration) (control.VersionedState, error) {
	if s == nil || s.client == nil {
		return control.VersionedState{}, control.ErrUnavailable
	}
	key := s.builder.BuildResilienceStateKey(name)
	var published control.VersionedState
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
			return control.ErrVersionConflict
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
		err = control.ErrVersionConflict
	}
	if err == nil {
		_ = s.client.Publish(ctx, s.builder.BuildResilienceSignalChannel(), name).Err()
	}
	return published, err
}

func (s *Store) Delete(ctx context.Context, name string, expected uint64) error {
	if s == nil || s.client == nil {
		return control.ErrUnavailable
	}
	key := s.builder.BuildResilienceStateKey(name)
	err := s.client.Watch(ctx, func(tx *redis.Tx) error {
		current, exists, err := loadTx(ctx, tx, key)
		if err != nil {
			return err
		}
		if !exists || current.Version != expected {
			return control.ErrVersionConflict
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Del(ctx, key)
			return nil
		})
		return err
	}, key)
	if errors.Is(err, redis.TxFailedErr) {
		return control.ErrVersionConflict
	}
	if err == nil {
		_ = s.client.Publish(ctx, s.builder.BuildResilienceSignalChannel(), name).Err()
	}
	return err
}

func (s *Store) Claim(ctx context.Context, requestID, instanceID string, ttl time.Duration) (bool, error) {
	if s == nil || s.client == nil {
		return false, control.ErrUnavailable
	}
	return s.client.SetNX(ctx, s.builder.BuildResilienceClaimKey(requestID, instanceID), "1", ttl).Result()
}

func (s *Store) PublishCommand(ctx context.Context, command control.Command, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return control.ErrUnavailable
	}
	raw, err := json.Marshal(command)
	if err != nil {
		return err
	}
	now := time.Now()
	ttlMillis, score := indexedExpiry(now, ttl)
	commandKey := s.builder.BuildResilienceCommandKey(command.Target.Component, control.ScopedRequestID(command.Actor.OrgID, command.RequestID))
	indexKey := s.builder.BuildResilienceCommandIndexKey(command.Target.Component)
	created, err := setNXIndexedValueScript.Run(
		ctx,
		s.client,
		[]string{commandKey, indexKey},
		raw,
		ttlMillis,
		score,
		now.UnixMilli(),
		persistentIndexScore,
	).Int64()
	if err != nil {
		return err
	}
	if created == 0 {
		return nil
	}
	_ = s.client.Publish(ctx, s.builder.BuildResilienceSignalChannel(), "command:"+command.Target.Component).Err()
	return nil
}

func (s *Store) ListCommands(ctx context.Context, component, instanceID string) ([]control.Command, error) {
	if s == nil || s.client == nil {
		return nil, control.ErrUnavailable
	}
	commands := []control.Command{}
	values, err := s.activeIndexedValues(ctx, s.builder.BuildResilienceCommandIndexKey(component), time.Now())
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		var command control.Command
		if json.Unmarshal(value.raw, &command) != nil {
			continue
		}
		if command.Target.InstanceID == "" || command.Target.InstanceID == "all" || command.Target.InstanceID == instanceID {
			commands = append(commands, command)
		}
	}
	return commands, nil
}

func (s *Store) PutCommandResult(ctx context.Context, result control.CommandResult, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return control.ErrUnavailable
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	scopedRequestID := control.ScopedRequestID(result.OrgID, result.RequestID)
	return s.setIndexedValue(
		ctx,
		s.builder.BuildResilienceCommandResultKey(scopedRequestID, result.InstanceID),
		s.builder.BuildResilienceCommandResultIndexKey(scopedRequestID),
		raw,
		ttl,
	)
}

func (s *Store) ListCommandResults(ctx context.Context, orgID int64, requestID string) ([]control.CommandResult, error) {
	if s == nil || s.client == nil {
		return nil, control.ErrUnavailable
	}
	results := []control.CommandResult{}
	indexKey := s.builder.BuildResilienceCommandResultIndexKey(control.ScopedRequestID(orgID, requestID))
	values, err := s.activeIndexedValues(ctx, indexKey, time.Now())
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		var result control.CommandResult
		if json.Unmarshal(value.raw, &result) == nil {
			results = append(results, result)
		}
	}
	return results, nil
}

func (s *Store) ListInstances(ctx context.Context, component string) ([]control.InstanceIdentity, error) {
	if s == nil || s.client == nil {
		return nil, control.ErrUnavailable
	}
	instances := []control.InstanceIdentity{}
	seen := make(map[string]struct{})
	values, err := s.activeIndexedValues(ctx, s.builder.BuildResilienceInstanceIndexKey(component), time.Now())
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		var identity control.InstanceIdentity
		if json.Unmarshal(value.raw, &identity) == nil {
			key := identity.InstanceID + "\x00" + identity.Generation
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			instances = append(instances, identity)
		}
	}
	return instances, nil
}

func (s *Store) Heartbeat(ctx context.Context, identity control.InstanceIdentity, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return control.ErrUnavailable
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	return s.setIndexedValue(
		ctx,
		s.builder.BuildResilienceInstanceKey(identity.Component, identity.InstanceID, identity.Generation),
		s.builder.BuildResilienceInstanceIndexKey(identity.Component),
		raw,
		ttl,
	)
}

type indexedValue struct {
	key string
	raw []byte
}

func indexedExpiry(now time.Time, ttl time.Duration) (int64, int64) {
	if ttl <= 0 {
		return 0, persistentIndexScore
	}
	ttlMillis := ttl.Milliseconds()
	if ttlMillis <= 0 {
		ttlMillis = 1
	}
	return ttlMillis, now.Add(time.Duration(ttlMillis) * time.Millisecond).UnixMilli()
}

func (s *Store) setIndexedValue(ctx context.Context, valueKey, indexKey string, raw []byte, ttl time.Duration) error {
	ttlMillis, score := indexedExpiry(time.Now(), ttl)
	return setIndexedValueScript.Run(ctx, s.client, []string{valueKey, indexKey}, raw, ttlMillis, score).Err()
}

func (s *Store) activeIndexedValues(ctx context.Context, indexKey string, now time.Time) ([]indexedValue, error) {
	nowMillis := strconv.FormatInt(now.UnixMilli(), 10)
	if err := s.client.ZRemRangeByScore(ctx, indexKey, "-inf", nowMillis).Err(); err != nil {
		return nil, err
	}
	keys, err := s.client.ZRangeByScore(ctx, indexKey, &redis.ZRangeBy{Min: "(" + nowMillis, Max: "+inf"}).Result()
	if err != nil || len(keys) == 0 {
		return []indexedValue{}, err
	}
	rawValues, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	values := make([]indexedValue, 0, len(keys))
	stale := make([]interface{}, 0)
	for i, rawValue := range rawValues {
		if rawValue == nil {
			stale = append(stale, keys[i])
			continue
		}
		raw, ok := rawValue.(string)
		if !ok {
			continue
		}
		values = append(values, indexedValue{key: keys[i], raw: []byte(raw)})
	}
	if len(stale) > 0 {
		if err := s.client.ZRem(ctx, indexKey, stale...).Err(); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (s *Store) WatchStateSignals(ctx context.Context) (<-chan string, error) {
	if s == nil || s.client == nil {
		return nil, control.ErrUnavailable
	}
	subscription := s.client.Subscribe(ctx, s.builder.BuildResilienceSignalChannel())
	if _, err := subscription.Receive(ctx); err != nil {
		_ = subscription.Close()
		return nil, err
	}
	out := make(chan string, 1)
	go func() {
		defer close(out)
		defer func() { _ = subscription.Close() }()
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

func loadTx(ctx context.Context, tx *redis.Tx, key string) (control.VersionedState, bool, error) {
	raw, err := tx.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return control.VersionedState{}, false, nil
	}
	if err != nil {
		return control.VersionedState{}, false, err
	}
	var state control.VersionedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return control.VersionedState{}, false, err
	}
	return state, true, nil
}

var _ control.StateStore = (*Store)(nil)
var _ control.InstanceHeartbeater = (*Store)(nil)
var _ control.StateSignalWatcher = (*Store)(nil)
var _ control.CommandStore = (*Store)(nil)
