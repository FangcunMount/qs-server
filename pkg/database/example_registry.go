// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"
	"log"
	"time"

	dbs "github.com/yshujie/questionnaire-scale/pkg/database/databases"

	redis "github.com/go-redis/redis/v7"
	"github.com/vinllen/mgo"
	"gorm.io/gorm"
)

// ExampleRegistryUsage 展示如何使用注册器模式
func ExampleRegistryUsage() {
	// 1. 创建注册器
	registry := NewRegistry()

	// 2. 根据需要注册数据库连接
	// 注册 MySQL
	mysqlConfig := &dbs.MySQLConfig{
		Host:                  "127.0.0.1:3306",
		Username:              "iam",
		Password:              "iam59!z$",
		Database:              "iam",
		MaxIdleConnections:    100,
		MaxOpenConnections:    100,
		MaxConnectionLifeTime: 10 * time.Second,
		LogLevel:              4,
	}
	mysqlConn := dbs.NewMySQLConnection(mysqlConfig)
	if err := registry.Register(dbs.MySQL, mysqlConfig, mysqlConn); err != nil {
		log.Fatalf("Failed to register MySQL: %v", err)
	}

	// 注册 Redis
	redisConfig := &dbs.RedisConfig{
		Host:      "127.0.0.1",
		Port:      6379,
		Password:  "iam59!z$",
		Database:  0,
		MaxIdle:   100,
		MaxActive: 100,
		Timeout:   5,
	}
	redisConn := dbs.NewRedisConnection(redisConfig)
	if err := registry.Register(dbs.Redis, redisConfig, redisConn); err != nil {
		log.Fatalf("Failed to register Redis: %v", err)
	}

	// 注册 MongoDB（可选）
	mongoConfig := &dbs.MongoConfig{
		URL: "mongodb://iam:iam59!z$@127.0.0.1:27017/iam_analytics?authSource=iam_analytics",
	}
	mongoConn := dbs.NewMongoDBConnection(mongoConfig)
	if err := registry.Register(dbs.MongoDB, mongoConfig, mongoConn); err != nil {
		log.Fatalf("Failed to register MongoDB: %v", err)
	}

	// 3. 初始化所有已注册的连接
	if err := registry.Init(); err != nil {
		log.Fatalf("Failed to initialize database connections: %v", err)
	}

	// 4. 在组件中使用数据库连接
	// 使用 MySQL
	if mysqlClient, err := registry.GetClient(dbs.MySQL); err == nil {
		if _, ok := mysqlClient.(*gorm.DB); ok {
			log.Println("MySQL client available")
			// 使用 db 进行数据库操作
			// db.Create(&User{...})
		}
	}

	// 使用 Redis
	if redisClient, err := registry.GetClient(dbs.Redis); err == nil {
		if _, ok := redisClient.(redis.UniversalClient); ok {
			log.Println("Redis client available")
			// 使用 client 进行缓存操作
			// client.Set("key", "value", time.Hour)
		}
	}

	// 使用 MongoDB
	if mongoClient, err := registry.GetClient(dbs.MongoDB); err == nil {
		if _, ok := mongoClient.(*mgo.Session); ok {
			log.Println("MongoDB client available")
			// 使用 session 进行文档操作
			// collection := session.DB("").C("collection")
		}
	}

	// 5. 健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := registry.HealthCheck(ctx); err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		log.Println("All databases are healthy")
	}

	// 6. 查看已注册的数据库类型
	registered := registry.ListRegistered()
	log.Printf("Registered databases: %v", registered)

	// 7. 优雅关闭
	defer func() {
		if err := registry.Close(); err != nil {
			log.Printf("Failed to close database registry: %v", err)
		}
	}()
}

// ComponentExample 展示组件如何使用注册器
type ComponentExample struct {
	registry *Registry
}

// NewComponentExample 创建组件实例
func NewComponentExample(registry *Registry) *ComponentExample {
	return &ComponentExample{
		registry: registry,
	}
}

// DoMySQLWork 执行 MySQL 相关操作
func (c *ComponentExample) DoMySQLWork() error {
	client, err := c.registry.GetClient(dbs.MySQL)
	if err != nil {
		return fmt.Errorf("MySQL not available: %w", err)
	}

	if _, ok := client.(*gorm.DB); ok {
		// 执行 MySQL 操作
		log.Println("Executing MySQL work...")
		// db.Create(&User{...})
		return nil
	}

	return fmt.Errorf("invalid MySQL client type")
}

