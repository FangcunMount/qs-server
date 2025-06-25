// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package mysql

import (
	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/store"
)

// datastore MySQL数据存储实现
type datastore struct {
	db *gorm.DB
}

// Users 返回用户存储接口实现
func (ds *datastore) Users() store.UserStore {
	return newUsers(ds)
}

// Close 关闭数据库连接（由外部 DatabaseManager 管理，这里为空实现）
func (ds *datastore) Close() error {
	// 连接由 DatabaseManager 管理，这里不需要关闭
	return nil
}

// NewMySQLStore 创建 MySQL 存储工厂（依赖注入方式）
func NewMySQLStore(db *gorm.DB) store.Factory {
	return &datastore{db: db}
}
