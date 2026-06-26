package actor

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for actor module installation.
type WireInput struct {
	MySQLDB             *gorm.DB
	RedisClient         redis.UniversalClient
	CacheBuilder        *keyspace.Builder
	TesteePolicy        cachepolicy.CachePolicy
	Observer            *observability.ComponentObserver
	TopicResolver       eventcatalog.TopicResolver
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
		TesteePolicy:        in.TesteePolicy,
		Observer:            in.Observer,
		TopicResolver:       in.TopicResolver,
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
