package migration

import (
	"context"
	"database/sql"
	"embed"
	"errors"
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

type runPreparer interface {
	PrepareRun(context.Context, *Config, uint) (func(context.Context) error, error)
}

// NewMigrator 创建 MySQL 迁移器。
func NewMigrator(db *sql.DB, config *Config) *Migrator {
	return &Migrator{
		driver: NewMySQLDriver(db),
		config: ensureConfigDefaults(config),
	}
}

// NewMongoMigrator 创建 MongoDB 迁移器。
func NewMongoMigrator(client *mongo.Client, config *Config) *Migrator {
	return &Migrator{
		driver: NewMongoDriver(client),
		config: ensureConfigDefaults(config),
	}
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
	// 注意：不要关闭 migrate 实例，因为它会关闭我们传入的数据库连接
	// migrate 实例内部的 source driver 会在进程结束时自动清理

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

	cleanup := func(context.Context) error { return nil }
	if preparer, ok := m.driver.(runPreparer); ok {
		cleanup, err = preparer.PrepareRun(context.Background(), m.config, versionBefore)
		if err != nil {
			return versionBefore, false, fmt.Errorf("prepare migration run: %w", err)
		}
	}

	// 执行迁移
	upErr := instance.Up()
	cleanupErr := cleanup(context.Background())
	if upErr != nil {
		if errors.Is(upErr, migrate.ErrNoChange) {
			if cleanupErr != nil {
				return versionBefore, false, fmt.Errorf("cleanup migration run: %w", cleanupErr)
			}
			// 数据库已是最新版本
			return versionBefore, false, nil
		}
		if cleanupErr != nil {
			return versionBefore, false, fmt.Errorf("migration failed: %w (cleanup failed: %v)", upErr, cleanupErr)
		}
		return versionBefore, false, fmt.Errorf("migration failed: %w", upErr)
	}
	if cleanupErr != nil {
		return versionBefore, true, fmt.Errorf("cleanup migration run: %w", cleanupErr)
	}

	// 获取新版本
	newVersion, _, verr := instance.Version()
	if verr != nil {
		return versionBefore, true, fmt.Errorf("failed to get new version: %w", verr)
	}

	return newVersion, true, nil
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
