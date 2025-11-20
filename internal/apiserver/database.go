package apiserver

import (
	"context"
	"fmt"
	"time"

	redis "github.com/go-redis/redis/v7"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
)

// DatabaseManager 数据库管理器
type DatabaseManager struct {
	registry *database.Registry
	config   *config.Config
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

	// 初始化MySQL连接
	if err := dm.initMySQL(); err != nil {
		return fmt.Errorf("failed to initialize MySQL: %w", err)
	}

	// 初始化Redis连接
	if err := dm.initRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// 初始化MongoDB连接
	if err := dm.initMongoDB(); err != nil {
		return fmt.Errorf("failed to initialize MongoDB: %w", err)
	}

	// 初始化数据库连接
	if err := dm.registry.Init(); err != nil {
		return fmt.Errorf("failed to initialize database connections: %w", err)
	}

	// 执行数据库迁移
	if err := dm.runMigrations(); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	log.Info("All database connections initialized successfully")
	return nil
}

// initMySQL 初始化MySQL连接
func (dm *DatabaseManager) initMySQL() error {
	mysqlConfig := &database.MySQLConfig{
		Host:                  dm.config.MySQLOptions.Host,
		Username:              dm.config.MySQLOptions.Username,
		Password:              dm.config.MySQLOptions.Password,
		Database:              dm.config.MySQLOptions.Database,
		MaxIdleConnections:    dm.config.MySQLOptions.MaxIdleConnections,
		MaxOpenConnections:    dm.config.MySQLOptions.MaxOpenConnections,
		MaxConnectionLifeTime: dm.config.MySQLOptions.MaxConnectionLifeTime,
		LogLevel:              dm.config.MySQLOptions.LogLevel,
		Logger:                logger.New(dm.config.MySQLOptions.LogLevel),
	}

	if mysqlConfig.Host == "" {
		log.Info("MySQL host not configured, skipping MySQL initialization")
		return nil
	}

	mysqlConn := database.NewMySQLConnection(mysqlConfig)
	return dm.registry.Register(database.MySQL, mysqlConfig, mysqlConn)
}

// initRedis 初始化Redis连接（双实例架构）
func (dm *DatabaseManager) initRedis() error {
	// 初始化 Cache Redis
	cacheConfig := &database.RedisConfig{
		Host:                  dm.config.RedisDualOptions.Cache.Host,
		Port:                  dm.config.RedisDualOptions.Cache.Port,
		Addrs:                 []string{}, // 双实例模式暂不支持集群
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
		log.Info("Cache Redis initialized successfully")
	}

	// 初始化 Store Redis
	storeConfig := &database.RedisConfig{
		Host:                  dm.config.RedisDualOptions.Store.Host,
		Port:                  dm.config.RedisDualOptions.Store.Port,
		Addrs:                 []string{},
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
		// 注册为 RedisStore 实例（如果需要单独访问）
		// 这里可以扩展 database.Type 来支持多个 Redis 实例
		// 暂时只注册一个主实例，Store 实例需要业务代码直接创建连接
		log.Info("Store Redis initialized successfully (not registered in registry)")
		_ = storeConn // 暂时不使用，未来可以扩展注册机制
	}

	return nil
}

// initMongoDB 初始化MongoDB连接
func (dm *DatabaseManager) initMongoDB() error {
	mongoConfig := &database.MongoConfig{
		URL:                      dm.config.MongoDBOptions.URL,
		UseSSL:                   dm.config.MongoDBOptions.UseSSL,
		SSLInsecureSkipVerify:    dm.config.MongoDBOptions.SSLInsecureSkipVerify,
		SSLAllowInvalidHostnames: dm.config.MongoDBOptions.SSLAllowInvalidHostnames,
		SSLCAFile:                dm.config.MongoDBOptions.SSLCAFile,
		SSLPEMKeyfile:            dm.config.MongoDBOptions.SSLPEMKeyfile,
	}

	if mongoConfig.URL == "" {
		log.Info("MongoDB URL not configured, skipping MongoDB initialization")
		return nil
	}

	mongoConn := database.NewMongoDBConnection(mongoConfig)
	return dm.registry.Register(database.MongoDB, mongoConfig, mongoConn)
}

// GetMySQLDB 获取MySQL数据库连接
func (dm *DatabaseManager) GetMySQLDB() (*gorm.DB, error) {
	client, err := dm.registry.GetClient(database.MySQL)
	if err != nil {
		return nil, err
	}

	db, ok := client.(*gorm.DB)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to *gorm.DB")
	}

	return db, nil
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

// GetMongoClient 获取MongoDB客户端
func (dm *DatabaseManager) GetMongoClient() (*mongo.Client, error) {
	client, err := dm.registry.GetClient(database.MongoDB)
	if err != nil {
		return nil, err
	}

	mongoClient, ok := client.(*mongo.Client)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to *mongo.Client")
	}

	return mongoClient, nil
}

// GetMongoSession 获取 MongoDB 会话 (兼容 mgo 接口)
// TODO: 这是一个临时的兼容方法，实际项目中应该统一使用现代的 MongoDB 驱动
func (dm *DatabaseManager) GetMongoSession() (interface{}, error) {
	// 这里返回一个模拟的 session，实际使用时需要实现 mgo 兼容层
	// 或者重构适配器使用现代的 MongoDB 驱动
	return nil, fmt.Errorf("mgo session compatibility not implemented - please use GetMongoClient() instead")
}

// GetMongoDB 获取 MongoDB 数据库
func (dm *DatabaseManager) GetMongoDB() (*mongo.Database, error) {
	// 使用默认数据库名，后续可以从配置中读取
	client, err := dm.registry.GetClient(database.MongoDB)
	if err != nil {
		return nil, err
	}

	mongoClient, ok := client.(*mongo.Client)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to *mongo.Client")
	}

	return mongoClient.Database(viper.GetString("mongodb.database")), nil
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

// runMigrations 执行数据库迁移
func (dm *DatabaseManager) runMigrations() error {
	// 检查是否启用迁移
	if !dm.config.MigrationOptions.Enabled {
		log.Info("Database migration is disabled, skipping...")
		return nil
	}

	// 获取 MySQL 连接
	gormDB, err := dm.GetMySQLDB()
	if err != nil {
		log.Warn("MySQL not configured, skipping migration")
		return nil
	}

	// 获取底层的 *sql.DB
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// 使用配置中的数据库名，如果未配置则使用 MySQL 配置中的数据库名
	database := dm.config.MigrationOptions.Database
	if database == "" {
		database = dm.config.MySQLOptions.Database
	}

	// 创建迁移配置
	migrationConfig := &migration.Config{
		Enabled:  dm.config.MigrationOptions.Enabled,
		AutoSeed: dm.config.MigrationOptions.AutoSeed,
		Database: database,
	}

	// 创建迁移器
	migrator := migration.NewMigrator(sqlDB, migrationConfig)

	// 执行迁移
	log.Info("Starting database migration...")
	version, migrated, err := migrator.Run()
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	if migrated {
		log.Infof("✅ Database migration completed successfully! Current version: %d", version)
	} else {
		log.Infof("✅ Database is already up to date! Current version: %d", version)
	}

	return nil
}
