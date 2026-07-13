package actor

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

// BootstrapInput carries container integration inputs for actor module bootstrap.
type BootstrapInput struct {
	MySQLDB             *gorm.DB
	ProfileLinkService  *iam.ProfileLinkService
	IdentityService     *iam.IdentityService
	OperationAccountSvc *iam.OperationAccountService
	OperatorAuthz       *iam.OperatorAuthzBundle
	RedisClient         redis.UniversalClient
	CacheBuilder        *keyspace.Builder
	CachePolicies       sharedcache.PolicyProvider
	Observer            *observability.ComponentObserver
	MySQLLimiter        backpressure.Acquirer
}

// Bootstrap assembles the actor module from container integration inputs.
func Bootstrap(in BootstrapInput) (*Module, error) {
	return New(Deps{
		MySQLDB:             in.MySQLDB,
		ProfileLinkService:  in.ProfileLinkService,
		IdentityService:     in.IdentityService,
		RedisClient:         in.RedisClient,
		CacheBuilder:        in.CacheBuilder,
		CachePolicies:       in.CachePolicies,
		OperatorAuthz:       in.OperatorAuthz,
		OperationAccountSvc: in.OperationAccountSvc,
		Observer:            in.Observer,
		MySQLLimiter:        in.MySQLLimiter,
	})
}
