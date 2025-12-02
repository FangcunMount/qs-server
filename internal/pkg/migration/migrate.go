package migration

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"go.mongodb.org/mongo-driver/mongo"
)

// migrations 嵌入迁移文件
// 这样打包后的二进制文件中就包含了迁移 SQL，无需挂载外部文件
//
//go:embed migrations/mysql/* migrations/mongodb/*
var migrations embed.FS

const (
	defaultTable = "schema_migrations"
)

// Config 迁移配置
type Config struct {
	Enabled              bool   // 是否启用自动迁移
	AutoSeed             bool   // 是否自动加载种子数据
	Database             string // 数据库名称
	MigrationsTable      string // MySQL 迁移记录表名
	MigrationsCollection string // MongoDB 迁移记录集合名
}

// Migrator 数据库迁移器
type Migrator struct {
	driver Driver
	config *Config
}

// NewMigrator 创建 MySQL 迁移器（保持向后兼容）
func NewMigrator(db *sql.DB, config *Config) *Migrator {
	return &Migrator{
		driver: NewMySQLDriver(db),
		config: ensureConfigDefaults(config),
	}
}

// NewMongoMigrator 创建 MongoDB 迁移器（保持向后兼容）
func NewMongoMigrator(client *mongo.Client, config *Config) *Migrator {
	return &Migrator{
		driver: NewMongoDriver(client),
		config: ensureConfigDefaults(config),
	}
}

// NewMigratorWithDriver 使用自定义驱动创建迁移器
// 这是推荐的创建方式，支持任意实现 Driver 接口的数据库
func NewMigratorWithDriver(driver Driver, config *Config) *Migrator {
	return &Migrator{
		driver: driver,
		config: ensureConfigDefaults(config),
	}
}

// Backend 返回当前使用的后端类型
func (m *Migrator) Backend() Backend {
	return m.driver.Backend()
}

// Run 执行数据库迁移并返回最新版本以及是否执行了迁移
//
// 工作流程:
// 1. 检查是否启用迁移
// 2. 创建 migrate 实例
// 3. 获取当前版本
// 4. 执行迁移到最新版本
// 5. 返回最新版本及是否执行了迁移
func (m *Migrator) Run() (uint, bool, error) {
	if !m.config.Enabled {
		return 0, false, nil
	}

	if err := m.validate(); err != nil {
		return 0, false, err
	}

	// 创建 migrate 实例
	instance, err := m.driver.CreateInstance(migrations, m.config)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		_, _ = instance.Close()
	}()

	// 获取当前版本
	currentVersion, dirty, err := instance.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return 0, false, fmt.Errorf("failed to get current version: %w", err)
	}

	var versionBefore uint
	if err == migrate.ErrNilVersion {
		versionBefore = 0
	} else {
		versionBefore = currentVersion
	}

	if dirty {
		return versionBefore, false, fmt.Errorf("database is in dirty state at version %d, please fix manually", versionBefore)
	}

	// 执行迁移
	if err := instance.Up(); err != nil {
		if err == migrate.ErrNoChange {
			// 数据库已是最新版本
			return versionBefore, false, nil
		}
		return versionBefore, false, fmt.Errorf("migration failed: %w", err)
	}

	// 获取新版本
	newVersion, _, verr := instance.Version()
	if verr != nil {
		return versionBefore, true, fmt.Errorf("failed to get new version: %w", verr)
	}

	return newVersion, true, nil
}

// Rollback 回滚最近的一次迁移
func (m *Migrator) Rollback() error {
	if err := m.validate(); err != nil {
		return err
	}

	instance, err := m.driver.CreateInstance(migrations, m.config)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = instance.Close()
	}()

	if err := instance.Steps(-1); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	return nil
}

// Version 获取当前数据库版本
func (m *Migrator) Version() (uint, bool, error) {
	if err := m.validate(); err != nil {
		return 0, false, err
	}

	instance, err := m.driver.CreateInstance(migrations, m.config)
	if err != nil {
		return 0, false, err
	}
	defer func() {
		_, _ = instance.Close()
	}()

	version, dirty, err := instance.Version()
	if err != nil {
		return 0, false, err
	}

	return version, dirty, nil
}

// validate 验证迁移器配置
func (m *Migrator) validate() error {
	if m.driver == nil {
		return fmt.Errorf("migration driver is nil")
	}
	if m.config == nil {
		return fmt.Errorf("migration config is nil")
	}
	if m.config.Database == "" {
		return fmt.Errorf("database name is required for migration")
	}
	return nil
}

func ensureConfigDefaults(cfg *Config) *Config {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.MigrationsTable == "" {
		cfg.MigrationsTable = defaultTable
	}
	if cfg.MigrationsCollection == "" {
		cfg.MigrationsCollection = defaultTable
	}
	return cfg
}
