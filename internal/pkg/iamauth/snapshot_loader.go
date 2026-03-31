package iamauth

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	authzv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/authz/v1"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"golang.org/x/sync/singleflight"
)

// SnapshotLoaderOptions 配置 IAM GetAuthorizationSnapshot。
type SnapshotLoaderOptions struct {
	AppName              string
	CacheTTL             time.Duration
	CasbinDomainOverride string
}

// SnapshotLoader CurrentAuthzSnapshot：GetAuthorizationSnapshot + 进程内缓存 + authz_version 水位失效。
type SnapshotLoader struct {
	client GRPCClient
	opts   SnapshotLoaderOptions

	mu             sync.Mutex
	cache          map[string]cachedSnap
	tenantAuthzVer map[string]int64
	group          singleflight.Group
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
		client:         client,
		opts:           opts,
		cache:          make(map[string]cachedSnap),
		tenantAuthzVer: make(map[string]int64),
	}
}

func cacheKey(domain, userID, app string) string {
	return domain + "\x00" + userID + "\x00" + app
}

func (l *SnapshotLoader) getCached(key, domain string) *authz.Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	ent, ok := l.cache[key]
	if !ok || time.Now().After(ent.expiresAt) {
		return nil
	}
	if ent.snap != nil {
		if global, ok := l.tenantAuthzVer[domain]; ok && ent.snap.AuthzVersion < global {
			return nil
		}
	}
	return ent.snap
}

func (l *SnapshotLoader) setCached(key, domain string, snap *authz.Snapshot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if snap != nil && snap.AuthzVersion > l.tenantAuthzVer[domain] {
		l.tenantAuthzVer[domain] = snap.AuthzVersion
	}
	l.cache[key] = cachedSnap{snap: snap, expiresAt: time.Now().Add(l.opts.CacheTTL)}
}

// DomainForOrg 与 Load / Grant / Revoke 共用的 Casbin domain。
func (l *SnapshotLoader) DomainForOrg(orgID int64) string {
	if l != nil && l.opts.CasbinDomainOverride != "" {
		return l.opts.CasbinDomainOverride
	}
	return strconv.FormatInt(orgID, 10)
}

// Load 拉取授权快照。
func (l *SnapshotLoader) Load(ctx context.Context, jwtTenantID, userIDStr string) (*authz.Snapshot, error) {
	if l == nil || l.client == nil || !l.client.IsEnabled() || l.client.SDK() == nil {
		return nil, fmt.Errorf("iam client not available for authorization snapshot")
	}
	domain := jwtTenantID
	if l.opts.CasbinDomainOverride != "" {
		domain = l.opts.CasbinDomainOverride
	}
	if domain == "" || userIDStr == "" {
		return nil, fmt.Errorf("domain and user id are required")
	}

	key := cacheKey(domain, userIDStr, l.opts.AppName)
	if snap := l.getCached(key, domain); snap != nil {
		return snap, nil
	}

	v, err, _ := l.group.Do(key, func() (interface{}, error) {
		if snap := l.getCached(key, domain); snap != nil {
			return snap, nil
		}
		sub := authz.SubjectKey(userIDStr)
		resp, err := l.client.SDK().Authz().GetAuthorizationSnapshot(ctx, &authzv1.GetAuthorizationSnapshotRequest{
			Subject: sub,
			Domain:  domain,
			AppName: l.opts.AppName,
		})
		if err != nil {
			return nil, err
		}
		snap := &authz.Snapshot{
			Roles:        append([]string(nil), resp.GetRoles()...),
			AuthzVersion: resp.GetAuthzVersion(),
			CasbinDomain: domain,
			IAMAppName:   l.opts.AppName,
		}
		for _, p := range resp.GetPermissions() {
			if p == nil {
				continue
			}
			snap.Permissions = append(snap.Permissions, authz.Permission{
				Resource: p.GetResource(),
				Action:   p.GetAction(),
			})
		}
		l.setCached(key, domain, snap)
		return snap, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*authz.Snapshot), nil
}
