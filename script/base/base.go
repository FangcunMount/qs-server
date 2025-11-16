package base

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gorm.io/gorm"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/pkg/database"
	"github.com/FangcunMount/qs-server/pkg/database/databases"
	"github.com/FangcunMount/qs-server/pkg/log"
)

// ScriptEnv 脚本环境
type ScriptEnv struct {
	Config   *options.Options
	Database *database.Registry
	MySQL    *gorm.DB
}

// InitOptions 初始化选项
type InitOptions struct {
	ConfigFile  string
	EnableMySQL bool
	EnableRedis bool
	EnableMongo bool
	LogLevel    string
	ScriptName  string
}

// DefaultInitOptions 返回默认初始化选项
func DefaultInitOptions() *InitOptions {
	return &InitOptions{
		ConfigFile:  "",
		EnableMySQL: true,
		EnableRedis: false,
		EnableMongo: false,
		LogLevel:    "info",
		ScriptName:  "script",
	}
}

// NewScriptEnv 创建脚本环境
func NewScriptEnv(opts *InitOptions) (*ScriptEnv, error) {
	if opts == nil {
		opts = DefaultInitOptions()
	}

	env := &ScriptEnv{
		Database: database.NewRegistry(),
	}

	// 1. 查找配置文件
	configFile, err := findConfigFile(opts.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("查找配置文件失败: %w", err)
	}

	// 2. 初始化日志
	if err := initLogger(opts.LogLevel, opts.ScriptName); err != nil {
		return nil, fmt.Errorf("初始化日志失败: %w", err)
	}

	// 3. 加载配置
	config, err := loadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	env.Config = config

	// 4. 初始化数据库连接
	if err := env.initDatabases(opts); err != nil {
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}

	log.Infof("✅ 脚本环境初始化完成 - %s", opts.ScriptName)
	return env, nil
}

// findConfigFile 查找配置文件
func findConfigFile(configFile string) (string, error) {
	if configFile != "" {
		// 检查指定的配置文件是否存在
		if _, err := os.Stat(configFile); err != nil {
			return "", fmt.Errorf("配置文件不存在: %s", configFile)
		}
		return configFile, nil
	}

	// 自动查找配置文件
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("获取工作目录失败: %w", err)
	}

	// 向上查找项目根目录中的配置文件
	for {
		configPath := filepath.Join(wd, "configs", "apiserver.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parent := filepath.Dir(wd)
		if parent == wd {
			return "", fmt.Errorf("无法找到配置文件 configs/apiserver.yaml，请指定配置文件路径")
		}
		wd = parent
	}
}

// initLogger 初始化日志
func initLogger(level, scriptName string) error {
	opts := log.NewOptions()
	opts.Level = level
	opts.Name = scriptName
	opts.Format = "console"
	opts.EnableColor = true
	opts.OutputPaths = []string{"stdout"}
	opts.ErrorOutputPaths = []string{"stderr"}

	log.Init(opts)
	return nil
}

// loadConfig 加载配置文件
func loadConfig(configFile string) (*options.Options, error) {
	// 设置 viper 配置
	viper.SetConfigFile(configFile)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析到配置对象
	opts := options.NewOptions()
	if err := viper.Unmarshal(opts); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	log.Infof("✅ 配置文件加载成功: %s", configFile)
	return opts, nil
}

// initDatabases 初始化数据库连接
func (env *ScriptEnv) initDatabases(opts *InitOptions) error {
	// 初始化 MySQL
	if opts.EnableMySQL {
		if err := env.initMySQL(); err != nil {
			return fmt.Errorf("MySQL 初始化失败: %w", err)
		}
	}

	// 初始化 Redis
	if opts.EnableRedis {
		if err := env.initRedis(); err != nil {
			return fmt.Errorf("Redis 初始化失败: %w", err)
		}
	}

	// 初始化 MongoDB
	if opts.EnableMongo {
		if err := env.initMongoDB(); err != nil {
			return fmt.Errorf("MongoDB 初始化失败: %w", err)
		}
	}

	// 初始化所有数据库连接
	if err := env.Database.Init(); err != nil {
		return fmt.Errorf("数据库连接初始化失败: %w", err)
	}

	return nil
}

