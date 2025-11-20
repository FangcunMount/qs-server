package migration

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// migrations 嵌入迁移文件
// 这样打包后的二进制文件中就包含了迁移 SQL，无需挂载外部文件
//
//go:embed migrations/*.sql
var migrations embed.FS

// Config 迁移配置
type Config struct {
	Enabled  bool   // 是否启用自动迁移
	AutoSeed bool   // 是否自动加载种子数据
	Database string // 数据库名称
}

// Migrator 数据库迁移器
type Migrator struct {
	db     *sql.DB
	config *Config
}

// NewMigrator 创建迁移器
func NewMigrator(db *sql.DB, config *Config) *Migrator {
	return &Migrator{
		db:     db,
		config: config,
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

	// 创建 migrate 实例
	instance, err := m.createMigrate()
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
	instance, err := m.createMigrate()
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
	instance, err := m.createMigrate()
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

// createMigrate 创建 migrate 实例
func (m *Migrator) createMigrate() (*migrate.Migrate, error) {
	// 1. 从嵌入文件系统创建源驱动
	sourceDriver, err := iofs.New(migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to create source driver: %w", err)
	}

	// 2. 创建 MySQL 数据库驱动
	databaseDriver, err := mysql.WithInstance(m.db, &mysql.Config{
		DatabaseName: m.config.Database,
		// 迁移表名，用于记录版本
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create database driver: %w", err)
	}

	// 3. 创建 migrate 实例
	instance, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		"mysql",
		databaseDriver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return instance, nil
}
