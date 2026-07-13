package query

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
)

// Versioned owns the version-token + versioned-key query cache path.
type Versioned struct {
	version    VersionTokenStore
	capability sharedcache.Capability
	policy     sharedcache.Policy
	ttl        time.Duration
	memory     *LocalHotCache[[]byte]
	observer   sharedcache.Observer
	payload    *redisstore.PayloadStore
}

type VersionedOptions struct {
	Store      sharedcache.Store
	Version    VersionTokenStore
	Capability sharedcache.Capability
	Policy     sharedcache.Policy
	TTL        time.Duration
	Memory     *LocalHotCache[[]byte]
	Observer   sharedcache.Observer
}

func NewVersioned(opts VersionedOptions) *Versioned {
	if opts.Store == nil || opts.Version == nil {
		return nil
	}
	return &Versioned{
		version:    opts.Version,
		capability: opts.Capability,
		policy:     opts.Policy,
		ttl:        opts.TTL,
		memory:     opts.Memory,
		observer:   opts.Observer,
		payload:    redisstore.NewPayloadStore(opts.Store, opts.Policy, opts.Observer),
	}
}

func (c *Versioned) CurrentVersion(ctx context.Context, versionKey string) (uint64, error) {
	if c == nil || c.version == nil {
		return 0, sharedcache.ErrMiss
	}
	return c.version.Current(ctx, versionKey)
}

func (c *Versioned) Get(ctx context.Context, versionKey string, buildDataKey func(uint64) string, dest any) error {
	if c == nil || c.payload == nil || buildDataKey == nil {
		return sharedcache.ErrMiss
	}
	version, err := c.CurrentVersion(ctx, versionKey)
	if err != nil {
		sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultMiss})
		return sharedcache.ErrMiss
	}
	key := buildDataKey(version)
	if c.memory != nil {
		if data, ok := c.memory.Get(key); ok {
			sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultHit})
			sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationPayloadRaw, Size: len(data)})
			sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationPayloadSet, Size: len(data)})
			if err := json.Unmarshal(data, dest); err != nil {
				sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultError, Err: err})
				return sharedcache.ErrMiss
			}
			return nil
		}
	}

	start := time.Now()
	data, err := c.payload.Get(ctx, key)
	if err != nil {
		result := sharedcache.ResultError
		if errors.Is(err, sharedcache.ErrMiss) {
			result = sharedcache.ResultMiss
		}
		sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: result, Duration: time.Since(start), Err: nonMissError(err)})
		if result == sharedcache.ResultError {
			sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultMiss})
		}
		return sharedcache.ErrMiss
	}
	if err := json.Unmarshal(data, dest); err != nil {
		sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultError, Duration: time.Since(start), Err: err})
		sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultMiss})
		return sharedcache.ErrMiss
	}
	sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationGet, Result: sharedcache.ResultHit, Duration: time.Since(start)})
	if c.memory != nil {
		c.memory.Set(key, data)
	}
	return nil
}

func (c *Versioned) Set(ctx context.Context, versionKey string, buildDataKey func(uint64) string, value any) {
	if c == nil || c.payload == nil || buildDataKey == nil || value == nil {
		return
	}
	version, err := c.CurrentVersion(ctx, versionKey)
	if err != nil {
		return
	}
	key := buildDataKey(version)
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	if c.memory != nil {
		c.memory.Set(key, raw)
	}
	start := time.Now()
	err = c.payload.Set(ctx, key, raw, c.policy.TTLOr(c.ttl))
	result := sharedcache.ResultOK
	if err != nil {
		result = sharedcache.ResultError
	}
	sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationSet, Result: result, Duration: time.Since(start), Err: err})
}

func (c *Versioned) Invalidate(ctx context.Context, versionKey string) error {
	if c == nil || c.version == nil {
		return nil
	}
	_, err := c.version.Bump(ctx, versionKey)
	result := sharedcache.ResultOK
	if err != nil {
		result = sharedcache.ResultError
	}
	sharedcache.Observe(c.observer, sharedcache.Event{Operation: sharedcache.OperationInvalidate, Result: result, Err: err})
	return err
}

func nonMissError(err error) error {
	if errors.Is(err, sharedcache.ErrMiss) {
		return nil
	}
	return err
}
