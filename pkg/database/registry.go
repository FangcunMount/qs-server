// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"
	"log"
	"sync"

	dbs "github.com/yshujie/questionnaire-scale/pkg/database/databases"
)

// Registry 数据库注册器
type Registry struct {
	mu          sync.RWMutex
	connections map[dbs.DatabaseType]dbs.Connection
	configs     map[dbs.DatabaseType]interface{}
	initialized bool
}

// NewRegistry 创建新的数据库注册器
func NewRegistry() *Registry {
	return &Registry{
		connections: make(map[dbs.DatabaseType]dbs.Connection),
		configs:     make(map[dbs.DatabaseType]interface{}),
	}
}

// Register 注册数据库连接
func (r *Registry) Register(dbType dbs.DatabaseType, config interface{}, connection dbs.Connection) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return fmt.Errorf("registry already initialized, cannot register new connections")
	}

	if connection == nil {
		return fmt.Errorf("connection cannot be nil")
	}

	if connection.Type() != dbType {
		return fmt.Errorf("connection type mismatch: expected %s, got %s", dbType, connection.Type())
	}

	r.connections[dbType] = connection
	r.configs[dbType] = config

	log.Printf("Registered database connection: %s", dbType)
	return nil
}

// Init 初始化所有已注册的数据库连接
func (r *Registry) Init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil
	}

	for dbType, connection := range r.connections {
		log.Printf("Initializing database connection: %s", dbType)
		if err := connection.Connect(); err != nil {
			return fmt.Errorf("failed to connect to %s: %w", dbType, err)
		}
	}

	r.initialized = true
	log.Println("All database connections initialized successfully")
	return nil
}

// Get 获取指定类型的数据库连接
func (r *Registry) Get(dbType dbs.DatabaseType) (dbs.Connection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	connection, exists := r.connections[dbType]
	if !exists {
		return nil, fmt.Errorf("database connection not found: %s", dbType)
	}

	return connection, nil
}

// GetClient 获取指定类型的数据库客户端
func (r *Registry) GetClient(dbType dbs.DatabaseType) (interface{}, error) {
	connection, err := r.Get(dbType)
	if err != nil {
		return nil, err
	}

	return connection.GetClient(), nil
}

// Close 关闭所有数据库连接
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error

	for dbType, connection := range r.connections {
		log.Printf("Closing database connection: %s", dbType)
		if err := connection.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close %s: %w", dbType, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}

	r.initialized = false
	log.Println("All database connections closed successfully")
	return nil
}

// HealthCheck 对所有数据库连接进行健康检查
func (r *Registry) HealthCheck(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error

	for dbType, connection := range r.connections {
		if err := connection.HealthCheck(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s health check failed: %w", dbType, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("database health check failed: %v", errs)
	}

	return nil
}

// ListRegistered 列出所有已注册的数据库类型
func (r *Registry) ListRegistered() []dbs.DatabaseType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]dbs.DatabaseType, 0, len(r.connections))
	for dbType := range r.connections {
		types = append(types, dbType)
	}

	return types
}

// IsRegistered 检查指定类型的数据库是否已注册
func (r *Registry) IsRegistered(dbType dbs.DatabaseType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.connections[dbType]
	return exists
}

// IsInitialized 检查注册器是否已初始化
func (r *Registry) IsInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.initialized
}
