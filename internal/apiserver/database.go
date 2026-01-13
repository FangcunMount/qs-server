package apiserver

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
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
func (dm *DatabaseManager) Initialize() error {
	ctx := context.Background()
	logger.L(ctx).Infow("Initializing database connections...",
		"component", "DatabaseManager",
		"action", "initialize",
	)

	// 初始化MySQL连接
	if err := dm.initMySQL(ctx); err != nil {
		return fmt.Errorf("failed to initialize MySQL: %w", err)
	}

	// 初始化Redis连接
	if err := dm.initRedis(ctx); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// 初始化MongoDB连接
	if err := dm.initMongoDB(ctx); err != nil {
		return fmt.Errorf("failed to initialize MongoDB: %w", err)
	}

	// 初始化数据库连接
	if err := dm.registry.Init(); err != nil {
		return fmt.Errorf("failed to initialize database connections: %w", err)
	}

	// 执行数据库迁移
	if err := dm.runMigrations(ctx); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	logger.L(ctx).Infow("All database connections initialized successfully",
		"component", "DatabaseManager",
		"action", "initialize",
		"result", "success",
	)
	return nil
}

// initMySQL 初始化MySQL连接
func (dm *DatabaseManager) initMySQL(ctx context.Context) error {
	mysqlConfig := &database.MySQLConfig{
		Host:                  dm.config.MySQLOptions.Host,
		Username:              dm.config.MySQLOptions.Username,
		Password:              dm.config.MySQLOptions.Password,
		Database:              dm.config.MySQLOptions.Database,
		MaxIdleConnections:    dm.config.MySQLOptions.MaxIdleConnections,
		MaxOpenConnections:    dm.config.MySQLOptions.MaxOpenConnections,
		MaxConnectionLifeTime: dm.config.MySQLOptions.MaxConnectionLifeTime,
		LogLevel:              dm.config.MySQLOptions.LogLevel,
		Logger:                logger.NewGormLogger(dm.config.MySQLOptions.LogLevel),
	}

	if mysqlConfig.Host == "" {
		logger.L(ctx).Infow("MySQL host not configured, skipping MySQL initialization",
			"component", "MySQL",
			"action", "initialize",
			"result", "skipped",
		)
		return nil
	}

	logger.L(ctx).Infow("Initializing MySQL connection",
		"component", "MySQL",
		"action", "initialize",
		"host", mysqlConfig.Host,
		"database", mysqlConfig.Database,
		"max_idle_connections", mysqlConfig.MaxIdleConnections,
		"max_open_connections", mysqlConfig.MaxOpenConnections,
		"max_connection_lifetime", mysqlConfig.MaxConnectionLifeTime.String(),
	)

	mysqlConn := database.NewMySQLConnection(mysqlConfig)
	return dm.registry.Register(database.MySQL, mysqlConfig, mysqlConn)
}

// initRedis 初始化Redis连接
func (dm *DatabaseManager) initRedis(ctx context.Context) error {
	redisConfig := &database.RedisConfig{
		Host:                  dm.config.RedisOptions.Host,
		Port:                  dm.config.RedisOptions.Port,
		Addrs:                 dm.config.RedisOptions.Addrs,
		Username:              dm.config.RedisOptions.Username,
		Password:              dm.config.RedisOptions.Password,
		Database:              dm.config.RedisOptions.Database,
		MaxIdle:               dm.config.RedisOptions.MaxIdle,
		MaxActive:             dm.config.RedisOptions.MaxActive,
		Timeout:               dm.config.RedisOptions.Timeout,
		MinIdleConns:          dm.config.RedisOptions.MinIdleConns,
		PoolTimeout:           dm.config.RedisOptions.PoolTimeout,
		DialTimeout:           dm.config.RedisOptions.DialTimeout,
		ReadTimeout:           dm.config.RedisOptions.ReadTimeout,
		WriteTimeout:          dm.config.RedisOptions.WriteTimeout,
		EnableCluster:         dm.config.RedisOptions.EnableCluster,
		UseSSL:                dm.config.RedisOptions.UseSSL,
		SSLInsecureSkipVerify: dm.config.RedisOptions.SSLInsecureSkipVerify,
	}

	if redisConfig.Host == "" && len(redisConfig.Addrs) == 0 {
		logger.L(ctx).Infow("Redis host not configured, skipping Redis initialization",
			"component", "Redis",
			"action", "initialize",
			"result", "skipped",
		)
		return nil
	}

	cacheConn := database.NewRedisConnection(redisConfig)
	if err := dm.registry.Register(database.Redis, redisConfig, cacheConn); err != nil {
		return fmt.Errorf("failed to register redis: %w", err)
	}
	dm.cacheRedis = cacheConn
	logger.L(ctx).Infow("Redis initialized successfully",
		"component", "Redis",
		"action", "initialize",
		"result", "success",
		"host", redisConfig.Host,
		"port", redisConfig.Port,
		"database", redisConfig.Database,
	)

	return nil
}

