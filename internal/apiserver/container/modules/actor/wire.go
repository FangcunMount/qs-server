package actor

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for actor module installation.
type WireInput struct {
	MySQLDB             *gorm.DB
	RedisClient         redis.UniversalClient
	CacheBuilder        *keyspace.Builder
	CachePolicies       sharedcache.PolicyProvider
	Observer            *observability.ComponentObserver
	MySQLLimiter        backpressure.Acquirer
	IAMEnabled          bool
	ProfileLinkService  *iam.ProfileLinkService
	IdentityService     *iam.IdentityService
	OperationAccountSvc *iam.OperationAccountService
	IAMClient           *iam.Client
	AuthzSnapshotLoader *iam.AuthzSnapshotLoader
}

// Wire builds and bootstraps the actor module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	bootstrap := BootstrapInput{
		MySQLDB:             in.MySQLDB,
		RedisClient:         in.RedisClient,
		CacheBuilder:        in.CacheBuilder,
		CachePolicies:       in.CachePolicies,
		Observer:            in.Observer,
		MySQLLimiter:        in.MySQLLimiter,
		ProfileLinkService:  in.ProfileLinkService,
		IdentityService:     in.IdentityService,
		OperationAccountSvc: in.OperationAccountSvc,
	}
	if in.IAMEnabled {
		bootstrap.OperatorAuthz = &iam.OperatorAuthzBundle{
			Assignment: iam.NewAuthzAssignmentClient(in.IAMClient),
			Snapshot:   in.AuthzSnapshotLoader,
		}
	}
	return Bootstrap(bootstrap)
}
