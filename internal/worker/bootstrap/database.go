package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// DatabaseManager 数据库管理器
type DatabaseManager struct {
	config          *config.Config
	registry        *database.Registry
	redisProfiles   *database.NamedRedisRegistry
	mongoRegistered bool
}

// NewDatabaseManager 创建数据库管理器
func NewDatabaseManager(cfg *config.Config) *DatabaseManager {
	return &DatabaseManager{
		config:   cfg,
		registry: database.NewRegistry(),
	}
}

// Initialize 初始化所有数据库连接
func (m *DatabaseManager) Initialize() error {
	log.Info("Initializing database connections...")

	if err := m.initRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	if err := m.initMongoDB(); err != nil {
		return fmt.Errorf("failed to initialize MongoDB: %w", err)
	}

	if m.registry != nil && m.mongoRegistered {
		if err := m.registry.Init(); err != nil {
			return fmt.Errorf("failed to initialize database registry: %w", err)
		}
	}

	log.Info("All database connections initialized successfully")
	return nil
}

// initRedis 初始化默认 Redis 与可选 named profiles。
func (m *DatabaseManager) initRedis() error {
	defaultConfig := toWorkerDatabaseRedisConfig(m.config.Redis)
	redisProfiles := make(map[string]*database.RedisConfig)
	for name, cfg := range m.config.RedisProfiles {
		databaseCfg := mergeWorkerDatabaseRedisConfig(defaultConfig, toWorkerDatabaseRedisConfig(cfg))
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
	if m.registry != nil {
		if err := m.registry.Close(); err != nil {
			log.Warnf("Failed to close database registry: %v", err)
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

func mergeWorkerDatabaseRedisConfig(base, override *database.RedisConfig) *database.RedisConfig {
	if override == nil {
		return cloneWorkerDatabaseRedisConfig(base)
	}
	if base == nil {
		return cloneWorkerDatabaseRedisConfig(override)
	}

	merged := cloneWorkerDatabaseRedisConfig(base)
	if merged == nil {
		merged = &database.RedisConfig{}
	}

	if override.Host != "" {
		merged.Host = override.Host
	}
	if override.Port != 0 {
		merged.Port = override.Port
	}
	if len(override.Addrs) > 0 {
		merged.Addrs = append([]string(nil), override.Addrs...)
	}
	if override.Username != "" {
		merged.Username = override.Username
	}
	if override.Password != "" {
		merged.Password = override.Password
	}

	merged.Database = override.Database

	if override.MaxIdle != 0 {
		merged.MaxIdle = override.MaxIdle
	}
	if override.MaxActive != 0 {
		merged.MaxActive = override.MaxActive
	}
	if override.Timeout != 0 {
		merged.Timeout = override.Timeout
	}
	if override.MinIdleConns != 0 {
		merged.MinIdleConns = override.MinIdleConns
	}
	if override.PoolTimeout != 0 {
		merged.PoolTimeout = override.PoolTimeout
	}
	if override.DialTimeout != 0 {
		merged.DialTimeout = override.DialTimeout
	}
	if override.ReadTimeout != 0 {
		merged.ReadTimeout = override.ReadTimeout
	}
	if override.WriteTimeout != 0 {
		merged.WriteTimeout = override.WriteTimeout
	}
	if override.EnableCluster {
		merged.EnableCluster = true
	}
	if override.UseSSL {
		merged.UseSSL = true
	}
	if override.SSLInsecureSkipVerify {
		merged.SSLInsecureSkipVerify = true
	}

	return merged
}

func (m *DatabaseManager) initMongoDB() error {
	if m.config == nil || m.config.MongoDB == nil {
		log.Warn("MongoDB not configured, skipping")
		return nil
	}
	opts := m.config.MongoDB
	if opts.URL == "" && opts.Host == "" {
		log.Warn("MongoDB host not configured, skipping")
		return nil
	}
	if opts.URL == "" && opts.Database == "" {
		log.Warn("MongoDB database not configured, skipping")
		return nil
	}
	mongoConfig := &database.MongoConfig{
		URL:                      opts.URL,
		Host:                     opts.Host,
		Username:                 opts.Username,
		Password:                 opts.Password,
		Database:                 opts.Database,
		ReplicaSet:               opts.ReplicaSet,
		DirectConnection:         opts.DirectConnection,
		UseSSL:                   opts.UseSSL,
		SSLInsecureSkipVerify:    opts.SSLInsecureSkipVerify,
		SSLAllowInvalidHostnames: opts.SSLAllowInvalidHostnames,
		SSLCAFile:                opts.SSLCAFile,
		SSLPEMKeyfile:            opts.SSLPEMKeyfile,
		EnableLogger:             opts.EnableLogger,
		SlowThreshold:            opts.SlowThreshold,
		LogCommandDetail:         opts.LogCommandDetail,
		LogReplyDetail:           opts.LogReplyDetail,
		LogStarted:               opts.LogStarted,
	}
	if err := m.registry.Register(database.MongoDB, mongoConfig, database.NewMongoDBConnection(mongoConfig)); err != nil {
		return err
	}
	m.mongoRegistered = true
	return nil
}

// GetMongoDatabase returns the configured Mongo database when available.
func (m *DatabaseManager) GetMongoDatabase() (*mongo.Database, error) {
	if m == nil || m.registry == nil || m.config == nil || m.config.MongoDB == nil {
		return nil, fmt.Errorf("mongodb is not configured")
	}
	if m.config.MongoDB.URL == "" && m.config.MongoDB.Host == "" {
		return nil, fmt.Errorf("mongodb is not configured")
	}
	if m.config.MongoDB.URL == "" && m.config.MongoDB.Database == "" {
		return nil, fmt.Errorf("mongodb is not configured")
	}
	client, err := m.registry.GetClient(database.MongoDB)
	if err != nil {
		return nil, err
	}
	mongoClient, ok := client.(*mongo.Client)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to *mongo.Client")
	}
	return mongoClient.Database(m.config.MongoDB.Database), nil
}

func cloneWorkerDatabaseRedisConfig(cfg *database.RedisConfig) *database.RedisConfig {
	if cfg == nil {
		return nil
	}
	copyCfg := *cfg
	if len(cfg.Addrs) > 0 {
		copyCfg.Addrs = append([]string(nil), cfg.Addrs...)
	}
	return &copyCfg
}
