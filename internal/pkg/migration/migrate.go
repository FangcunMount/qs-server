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
	retirementPair       *PairPreflight
	runContext           context.Context
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
func (m *Migrator) Run() (uint, bool, error) { return m.run() }

// Explicit historical targets are package-internal; normal startup always runs
// the paired preflight and cannot choose a bypass mode.
func (m *Migrator) run(target ...uint) (version uint, changed bool, resultErr error) {
	if len(target) > 1 {
		return 0, false, retirementError("invalid historical migration target count")
	}
	var historicalTarget uint
	if len(target) == 1 {
		historicalTarget = target[0]
		if historicalTarget == 0 {
			return 0, false, retirementError("explicit zero historical boundary refused")
		}
	}
	if !m.config.Enabled {
		return 0, false, nil
	}

	if err := m.validate(); err != nil {
		return 0, false, err
	}
	backend := BackendMySQL
	limit := compatibilitySQLVersion - 1
	if m.driver.SourcePath() == "migrations/mongodb" {
		backend = BackendMongo
		limit = compatibilityMongoVersion - 1
	}
	if historicalTarget > limit {
		return 0, false, retirementError("historical target crosses retirement boundary")
	}
	if historicalTarget == 0 {
		p := m.config.retirementPair
		if p == nil {
			return 0, false, retirementError("paired preflight required before latest migration")
		}
		if m.config.MigrationsTable != defaultTable || m.config.MigrationsCollection != defaultTable {
			return 0, false, retirementError("paired migration namespace override rejected")
		}
		if backend == BackendMySQL {
			d, ok := m.driver.(*MySQLDriver)
			if !ok || (p.sqlConn == nil && d.db != p.sqlDB) || (p.sqlConn != nil && d.borrowedConn != p.sqlConn) || m.config.Database != p.config.MySQLDatabase {
				return 0, false, retirementError("paired mysql connection mismatch")
			}
		} else {
			d, ok := m.driver.(*MongoDriver)
			if !ok || d.client != p.mongo || m.config.Database != p.config.MongoDatabase {
				return 0, false, retirementError("paired mongo connection mismatch")
			}
		}
		if err := p.validateStart(migrationRunContext(m.config), backend); err != nil {
			return 0, false, err
		}
	}
	if historicalTarget == 0 {
		// Registered before the owned-connection finalizer so release errors also
		// invalidate this live pair. No retry can adopt a partial/unknown response.
		defer func() {
			if resultErr != nil {
				m.config.retirementPair.migrationResultFailed()
			}
		}()
	}
	if finalizer, ok := m.driver.(interface{ finishRun() error }); ok {
		defer func() {
			if err := finalizer.finishRun(); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("release run-owned migration connection: %w", err))
			}
			if resultErr == nil && backend == BackendMySQL && m.config.retirementPair != nil && version == compatibilitySQLVersion {
				m.config.retirementPair.sqlFinished()
			}
		}()
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
	if historicalTarget != 0 && versionBefore > limit {
		return versionBefore, false, retirementError("historical runner cannot adopt retirement head")
	}

	cleanup := func(context.Context) error { return nil }
	if preparer, ok := m.driver.(runPreparer); ok {
		cleanup, err = preparer.PrepareRun(migrationRunContext(m.config), m.config, versionBefore)
		if err != nil {
			return versionBefore, false, fmt.Errorf("prepare migration run: %w", err)
		}
	}

	// 执行迁移
	var upErr error
	if historicalTarget == 0 {
		upErr = instance.Up()
	} else {
		upErr = instance.Migrate(historicalTarget)
	}
	cleanupErr := cleanup(migrationRunContext(m.config))
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

func migrationRunContext(c *Config) context.Context {
	if c != nil && c.runContext != nil {
		return c.runContext
	}
	return context.Background()
}
