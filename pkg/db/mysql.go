// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Options 定义了 MySQL 数据库的选项。
type Options struct {
	Host                  string
	Username              string
	Password              string
	Database              string
	MaxIdleConnections    int
	MaxOpenConnections    int
	MaxConnectionLifeTime time.Duration
	LogLevel              int
	Logger                logger.Interface
}

// New 创建一个新的 gorm db 实例。
// 参数 opts 是 *Options 类型，表示 MySQL 数据库的选项。
// 返回值是一个 *gorm.DB 类型，表示 gorm 数据库实例。
// 返回值是一个 error 类型，表示创建数据库实例时发生的错误。
func New(opts *Options) (*gorm.DB, error) {
	// 构建 DSN 字符串
	dsn := fmt.Sprintf(`%s:%s@tcp(%s)/%s?charset=utf8&parseTime=%t&loc=%s`,
		opts.Username,
		opts.Password,
		opts.Host,
		opts.Database,
		true,
		"Local")

	// 创建 gorm 数据库实例
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: opts.Logger,
	})
	if err != nil {
		return nil, err
	}

	// 获取 sql 数据库实例
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 设置最大打开连接数
	sqlDB.SetMaxOpenConns(opts.MaxOpenConnections)

	// 设置连接最大生存时间
	sqlDB.SetConnMaxLifetime(opts.MaxConnectionLifeTime)

	// 设置最大空闲连接数
	sqlDB.SetMaxIdleConns(opts.MaxIdleConnections)

	return db, nil
}
