package collection

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
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
func (dm *DatabaseManager) Initialize() error {
	log.Info("Initializing database connections...")

	// 初始化Redis连接
	if err := dm.initRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// 初始化数据库连接
	if err := dm.registry.Init(); err != nil {
		return fmt.Errorf("failed to initialize database connections: %w", err)
	}

	log.Info("All database connections initialized successfully")
	return nil
}

// initRedis 初始化Redis连接（双实例架构）
func (dm *DatabaseManager) initRedis() error {
	// 初始化 Cache Redis
	cacheConfig := &database.RedisConfig{
		Host:                  dm.config.RedisDualOptions.Cache.Host,
		Port:                  dm.config.RedisDualOptions.Cache.Port,
		Addrs:                 []string{}, // 双实例模式暂不支持集群
		Username:              dm.config.RedisDualOptions.Cache.Username,
		Password:              dm.config.RedisDualOptions.Cache.Password,
		Database:              dm.config.RedisDualOptions.Cache.Database,
		MaxIdle:               dm.config.RedisDualOptions.Cache.MaxIdle,
		MaxActive:             dm.config.RedisDualOptions.Cache.MaxActive,
		Timeout:               dm.config.RedisDualOptions.Cache.Timeout,
		EnableCluster:         dm.config.RedisDualOptions.Cache.EnableCluster,
		UseSSL:                dm.config.RedisDualOptions.Cache.UseSSL,
		SSLInsecureSkipVerify: false,
	}

	if cacheConfig.Host == "" {
		log.Info("Cache Redis host not configured, skipping Cache Redis initialization")
	} else {
		cacheConn := database.NewRedisConnection(cacheConfig)
		// 注册为主 Redis 实例（保持向后兼容）
		if err := dm.registry.Register(database.Redis, cacheConfig, cacheConn); err != nil {
			return fmt.Errorf("failed to register cache redis: %w", err)
		}
		dm.cacheRedis = cacheConn
		log.Info("Cache Redis initialized successfully")
	}

	// 初始化 Store Redis
	storeConfig := &database.RedisConfig{
		Host:                  dm.config.RedisDualOptions.Store.Host,
		Port:                  dm.config.RedisDualOptions.Store.Port,
		Addrs:                 []string{},
		Username:              dm.config.RedisDualOptions.Store.Username,
		Password:              dm.config.RedisDualOptions.Store.Password,
		Database:              dm.config.RedisDualOptions.Store.Database,
		MaxIdle:               dm.config.RedisDualOptions.Store.MaxIdle,
		MaxActive:             dm.config.RedisDualOptions.Store.MaxActive,
		Timeout:               dm.config.RedisDualOptions.Store.Timeout,
		EnableCluster:         dm.config.RedisDualOptions.Store.EnableCluster,
		UseSSL:                dm.config.RedisDualOptions.Store.UseSSL,
		SSLInsecureSkipVerify: false,
	}

	if storeConfig.Host == "" {
		log.Info("Store Redis host not configured, skipping Store Redis initialization")
	} else {
		storeConn := database.NewRedisConnection(storeConfig)
		if err := storeConn.Connect(); err != nil {
			return fmt.Errorf("failed to connect store redis (%s:%d): %w", storeConfig.Host, storeConfig.Port, err)
		}
		dm.storeRedis = storeConn
		log.Infof("Store Redis connected successfully to %s:%d (not registered in registry)", storeConfig.Host, storeConfig.Port)
		_ = storeConn // 暂时不注册到 registry，后续如需复用可扩展注册机制
	}

	return nil
}

// GetRedisClient 获取Redis客户端
func (dm *DatabaseManager) GetRedisClient() (redis.UniversalClient, error) {
	client, err := dm.registry.GetClient(database.Redis)
	if err != nil {
		return nil, err
	}

	redisClient, ok := client.(redis.UniversalClient)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to redis.UniversalClient")
	}

	return redisClient, nil
}

// GetStoreRedisClient 获取 Store Redis 客户端（未注册到 registry）
func (dm *DatabaseManager) GetStoreRedisClient() (redis.UniversalClient, error) {
	if dm.storeRedis == nil {
		return nil, fmt.Errorf("store redis not initialized")
	}
	return dm.storeRedis.GetClient().(redis.UniversalClient), nil
}

// HealthCheck 数据库健康检查
func (dm *DatabaseManager) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return dm.registry.HealthCheck(ctx)
}

// Close 关闭所有数据库连接
func (dm *DatabaseManager) Close() error {
	log.Info("Closing database connections...")
	return dm.registry.Close()
}

// GetRegistry 获取数据库注册器（用于测试和调试）
func (dm *DatabaseManager) GetRegistry() *database.Registry {
	return dm.registry
}
