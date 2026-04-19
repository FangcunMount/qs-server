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
	options "github.com/FangcunMount/qs-server/internal/pkg/options"
)

// DatabaseManager 数据库管理器
type DatabaseManager struct {
	registry      *database.Registry
	config        *config.Config
	redisProfiles *database.NamedRedisRegistry
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
	if dm.redisProfiles != nil && dm.redisProfiles.HasConnections() {
		if err := dm.redisProfiles.Connect(); err != nil {
			return fmt.Errorf("failed to initialize redis profiles: %w", err)
		}
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
	redisConfig := toDatabaseRedisConfig(dm.config.RedisOptions)
	redisProfiles := make(map[string]*database.RedisConfig)
	for name, cfg := range dm.config.RedisProfiles {
		if databaseCfg := mergeDatabaseRedisConfig(redisConfig, toDatabaseRedisConfig(cfg)); databaseCfg != nil && (databaseCfg.Host != "" || len(databaseCfg.Addrs) > 0) {
			redisProfiles[name] = databaseCfg
		}
	}

	if redisConfig == nil && len(redisProfiles) == 0 {
		logger.L(ctx).Infow("Redis host not configured, skipping Redis initialization",
			"component", "Redis",
			"action", "initialize",
			"result", "skipped",
		)
		return nil
	}

	dm.redisProfiles = database.NewNamedRedisRegistry(redisConfig, redisProfiles)
	logger.L(ctx).Infow("Redis initialized successfully",
		"component", "Redis",
		"action", "initialize",
		"result", "success",
		"host", redisHostForLog(redisConfig),
		"port", redisPortForLog(redisConfig),
		"database", redisDatabaseForLog(redisConfig),
		"profile_count", len(redisProfiles),
	)

	return nil
}

// initMongoDB 初始化MongoDB连接
func (dm *DatabaseManager) initMongoDB(ctx context.Context) error {
	if dm.config.MongoDBOptions == nil {
		logger.L(ctx).Infow("MongoDB options not configured, skipping MongoDB initialization",
			"component", "MongoDB",
			"action", "initialize",
			"result", "skipped",
		)
		return nil
	}

	opts := dm.config.MongoDBOptions
	if opts.URL == "" && opts.Host == "" {
		logger.L(ctx).Infow("MongoDB host not configured, skipping MongoDB initialization",
			"component", "MongoDB",
			"action", "initialize",
			"result", "skipped",
		)
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
	return dm.GetRedisClientByProfile("")
}

// GetRedisClientByProfile 获取指定 profile 的 Redis 客户端。
// 未配置的 profile 会回退默认 Redis；已配置但不可用的 profile 会返回错误。
func (dm *DatabaseManager) GetRedisClientByProfile(profile string) (redis.UniversalClient, error) {
	if dm.redisProfiles == nil {
		return nil, fmt.Errorf("redis is not configured")
	}
	return dm.redisProfiles.GetClient(profile)
}

// GetRedisProfileStatus 返回指定 profile 当前的可用性状态。
func (dm *DatabaseManager) GetRedisProfileStatus(profile string) database.RedisProfileStatus {
	if dm == nil || dm.redisProfiles == nil {
		return database.RedisProfileStatus{
			Name:  profile,
			State: database.RedisProfileStateMissing,
		}
	}
	return dm.redisProfiles.ProfileStatus(profile)
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

	if err := dm.registry.HealthCheck(ctx); err != nil {
		return err
	}
	if dm.redisProfiles != nil {
		if err := dm.redisProfiles.HealthCheck(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Close 关闭所有数据库连接
func (dm *DatabaseManager) Close() error {
	ctx := context.Background()
	logger.L(ctx).Infow("Closing database connections...",
		"component", "DatabaseManager",
		"action", "close",
	)
	var lastErr error
	if err := dm.registry.Close(); err != nil {
		lastErr = err
	}
	if dm.redisProfiles != nil {
		if err := dm.redisProfiles.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// GetRegistry 获取数据库注册器（用于测试和调试）
func (dm *DatabaseManager) GetRegistry() *database.Registry {
	return dm.registry
}

func toDatabaseRedisConfig(opts *options.RedisOptions) *database.RedisConfig {
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

func mergeDatabaseRedisConfig(base, override *database.RedisConfig) *database.RedisConfig {
	if override == nil {
		return cloneDatabaseRedisConfig(base)
	}
	if base == nil {
		return cloneDatabaseRedisConfig(override)
	}

	merged := cloneDatabaseRedisConfig(base)
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

	// Redis DB 0 is valid, so always honor the profile DB selection.
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

func cloneDatabaseRedisConfig(cfg *database.RedisConfig) *database.RedisConfig {
	if cfg == nil {
		return nil
	}
	copyCfg := *cfg
	if len(cfg.Addrs) > 0 {
		copyCfg.Addrs = append([]string(nil), cfg.Addrs...)
	}
	return &copyCfg
}

func redisHostForLog(cfg *database.RedisConfig) string {
	if cfg == nil {
		return ""
	}
	if cfg.Host != "" {
		return cfg.Host
	}
	if len(cfg.Addrs) > 0 {
		return cfg.Addrs[0]
	}
	return ""
}

func redisPortForLog(cfg *database.RedisConfig) int {
	if cfg == nil {
		return 0
	}
	return cfg.Port
}

func redisDatabaseForLog(cfg *database.RedisConfig) int {
	if cfg == nil {
		return 0
	}
	return cfg.Database
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