// DoRedisWork 执行 Redis 相关操作
func (c *ComponentExample) DoRedisWork() error {
	client, err := c.registry.GetClient(dbs.Redis)
	if err != nil {
		return fmt.Errorf("redis not available: %w", err)
	}

	if _, ok := client.(redis.UniversalClient); ok {
		// 执行 Redis 操作
		log.Println("Executing Redis work...")
		// redisClient.Set("key", "value", time.Hour)
		return nil
	}

	return fmt.Errorf("invalid Redis client type")
}

// DoMongoWork 执行 MongoDB 相关操作
func (c *ComponentExample) DoMongoWork() error {
	client, err := c.registry.GetClient(dbs.MongoDB)
	if err != nil {
		return fmt.Errorf("mongodb not available: %w", err)
	}

	if _, ok := client.(*mgo.Session); ok {
		// 执行 MongoDB 操作
		log.Println("Executing MongoDB work...")
		// collection := session.DB("").C("collection")
		return nil
	}

	return fmt.Errorf("invalid MongoDB client type")
}

// ConditionalDatabaseUsage 展示条件性使用数据库
func ConditionalDatabaseUsage(registry *Registry) {
	// 检查 MySQL 是否可用
	if registry.IsRegistered(dbs.MySQL) {
		log.Println("MySQL is registered and available")
		// 使用 MySQL
	} else {
		log.Println("MySQL is not registered")
	}

	// 检查 Redis 是否可用
	if registry.IsRegistered(dbs.Redis) {
		log.Println("Redis is registered and available")
		// 使用 Redis
	} else {
		log.Println("Redis is not registered")
	}

	// 检查 MongoDB 是否可用
	if registry.IsRegistered(dbs.MongoDB) {
		log.Println("MongoDB is registered and available")
		// 使用 MongoDB
	} else {
		log.Println("MongoDB is not registered")
	}
}

// ApplicationExample 展示在应用程序中的使用
func ApplicationExample() {
	// 创建注册器
	registry := NewRegistry()

	// 根据配置注册数据库
	// 这里可以根据配置文件或环境变量决定注册哪些数据库

	// 注册 MySQL（必需）
	mysqlConfig := &dbs.MySQLConfig{
		Host:     "127.0.0.1:3306",
		Username: "iam",
		Password: "iam59!z$",
		Database: "iam",
	}
	mysqlConn := dbs.NewMySQLConnection(mysqlConfig)
	registry.Register(dbs.MySQL, mysqlConfig, mysqlConn)

	// 注册 Redis（必需）
	redisConfig := &dbs.RedisConfig{
		Host:     "127.0.0.1",
		Port:     6379,
		Password: "iam59!z$",
	}
	redisConn := dbs.NewRedisConnection(redisConfig)
	registry.Register(dbs.Redis, redisConfig, redisConn)

	// 注册 MongoDB（可选，根据配置决定）
	if shouldUseMongoDB() {
		mongoConfig := &dbs.MongoConfig{
			URL: "mongodb://iam:iam59!z$@127.0.0.1:27017/iam_analytics",
		}
		mongoConn := dbs.NewMongoDBConnection(mongoConfig)
		registry.Register(dbs.MongoDB, mongoConfig, mongoConn)
	}

	// 初始化所有连接
	if err := registry.Init(); err != nil {
		log.Fatalf("Failed to initialize databases: %v", err)
	}

	// 创建组件并使用数据库
	component := NewComponentExample(registry)

	// 执行工作
	if err := component.DoMySQLWork(); err != nil {
		log.Printf("MySQL work failed: %v", err)
	}

	if err := component.DoRedisWork(); err != nil {
		log.Printf("Redis work failed: %v", err)
	}

	if err := component.DoMongoWork(); err != nil {
		log.Printf("MongoDB work failed: %v", err)
	}

	// 优雅关闭
	defer registry.Close()
}

// shouldUseMongoDB 根据配置决定是否使用 MongoDB
func shouldUseMongoDB() bool {
	// 这里可以根据配置文件或环境变量来决定
	// 例如：检查 MONGODB_ENABLED 环境变量
	return true // 示例：总是启用
}
