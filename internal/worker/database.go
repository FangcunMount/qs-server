package worker

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	redis "github.com/redis/go-redis/v9"
)

// DatabaseManager 数据库管理器
type DatabaseManager struct {
	registry   *database.Registry
	config     *config.Config
	cacheRedis *database.RedisConnection
	storeRedis *database.RedisConnection
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

	// 初始化 Cache Redis
	if m.config.Redis.Cache != nil && m.config.Redis.Cache.Host != "" {
		cacheConfig := &database.RedisConfig{
			Host:          m.config.Redis.Cache.Host,
			Port:          m.config.Redis.Cache.Port,
			Username:      m.config.Redis.Cache.Username,
			Password:      m.config.Redis.Cache.Password,
			Database:      m.config.Redis.Cache.Database,
			MaxIdle:       m.config.Redis.Cache.MaxIdle,
			MaxActive:     m.config.Redis.Cache.MaxActive,
			Timeout:       m.config.Redis.Cache.Timeout,
			EnableCluster: m.config.Redis.Cache.EnableCluster,
			UseSSL:        m.config.Redis.Cache.UseSSL,
		}

		cacheConn := database.NewRedisConnection(cacheConfig)
		if err := m.registry.Register(database.Redis, cacheConfig, cacheConn); err != nil {
			return fmt.Errorf("failed to register cache redis: %w", err)
		}
		m.cacheRedis = cacheConn
		log.Info("Cache Redis initialized successfully")
	}

	// 初始化 Store Redis
	if m.config.Redis.Store != nil && m.config.Redis.Store.Host != "" {
		storeConfig := &database.RedisConfig{
			Host:          m.config.Redis.Store.Host,
			Port:          m.config.Redis.Store.Port,
			Username:      m.config.Redis.Store.Username,
			Password:      m.config.Redis.Store.Password,
			Database:      m.config.Redis.Store.Database,
			MaxIdle:       m.config.Redis.Store.MaxIdle,
			MaxActive:     m.config.Redis.Store.MaxActive,
			Timeout:       m.config.Redis.Store.Timeout,
			EnableCluster: m.config.Redis.Store.EnableCluster,
			UseSSL:        m.config.Redis.Store.UseSSL,
		}

		storeConn := database.NewRedisConnection(storeConfig)
		if err := storeConn.Connect(); err != nil {
			return fmt.Errorf("failed to connect store redis: %w", err)
		}
		m.storeRedis = storeConn
		log.Infof("Store Redis connected successfully to %s:%d", storeConfig.Host, storeConfig.Port)
	}

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

// GetStoreRedisClient 获取存储 Redis 客户端
func (m *DatabaseManager) GetStoreRedisClient() (redis.UniversalClient, error) {
	if m.storeRedis == nil {
		return nil, fmt.Errorf("store redis not initialized")
	}
	return m.storeRedis.GetClient().(redis.UniversalClient), nil
}

// Close 关闭所有数据库连接
func (m *DatabaseManager) Close() error {
	log.Info("Closing database connections...")

	if m.storeRedis != nil {
		if err := m.storeRedis.Close(); err != nil {
			log.Warnf("Failed to close Store Redis: %v", err)
		}
	}

	if err := m.registry.Close(); err != nil {
		log.Warnf("Failed to close registry: %v", err)
	}

	log.Info("All database connections closed")
	return nil
}