// initMongoDB 初始化MongoDB连接
func (dm *DatabaseManager) initMongoDB(ctx context.Context) error {
	// 直接传递分离参数，由 MongoConfig.BuildURL() 构建连接 URL
	mongoConfig := &database.MongoConfig{
		Host:                     dm.config.MongoDBOptions.Host,
		Username:                 dm.config.MongoDBOptions.Username,
		Password:                 dm.config.MongoDBOptions.Password,
		Database:                 dm.config.MongoDBOptions.Database,
		UseSSL:                   dm.config.MongoDBOptions.UseSSL,
		SSLInsecureSkipVerify:    dm.config.MongoDBOptions.SSLInsecureSkipVerify,
		SSLAllowInvalidHostnames: dm.config.MongoDBOptions.SSLAllowInvalidHostnames,
		SSLCAFile:                dm.config.MongoDBOptions.SSLCAFile,
		SSLPEMKeyfile:            dm.config.MongoDBOptions.SSLPEMKeyfile,
		// 日志配置
		EnableLogger:  dm.config.MongoDBOptions.EnableLogger,
		SlowThreshold: dm.config.MongoDBOptions.SlowThreshold,
	}

	if mongoConfig.Host == "" {
		logger.L(ctx).Infow("MongoDB host not configured, skipping MongoDB initialization",
			"component", "MongoDB",
			"action", "initialize",
			"result", "skipped",
		)
		return nil
	}

	mongoConn := database.NewMongoDBConnection(mongoConfig)
	return dm.registry.Register(database.MongoDB, mongoConfig, mongoConn)
}