// initMySQL 初始化 MySQL 连接
func (env *ScriptEnv) initMySQL() error {
	mysqlConfig := &databases.MySQLConfig{
		Host:                  env.Config.MySQLOptions.Host,
		Username:              env.Config.MySQLOptions.Username,
		Password:              env.Config.MySQLOptions.Password,
		Database:              env.Config.MySQLOptions.Database,
		MaxIdleConnections:    env.Config.MySQLOptions.MaxIdleConnections,
		MaxOpenConnections:    env.Config.MySQLOptions.MaxOpenConnections,
		MaxConnectionLifeTime: env.Config.MySQLOptions.MaxConnectionLifeTime,
		LogLevel:              env.Config.MySQLOptions.LogLevel,
	}

	if mysqlConfig.Host == "" {
		log.Warn("MySQL 配置为空，跳过 MySQL 初始化")
		return nil
	}

	mysqlConn := databases.NewMySQLConnection(mysqlConfig)
	if err := env.Database.Register(databases.MySQL, mysqlConfig, mysqlConn); err != nil {
		return err
	}

	log.Info("✅ MySQL 连接注册成功")
	return nil
}

// initRedis 初始化 Redis 连接
func (env *ScriptEnv) initRedis() error {
	redisConfig := &databases.RedisConfig{
		Host:                  env.Config.RedisOptions.Host,
		Port:                  env.Config.RedisOptions.Port,
		Addrs:                 env.Config.RedisOptions.Addrs,
		Password:              env.Config.RedisOptions.Password,
		Database:              env.Config.RedisOptions.Database,
		MaxIdle:               env.Config.RedisOptions.MaxIdle,
		MaxActive:             env.Config.RedisOptions.MaxActive,
		Timeout:               env.Config.RedisOptions.Timeout,
		EnableCluster:         env.Config.RedisOptions.EnableCluster,
		UseSSL:                env.Config.RedisOptions.UseSSL,
		SSLInsecureSkipVerify: env.Config.RedisOptions.SSLInsecureSkipVerify,
	}

	if redisConfig.Host == "" && len(redisConfig.Addrs) == 0 {
		log.Warn("Redis 配置为空，跳过 Redis 初始化")
		return nil
	}

	redisConn := databases.NewRedisConnection(redisConfig)
	if err := env.Database.Register(databases.Redis, redisConfig, redisConn); err != nil {
		return err
	}

	log.Info("✅ Redis 连接注册成功")
	return nil
}

// initMongoDB 初始化 MongoDB 连接
func (env *ScriptEnv) initMongoDB() error {
	mongoConfig := &databases.MongoConfig{
		URL:                      env.Config.MongoDBOptions.URL,
		UseSSL:                   env.Config.MongoDBOptions.UseSSL,
		SSLInsecureSkipVerify:    env.Config.MongoDBOptions.SSLInsecureSkipVerify,
		SSLAllowInvalidHostnames: env.Config.MongoDBOptions.SSLAllowInvalidHostnames,
		SSLCAFile:                env.Config.MongoDBOptions.SSLCAFile,
		SSLPEMKeyfile:            env.Config.MongoDBOptions.SSLPEMKeyfile,
	}

	if mongoConfig.URL == "" {
		log.Warn("MongoDB 配置为空，跳过 MongoDB 初始化")
		return nil
	}

	mongoConn := databases.NewMongoDBConnection(mongoConfig)
	if err := env.Database.Register(databases.MongoDB, mongoConfig, mongoConn); err != nil {
		return err
	}

	log.Info("✅ MongoDB 连接注册成功")
	return nil
}

// GetMySQLDB 获取 MySQL 数据库连接
func (env *ScriptEnv) GetMySQLDB() (*gorm.DB, error) {
	if env.MySQL != nil {
		return env.MySQL, nil
	}

	client, err := env.Database.GetClient(databases.MySQL)
	if err != nil {
		return nil, fmt.Errorf("获取 MySQL 客户端失败: %w", err)
	}

	db, ok := client.(*gorm.DB)
	if !ok {
		return nil, fmt.Errorf("MySQL 客户端类型断言失败")
	}

	env.MySQL = db
	return db, nil
}

