package common

import (
	"context"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	redis "github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const (
	dsnEnvKey   = "IAM_SEEDER_DSN"
	redisEnvKey = "IAM_SEEDER_REDIS"
)

// ResolveDSN returns the DSN to use, preferring an explicit value and
// falling back to the IAM_SEEDER_DSN environment variable.
func ResolveDSN(explicit string) string {
	if explicit != "" {
		return explicit
	}

	if env := os.Getenv(dsnEnvKey); env != "" {
		return env
	}

	log.Fatalf("mysql dsn is required (use --dsn flag or set %s)", dsnEnvKey)
	return ""
}

// ResolveRedisAddr resolves the redis address from flag or environment.
func ResolveRedisAddr(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv(redisEnvKey); env != "" {
		return env
	}
	return ""
}

// MustOpenGORM opens a GORM MySQL connection and verifies it.
// If verbose is true, SQL logs will be printed to stdout.
func MustOpenGORM(dsn string, verbose bool) *gorm.DB {
	logLevel := logger.Silent
	if verbose {
		logLevel = logger.Info // 打印所有 SQL 日志
	}
	cfg := &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		DisableAutomaticPing: false,
	}

	db, err := gorm.Open(mysql.Open(dsn), cfg)
	if err != nil {
		log.Fatalf("failed to open gorm connection: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get sql DB from gorm: %v", err)
	}

	sqlDB.SetConnMaxIdleTime(30 * time.Second)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)

	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("failed to ping mysql: %v", err)
	}

	return db
}

// CloseGORM closes the underlying sql.DB of a gorm instance.
func CloseGORM(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}

// MustOpenRedis creates a Redis client if address is provided.
// For Aliyun Redis, password format can be:
// - Simple password: "your_password"
// - Instance ID format: "instance_id:password" (for Aliyun Redis cluster)
// - Username format: "username:password" (for Redis 6+ ACL)
//
// Deprecated: Use MustOpenRedisWithAuth instead for better control.
func MustOpenRedis(addr string, password ...string) *redis.Client {
	return MustOpenRedisWithAuth(addr, "", password...)
}

// MustOpenRedisWithAuth creates a Redis client with username and password.
// For Aliyun Redis ACL:
// - username: Redis 6.0+ ACL username (e.g., "iam_user"), leave empty for default user
// - password: Can be simple password or "instanceId:password" for Aliyun cluster
func MustOpenRedisWithAuth(addr string, username string, password ...string) *redis.Client {
	if addr == "" {
		return nil
	}

	pwd := ""
	if len(password) > 0 {
		pwd = password[0]
	}

	opts := &redis.Options{
		Addr:         addr,
		Username:     username,
		Password:     pwd,
		DB:           0, // default DB
		DialTimeout:  10 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		PoolTimeout:  30 * time.Second,
		MinIdleConns: 10,
		MaxRetries:   3,
	}

	// Enable debug logging for connection issues
	if username != "" {
		log.Printf("Connecting to Redis at %s with username %s", addr, username)
	} else {
		log.Printf("Connecting to Redis at %s (no username, using default)", addr)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to ping redis at %s: %v", addr, err)
	}

	log.Printf("✅ Successfully connected to Redis at %s", addr)
	return client
}
