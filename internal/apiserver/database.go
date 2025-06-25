package apiserver

import (
	"context"
	"fmt"
	"time"

	redis "github.com/go-redis/redis/v7"
	"github.com/vinllen/mgo"
	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/config"
	"github.com/yshujie/questionnaire-scale/pkg/database"
	"github.com/yshujie/questionnaire-scale/pkg/database/databases"
	"github.com/yshujie/questionnaire-scale/pkg/log"
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

	log.Info("All database connections initialized successfully")
	return nil
}

// initMySQL 初始化MySQL连接
func (dm *DatabaseManager) initMySQL() error {
	mysqlConfig := &databases.MySQLConfig{
		Host:                  dm.config.MySQLOptions.Host,
		Username:              dm.config.MySQLOptions.Username,
		Password:              dm.config.MySQLOptions.Password,
		Database:              dm.config.MySQLOptions.Database,
		MaxIdleConnections:    dm.config.MySQLOptions.MaxIdleConnections,
		MaxOpenConnections:    dm.config.MySQLOptions.MaxOpenConnections,
		MaxConnectionLifeTime: dm.config.MySQLOptions.MaxConnectionLifeTime,
		LogLevel:              dm.config.MySQLOptions.LogLevel,
	}

	if mysqlConfig.Host == "" {
		log.Info("MySQL host not configured, skipping MySQL initialization")
		return nil
	}

	mysqlConn := databases.NewMySQLConnection(mysqlConfig)
	return dm.registry.Register(databases.MySQL, mysqlConfig, mysqlConn)
}

// initRedis 初始化Redis连接
func (dm *DatabaseManager) initRedis() error {
	redisConfig := &databases.RedisConfig{
		Host:                  dm.config.RedisOptions.Host,
		Port:                  dm.config.RedisOptions.Port,
		Addrs:                 dm.config.RedisOptions.Addrs,
		Password:              dm.config.RedisOptions.Password,
		Database:              dm.config.RedisOptions.Database,
		MaxIdle:               dm.config.RedisOptions.MaxIdle,
		MaxActive:             dm.config.RedisOptions.MaxActive,
		Timeout:               dm.config.RedisOptions.Timeout,
		EnableCluster:         dm.config.RedisOptions.EnableCluster,
		UseSSL:                dm.config.RedisOptions.UseSSL,
		SSLInsecureSkipVerify: dm.config.RedisOptions.SSLInsecureSkipVerify,
	}

	if redisConfig.Host == "" && len(redisConfig.Addrs) == 0 {
		log.Info("Redis host not configured, skipping Redis initialization")
		return nil
	}

	redisConn := databases.NewRedisConnection(redisConfig)
	return dm.registry.Register(databases.Redis, redisConfig, redisConn)
}

// initMongoDB 初始化MongoDB连接
func (dm *DatabaseManager) initMongoDB() error {
	mongoConfig := &databases.MongoConfig{
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

	mongoConn := databases.NewMongoDBConnection(mongoConfig)
	return dm.registry.Register(databases.MongoDB, mongoConfig, mongoConn)
}

// GetMySQLDB 获取MySQL数据库连接
func (dm *DatabaseManager) GetMySQLDB() (*gorm.DB, error) {
	client, err := dm.registry.GetClient(databases.MySQL)
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
	client, err := dm.registry.GetClient(databases.Redis)
	if err != nil {
		return nil, err
	}

	redisClient, ok := client.(redis.UniversalClient)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to redis.UniversalClient")
	}

	return redisClient, nil
}

// GetMongoSession 获取MongoDB会话
func (dm *DatabaseManager) GetMongoSession() (*mgo.Session, error) {
	client, err := dm.registry.GetClient(databases.MongoDB)
	if err != nil {
		return nil, err
	}

	session, ok := client.(*mgo.Session)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to *mgo.Session")
	}

	return session, nil
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

// GetRegistry 获取数据库注册器
func (dm *DatabaseManager) GetRegistry() *database.Registry {
	return dm.registry
}
