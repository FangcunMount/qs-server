package iamauth

import (
	"context"
	"fmt"
	sdkauthz "github.com/FangcunMount/iam/v5/pkg/sdk/authz"
	"strconv"
	"sync"
	"time"

	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"golang.org/x/sync/singleflight"
)

// SnapshotLoaderOptions 配置 IAM GetAuthorizationSnapshot。
type SnapshotLoaderOptions struct {
	includeAssignmentFacts bool
	AppName                string
	CacheTTL               time.Duration
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

			AppName: l.opts.AppName, IncludeAssignmentFacts: l.opts.includeAssignmentFacts,
		})
		if err != nil {
			return nil, err
		}
		if resp.GetScopeContractVersion() != 0 {
			if err := sdkauthz.ValidateScopedSnapshot(resp); err != nil {
				return nil, err
			}
		}
		directRoles := append([]string(nil), resp.GetDirectRoles()...)
		snap := &authz.Snapshot{
			ScopeContractVersion: resp.GetScopeContractVersion(),
			DirectRoles:          directRoles,
			// Independent role model: wire field retained; value equals direct roles.
			EffectiveRoles: append([]string(nil), directRoles...),
			AuthzVersion:   resp.GetPolicyVersion(),

			IAMAppName: l.opts.AppName,
		}
		snap.AssignmentFactsComplete = resp.GetAssignmentFactsComplete()
		for _, f := range resp.GetAssignmentFacts() {
			if f == nil {
				return nil, fmt.Errorf("invalid assignment fact")
			}
			snap.AssignmentFacts = append(snap.AssignmentFacts, authz.AssignmentRoleFact{RoleID: f.GetRoleId(), RoleName: f.GetRoleName(), ManagementProtection: f.GetManagementProtection()})
		}
		for _, p := range resp.GetPermissions() {
			if p == nil {
				continue
			}
			permission := authz.Permission{Resource: p.GetResource(), Action: p.GetAction(), Mode: authz.AuthorizationMode(p.GetMode())}
			for _, value := range p.GetScopes() {
				// A legacy producer cannot establish a data range.
				if snap.ScopeContractVersion != 1 {
					break
				}
				org, _ := strconv.ParseInt(value.GetOrgId(), 10, 64)
				kind := "stores"
				if value.GetKind() == authzv4.DataScopeKind_ALL_STORES {
					kind = "all_stores"
				}
				stores := make([]uint64, 0, len(value.GetStoreIds()))
				for _, raw := range value.GetStoreIds() {
					id, _ := strconv.ParseUint(raw, 10, 64)
					stores = append(stores, id)
				}
				permission.Scopes = append(permission.Scopes, authz.DataScope{OrgID: org, Kind: kind, StoreIDs: stores})
			}
			snap.Permissions = append(snap.Permissions, permission)
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

// LoadFresh bypasses the process cache and in-flight cached reads for maintenance decisions.
func (l *SnapshotLoader) LoadFresh(ctx context.Context, userID string) (*authz.Snapshot, error) {
	if l == nil {
		return nil, fmt.Errorf("authorization loader unavailable")
	}
	fresh := NewSnapshotLoader(l.client, l.opts)
	l.mu.Lock()
	fresh.globalVersion = l.globalVersion
	l.mu.Unlock()
	snap, err := fresh.Load(ctx, userID)
	if err != nil {
		return nil, err
	}
	if snap == nil || snap.AuthzVersion <= 0 {
		return nil, fmt.Errorf("invalid authoritative authorization snapshot")
	}
	// Recheck the shared watermark after the remote read; a concurrent policy event must not be lost.
	if err := l.setCached(cacheKey(userID, l.opts.AppName), snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// LoadAssignmentFacts is restricted by IAM and never falls back to an app-filtered role list.
func (l *SnapshotLoader) LoadAssignmentFacts(ctx context.Context, userID string) (*authz.Snapshot, error) {
	if l == nil {
		return nil, fmt.Errorf("authorization loader unavailable")
	}
	opts := l.opts
	opts.includeAssignmentFacts = true
	fresh := NewSnapshotLoader(l.client, opts)
	l.mu.Lock()
	fresh.globalVersion = l.globalVersion
	l.mu.Unlock()
	snap, err := fresh.Load(ctx, userID)
	if err != nil {
		return nil, err
	}
	if snap == nil || !snap.AssignmentFactsComplete || snap.AuthzVersion <= 0 {
		return nil, fmt.Errorf("IAM does not provide complete assignment facts; retirement is unavailable")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if snap.AuthzVersion < l.globalVersion {
		return nil, fmt.Errorf("assignment facts are older than observed policy version")
	}
	l.globalVersion = snap.AuthzVersion
	return snap, nil
}

// LoadScopedAssignmentFacts reads authoritative management facts without caching
// or falling back to legacy role-only projections.
func (l *SnapshotLoader) LoadScopedAssignmentFacts(ctx context.Context, userID string) (*authzv4.GetAuthorizationSnapshotResponse, error) {
	if l == nil || l.client == nil || l.client.SDK() == nil {
		return nil, fmt.Errorf("authorization loader unavailable")
	}
	resp, err := l.client.SDK().Authz().GetScopedAuthorizationSnapshot(ctx, &authzv4.GetAuthorizationSnapshotRequest{Subject: authz.SubjectKey(userID), AppName: l.opts.AppName, IncludeAssignmentFacts: true})
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if resp.PolicyVersion < l.globalVersion {
		return nil, fmt.Errorf("assignment scope facts older than observed policy version")
	}
	l.globalVersion = resp.PolicyVersion
	return resp, nil
}
