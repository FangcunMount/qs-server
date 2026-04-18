package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	redis "github.com/redis/go-redis/v9"
)

// DatabaseManager 数据库管理器
type DatabaseManager struct {
	config        *config.Config
	redisProfiles *database.NamedRedisRegistry
}

// NewDatabaseManager 创建数据库管理器
func NewDatabaseManager(cfg *config.Config) *DatabaseManager {
	return &DatabaseManager{
		config: cfg,
	}
}

// Initialize 初始化所有数据库连接
func (m *DatabaseManager) Initialize() error {
	log.Info("Initializing database connections...")

	if err := m.initRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	log.Info("All database connections initialized successfully")
	return nil
}

// initRedis 初始化默认 Redis 与可选 named profiles。
func (m *DatabaseManager) initRedis() error {
	defaultConfig := toWorkerDatabaseRedisConfig(m.config.Redis)
	redisProfiles := make(map[string]*database.RedisConfig)
	for name, cfg := range m.config.RedisProfiles {
		databaseCfg := toWorkerDatabaseRedisConfig(cfg)
		if databaseCfg == nil || (databaseCfg.Host == "" && len(databaseCfg.Addrs) == 0) {
			continue
		}
		redisProfiles[name] = databaseCfg
	}

	if defaultConfig == nil && len(redisProfiles) == 0 {
		log.Warn("Redis not configured, skipping")
		return nil
	}

	m.redisProfiles = database.NewNamedRedisRegistry(defaultConfig, redisProfiles)
	if err := m.redisProfiles.Connect(); err != nil {
		return err
	}

	log.Infof("Redis initialized successfully (profile_count=%d)", len(redisProfiles))
	return nil
}

// GetRedisClient 获取默认 Redis 客户端。
func (m *DatabaseManager) GetRedisClient() (redis.UniversalClient, error) {
	return m.GetRedisClientByProfile("")
}

// GetRedisClientByProfile 获取指定 profile 的 Redis 客户端。
// 未配置的 profile 回退默认 Redis；已配置但不可用的 profile 返回错误。
func (m *DatabaseManager) GetRedisClientByProfile(profile string) (redis.UniversalClient, error) {
	if m.redisProfiles == nil {
		return nil, fmt.Errorf("redis is not configured")
	}
	return m.redisProfiles.GetClient(profile)
}

// GetRedisProfileStatus 返回指定 profile 当前的可用性状态。
func (m *DatabaseManager) GetRedisProfileStatus(profile string) database.RedisProfileStatus {
	if m == nil || m.redisProfiles == nil {
		return database.RedisProfileStatus{
			Name:  profile,
			State: database.RedisProfileStateMissing,
		}
	}
	return m.redisProfiles.ProfileStatus(profile)
}

// HealthCheck 数据库健康检查。
func (m *DatabaseManager) HealthCheck() error {
	if m.redisProfiles == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.redisProfiles.HealthCheck(ctx)
}

// Close 关闭所有数据库连接
func (m *DatabaseManager) Close() error {
	log.Info("Closing database connections...")

	if m.redisProfiles != nil {
		if err := m.redisProfiles.Close(); err != nil {
			log.Warnf("Failed to close redis profiles: %v", err)
			return err
		}
	}

	log.Info("All database connections closed")
	return nil
}

func toWorkerDatabaseRedisConfig(opts *options.RedisOptions) *database.RedisConfig {
	if opts == nil {
		return nil
	}
	return &database.RedisConfig{
		Host:                  opts.Host,
		Port:                  opts.Port,
		Addrs:                 opts.Addrs,
		Username:              opts.Username,
		Password:              opts.Password,
		Database:              opts.Database,
		MaxIdle:               opts.MaxIdle,
		MaxActive:             opts.MaxActive,
		Timeout:               opts.Timeout,
		MinIdleConns:          opts.MinIdleConns,
		PoolTimeout:           opts.PoolTimeout,
		DialTimeout:           opts.DialTimeout,
		ReadTimeout:           opts.ReadTimeout,
		WriteTimeout:          opts.WriteTimeout,
		EnableCluster:         opts.EnableCluster,
		UseSSL:                opts.UseSSL,
		SSLInsecureSkipVerify: opts.SSLInsecureSkipVerify,
	}
}