// GetMySQLDB 获取MySQL数据库连接
func (dm *DatabaseManager) GetMySQLDB() (*gorm.DB, error) {
	ctx := context.Background()
	client, err := dm.registry.GetClient(database.MySQL)
	if err != nil {
		logger.L(ctx).Errorw("Failed to get MySQL client from registry",
			"component", "MySQL",
			"action", "query",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	db, ok := client.(*gorm.DB)
	if !ok {
		return nil, fmt.Errorf("failed to cast client to *gorm.DB")
	}

	// 检查连接状态
	sqlDB, err := db.DB()
	if err != nil {
		logger.L(ctx).Errorw("Failed to get sql.DB from gorm.DB",
			"component", "MySQL",
			"action", "get_sql_db",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		logger.L(ctx).Errorw("MySQL connection ping failed",
			"component", "MySQL",
			"action", "ping",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, fmt.Errorf("mysql ping failed: %w", err)
	}

	stats := sqlDB.Stats()
	logger.L(ctx).Debugw("MySQL connection stats",
		"component", "MySQL",
		"open_connections", stats.OpenConnections,
		"in_use", stats.InUse,
		"idle", stats.Idle,
		"wait_count", stats.WaitCount,
		"max_open_connections", stats.MaxOpenConnections,
	)

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
	ctx := context.Background()
	logger.L(ctx).Infow("Closing database connections...",
		"component", "DatabaseManager",
		"action", "close",
	)
	return dm.registry.Close()
}

// GetRegistry 获取数据库注册器（用于测试和调试）
func (dm *DatabaseManager) GetRegistry() *database.Registry {
	return dm.registry
}

// runMigrations 执行数据库迁移
func (dm *DatabaseManager) runMigrations(ctx context.Context) error {
	// 检查是否启用迁移
	if !dm.config.MigrationOptions.Enabled {
		logger.L(ctx).Infow("Database migration is disabled, skipping...",
			"component", "Migration",
			"result", "skipped",
		)
		return nil
	}

	var ran bool

	// MySQL 迁移
	if gormDB, err := dm.GetMySQLDB(); err != nil {
		logger.L(ctx).Warnw("MySQL not configured, skipping MySQL migration",
			"component", "MySQLMigration",
			"result", "skipped",
		)
	} else {
		sqlDB, derr := gormDB.DB()
		if derr != nil {
			return fmt.Errorf("failed to get sql.DB: %w", derr)
		}

		database := dm.config.MigrationOptions.Database
		if database == "" {
			database = dm.config.MySQLOptions.Database
		}

		migrationConfig := &migration.Config{
			Enabled:  dm.config.MigrationOptions.Enabled,
			AutoSeed: dm.config.MigrationOptions.AutoSeed,
			Database: database,
		}

		migrator := migration.NewMigrator(sqlDB, migrationConfig)

		logger.L(ctx).Infow("Starting MySQL database migration...",
			"component", "MySQLMigration",
			"action", "migrate",
			"database", database,
		)
		version, migrated, merr := migrator.Run()
		if merr != nil {
			return fmt.Errorf("mysql migration failed: %w", merr)
		}

		if migrated {
			logger.L(ctx).Infow("✅ MySQL migration completed successfully",
				"component", "MySQLMigration",
				"action", "migrate",
				"result", "success",
				"version", version,
			)
		} else {
			logger.L(ctx).Infow("✅ MySQL schema is already up to date",
				"component", "MySQLMigration",
				"result", "up_to_date",
				"version", version,
			)
		}
		ran = true
	}

	// MongoDB 迁移（可选）
	mongoDBName := viper.GetString("mongodb.database")
	if mongoClient, err := dm.GetMongoClient(); err != nil {
		logger.L(ctx).Infow("MongoDB not configured or unavailable, skipping migration",
			"component", "MongoDBMigration",
			"result", "skipped",
			"error", err.Error(),
		)
	} else if mongoDBName == "" {
		logger.L(ctx).Warnw("MongoDB database name not configured, skipping MongoDB migration",
			"component", "MongoDBMigration",
			"result", "skipped",
		)
	} else {
		mongoDatabase := dm.config.MigrationOptions.Database
		if mongoDatabase == "" {
			mongoDatabase = mongoDBName
		}

		mongoConfig := &migration.Config{
			Enabled:              dm.config.MigrationOptions.Enabled,
			AutoSeed:             dm.config.MigrationOptions.AutoSeed,
			Database:             mongoDatabase,
			MigrationsCollection: "schema_migrations",
		}

		mongoMigrator := migration.NewMongoMigrator(mongoClient, mongoConfig)
		logger.L(ctx).Infow("Starting MongoDB migration...",
			"component", "MongoDBMigration",
			"action", "migrate",
			"database", mongoDatabase,
		)
		mongoVersion, mongoMigrated, merr := mongoMigrator.Run()
		if merr != nil {
			return fmt.Errorf("mongodb migration failed: %w", merr)
		}

		if mongoMigrated {
			logger.L(ctx).Infow("✅ MongoDB migration completed successfully",
				"component", "MongoDBMigration",
				"action", "migrate",
				"result", "success",
				"version", mongoVersion,
			)
		} else {
			logger.L(ctx).Infow("✅ MongoDB schema is already up to date",
				"component", "MongoDBMigration",
				"result", "up_to_date",
				"version", mongoVersion,
			)
		}
		ran = true
	}

	if !ran {
		logger.L(ctx).Warnw("No database migration target configured, skipping...",
			"component", "Migration",
			"result", "skipped",
		)
	}

	return nil
}
