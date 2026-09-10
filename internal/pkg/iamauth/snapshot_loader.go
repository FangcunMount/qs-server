package iamauth

import (
	"context"
	"fmt"
	"sync"
	"time"

	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"golang.org/x/sync/singleflight"
)

// SnapshotLoaderOptions 配置 IAM GetAuthorizationSnapshot。
type SnapshotLoaderOptions struct {
	AppName  string
	CacheTTL time.Duration
}

// SnapshotLoader CurrentAuthzSnapshot：GetAuthorizationSnapshot + 进程内缓存 + authz_version 水位失效。
type SnapshotLoader struct {
	client GRPCClient
	opts   SnapshotLoaderOptions

	mu            sync.Mutex
	cache         map[string]cachedSnap
	globalVersion int64
	group         singleflight.Group
}

type cachedSnap struct {
	snap      *authz.Snapshot
	expiresAt time.Time
}

// NewSnapshotLoader 创建加载器。
func NewSnapshotLoader(client GRPCClient, opts SnapshotLoaderOptions) *SnapshotLoader {
	if opts.AppName == "" {
		opts.AppName = "qs"
	}
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = 30 * time.Second
	}
	return &SnapshotLoader{
		client: client,
		opts:   opts,
		cache:  make(map[string]cachedSnap),
	}
}

func cacheKey(userID, app string) string {
	return userID + "\x00" + app
}

func (l *SnapshotLoader) getCached(key string) *authz.Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	ent, ok := l.cache[key]
	if !ok || time.Now().After(ent.expiresAt) {
		return nil
	}
	if ent.snap != nil {
		if ent.snap.AuthzVersion < l.globalVersion {
			return nil
		}
	}
	return ent.snap
}

func (l *SnapshotLoader) setCached(key string, snap *authz.Snapshot) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if snap == nil || snap.AuthzVersion < l.globalVersion {
		return fmt.Errorf("authorization snapshot is older than the observed policy version")
	}
	if snap.AuthzVersion > l.globalVersion {
		l.globalVersion = snap.AuthzVersion
	}
	l.cache[key] = cachedSnap{snap: snap, expiresAt: time.Now().Add(l.opts.CacheTTL)}
	return nil
}

// ObserveAuthzVersion 推进全局授权版本水位并剔除旧快照。
func (l *SnapshotLoader) ObserveAuthzVersion(version int64) {
	if l == nil || version <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if version <= l.globalVersion {
		return
	}
	l.globalVersion = version
	for key, ent := range l.cache {
		if ent.snap != nil && ent.snap.AuthzVersion < version {
			delete(l.cache, key)
		}
	}
}

// Load 拉取授权快照。
func (l *SnapshotLoader) Load(ctx context.Context, userIDStr string) (*authz.Snapshot, error) {
	if l == nil || l.client == nil || !l.client.IsEnabled() || l.client.SDK() == nil {
		return nil, fmt.Errorf("iam client not available for authorization snapshot")
	}
	if userIDStr == "" {
		return nil, fmt.Errorf("user id is required")
	}

	key := cacheKey(userIDStr, l.opts.AppName)
	if snap := l.getCached(key); snap != nil {
		return snap, nil
	}

	v, err, _ := l.group.Do(key, func() (interface{}, error) {
		if snap := l.getCached(key); snap != nil {
			return snap, nil
		}
		sub := authz.SubjectKey(userIDStr)
		resp, err := l.client.SDK().Authz().GetAuthorizationSnapshot(ctx, &authzv4.GetAuthorizationSnapshotRequest{
			Subject: sub,

			AppName: l.opts.AppName,
		})
		if err != nil {
			return nil, err
		}
		directRoles := append([]string(nil), resp.GetDirectRoles()...)
		snap := &authz.Snapshot{
			DirectRoles: directRoles,
			// Independent role model: wire field retained; value equals direct roles.
			EffectiveRoles: append([]string(nil), directRoles...),
			AuthzVersion:   resp.GetPolicyVersion(),

			IAMAppName: l.opts.AppName,
		}
		for _, p := range resp.GetPermissions() {
			if p == nil {
				continue
			}
			snap.Permissions = append(snap.Permissions, authz.Permission{
				Resource: p.GetResource(),
				Action:   p.GetAction(),
				Mode:     authz.AuthorizationMode(p.GetMode()),
			})
		}
		if err := l.setCached(key, snap); err != nil {
			return nil, err
		}
		return snap, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*authz.Snapshot), nil
}