// Close 关闭所有数据库连接
func (env *ScriptEnv) Close() error {
	if env.Database != nil {
		if err := env.Database.Close(); err != nil {
			log.Errorf("关闭数据库连接失败: %v", err)
			return err
		}
		log.Info("✅ 数据库连接已关闭")
	}
	return nil
}

// PrintSummary 打印环境信息摘要
func (env *ScriptEnv) PrintSummary() {
	log.Info("📋 环境信息摘要:")
	if env.Config != nil {
		log.Infof("  MySQL: %s", env.Config.MySQLOptions.Host)
		log.Infof("  Redis: %s:%d", env.Config.RedisOptions.Host, env.Config.RedisOptions.Port)
		log.Infof("  MongoDB: %s", env.Config.MongoDBOptions.URL)
	}

	// 打印已注册的数据库类型
	registeredDbs := env.Database.ListRegistered()
	if len(registeredDbs) > 0 {
		log.Infof("  已注册数据库: %v", registeredDbs)
	} else {
		log.Warn("  未注册任何数据库连接")
	}
}

// ScriptRunner 脚本运行器接口 - 实现模版方法模式
type ScriptRunner interface {
	// Initialize 初始化运行环境
	Initialize() error
	// Execute 执行业务操作
	Execute() error
	// Finalize 执行完毕后的清理操作
	Finalize() error
}

// ScriptTemplate 脚本模版 - 包含通用的环境管理
type ScriptTemplate struct {
	Env        *ScriptEnv
	ScriptName string
	opts       *InitOptions
}

// NewScriptTemplate 创建脚本模版
func NewScriptTemplate(scriptName string, opts *InitOptions) *ScriptTemplate {
	if opts == nil {
		opts = DefaultInitOptions()
	}
	opts.ScriptName = scriptName

	return &ScriptTemplate{
		ScriptName: scriptName,
		opts:       opts,
	}
}

// Run 模版方法 - 按顺序执行初始化、业务操作、清理
func (st *ScriptTemplate) Run(runner ScriptRunner) error {
	log.Infof("🚀 开始运行脚本: %s", st.ScriptName)

	// 1. 初始化运行环境
	log.Info("📋 第一阶段: 初始化运行环境")
	if err := st.initializeEnv(); err != nil {
		return fmt.Errorf("环境初始化失败: %w", err)
	}

	if err := runner.Initialize(); err != nil {
		st.cleanup()
		return fmt.Errorf("脚本初始化失败: %w", err)
	}
	log.Info("✅ 运行环境初始化完成")

	// 2. 执行业务操作
	log.Info("⚙️ 第二阶段: 执行业务操作")
	if err := runner.Execute(); err != nil {
		st.cleanup()
		return fmt.Errorf("业务操作执行失败: %w", err)
	}
	log.Info("✅ 业务操作执行完成")

	// 3. 执行完毕后的清理
	log.Info("🧹 第三阶段: 执行清理操作")
	if err := runner.Finalize(); err != nil {
		log.Errorf("⚠️ 清理操作失败: %v", err)
		// 清理操作失败不返回错误，继续执行环境清理
	} else {
		log.Info("✅ 清理操作完成")
	}

	// 4. 清理环境
	st.cleanup()

	log.Infof("🎉 脚本运行完成: %s", st.ScriptName)
	return nil
}

// initializeEnv 初始化基础环境
func (st *ScriptTemplate) initializeEnv() error {
	env, err := NewScriptEnv(st.opts)
	if err != nil {
		return err
	}
	st.Env = env
	return nil
}

// cleanup 清理环境资源
func (st *ScriptTemplate) cleanup() {
	if st.Env != nil {
		if err := st.Env.Close(); err != nil {
			log.Errorf("⚠️ 环境清理失败: %v", err)
		} else {
			log.Info("✅ 环境清理完成")
		}
	}
}

// GetEnv 获取脚本环境（供具体脚本使用）
func (st *ScriptTemplate) GetEnv() *ScriptEnv {
	return st.Env
}
