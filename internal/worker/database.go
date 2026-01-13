package worker

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	redis "github.com/redis/go-redis/v9"
)

// DatabaseManager 数据库管理器
type DatabaseManager struct {
	registry   *database.Registry
	config     *config.Config
	cacheRedis *database.RedisConnection
}

// NewDatabaseManager 创建数据库管理器
func NewDatabaseManager(cfg *config.Config) *DatabaseManager {
	return &DatabaseManager{
		registry: database.NewRegistry(),
		config:   cfg,
	}
}

// Initialize 初始化所有数据库连接
func (m *DatabaseManager) Initialize() error {
	log.Info("Initializing database connections...")

	// 初始化Redis连接
	if err := m.initRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// 初始化数据库连接
	if err := m.registry.Init(); err != nil {
		return fmt.Errorf("failed to initialize database connections: %w", err)
	}

	log.Info("All database connections initialized successfully")
	return nil
}

// initRedis 初始化Redis连接（双实例架构）
func (m *DatabaseManager) initRedis() error {
	if m.config.Redis == nil {
		log.Warn("Redis not configured, skipping")
		return nil
	}

	redisCfg := m.config.Redis
	if redisCfg == nil || (redisCfg.Host == "" && len(redisCfg.Addrs) == 0) {
		log.Warn("Redis not configured, skipping")
		return nil
	}

	cacheConfig := &database.RedisConfig{
		Host:                  redisCfg.Host,
		Port:                  redisCfg.Port,
		Addrs:                 redisCfg.Addrs,
		Username:              redisCfg.Username,
		Password:              redisCfg.Password,
		Database:              redisCfg.Database,
		MaxIdle:               redisCfg.MaxIdle,
		MaxActive:             redisCfg.MaxActive,
		Timeout:               redisCfg.Timeout,
		MinIdleConns:          redisCfg.MinIdleConns,
		PoolTimeout:           redisCfg.PoolTimeout,
		DialTimeout:           redisCfg.DialTimeout,
		ReadTimeout:           redisCfg.ReadTimeout,
		WriteTimeout:          redisCfg.WriteTimeout,
		EnableCluster:         redisCfg.EnableCluster,
		UseSSL:                redisCfg.UseSSL,
		SSLInsecureSkipVerify: redisCfg.SSLInsecureSkipVerify,
	}

	cacheConn := database.NewRedisConnection(cacheConfig)
	if err := m.registry.Register(database.Redis, cacheConfig, cacheConn); err != nil {
		return fmt.Errorf("failed to register redis: %w", err)
	}
	m.cacheRedis = cacheConn
	log.Info("Redis initialized successfully")

	return nil
}

// GetRedisClient 获取缓存 Redis 客户端
func (m *DatabaseManager) GetRedisClient() (redis.UniversalClient, error) {
	client, err := m.registry.GetClient(database.Redis)
	if err != nil {
		return nil, err
	}

	redisClient, ok := client.(redis.UniversalClient)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to redis.UniversalClient")
	}

	return redisClient, nil
}

// Close 关闭所有数据库连接
func (m *DatabaseManager) Close() error {
	log.Info("Closing database connections...")

	if err := m.registry.Close(); err != nil {
		log.Warnf("Failed to close registry: %v", err)
	}

	log.Info("All database connections closed")
	return nil
}
