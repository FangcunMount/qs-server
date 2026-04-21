package collection

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	redis "github.com/redis/go-redis/v9"
)

// DatabaseManager manages collection-server Redis connectivity.
type DatabaseManager struct {
	config        *config.Config
	redisProfiles *database.NamedRedisRegistry
}

func NewDatabaseManager(cfg *config.Config) *DatabaseManager {
	return &DatabaseManager{config: cfg}
}

func (m *DatabaseManager) Initialize() error {
	log.Info("Initializing collection-server database connections...")

	if err := m.initRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	log.Info("Collection-server database connections initialized successfully")
	return nil
}

func (m *DatabaseManager) initRedis() error {
	defaultConfig := toDatabaseRedisConfig(m.config.RedisOptions)
	redisProfiles := make(map[string]*database.RedisConfig)
	for name, cfg := range m.config.RedisProfiles {
		databaseCfg := mergeDatabaseRedisConfig(defaultConfig, toDatabaseRedisConfig(cfg))
		if databaseCfg == nil || (databaseCfg.Host == "" && len(databaseCfg.Addrs) == 0) {
			continue
		}
		redisProfiles[name] = databaseCfg
	}

	if defaultConfig == nil && len(redisProfiles) == 0 {
		log.Warn("collection-server Redis not configured, running without Redis runtime")
		return nil
	}

	m.redisProfiles = database.NewNamedRedisRegistry(defaultConfig, redisProfiles)
	if err := m.redisProfiles.Connect(); err != nil {
		return err
	}

	log.Infof("collection-server Redis initialized successfully (profile_count=%d)", len(redisProfiles))
	return nil
}

func (m *DatabaseManager) GetRedisClient() (redis.UniversalClient, error) {
	return m.GetRedisClientByProfile("")
}

func (m *DatabaseManager) GetRedisClientByProfile(profile string) (redis.UniversalClient, error) {
	if m.redisProfiles == nil {
		return nil, fmt.Errorf("redis is not configured")
	}
	return m.redisProfiles.GetClient(profile)
}

func (m *DatabaseManager) GetRedisProfileStatus(profile string) database.RedisProfileStatus {
	if m == nil || m.redisProfiles == nil {
		return database.RedisProfileStatus{Name: profile, State: database.RedisProfileStateMissing}
	}
	return m.redisProfiles.ProfileStatus(profile)
}

func (m *DatabaseManager) HealthCheck() error {
	if m.redisProfiles == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.redisProfiles.HealthCheck(ctx)
}

func (m *DatabaseManager) Close() error {
	if m.redisProfiles == nil {
		return nil
	}
	return m.redisProfiles.Close()
}

func toDatabaseRedisConfig(opts *genericoptions.RedisOptions) *database.RedisConfig {
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
