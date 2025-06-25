// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package mysql

import (
	"context"

	v1 "github.com/yshujie/questionnaire-scale/pkg/api/apiserver/v1"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
	metav1 "github.com/yshujie/questionnaire-scale/pkg/meta/v1"
	"gorm.io/gorm"
)

type users struct {
	db *gorm.DB
}

func newUsers(ds *datastore) *users {
	return &users{ds.db}
}

// Create creates a new user account.
func (u *users) Create(ctx context.Context, user *v1.User, opts metav1.CreateOptions) error {
	return u.db.WithContext(ctx).Create(&user).Error
}

// Update updates an user account information.
func (u *users) Update(ctx context.Context, user *v1.User, opts metav1.UpdateOptions) error {
	return u.db.WithContext(ctx).Save(user).Error
}

// Delete deletes the user by the user identifier.
func (u *users) Delete(ctx context.Context, username string, opts metav1.DeleteOptions) error {
	if opts.Unscoped {
		u.db = u.db.Unscoped()
	}

	err := u.db.WithContext(ctx).Where("name = ?", username).Delete(&v1.User{}).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("database error: " + err.Error())
	}

	return nil
}

// DeleteCollection batch deletes the users.
func (u *users) DeleteCollection(ctx context.Context, usernames []string, opts metav1.DeleteOptions) error {
	if opts.Unscoped {
		u.db = u.db.Unscoped()
	}

	return u.db.WithContext(ctx).Where("name in (?)", usernames).Delete(&v1.User{}).Error
}

// Get return an user by the user identifier.
func (u *users) Get(ctx context.Context, username string, opts metav1.GetOptions) (*v1.User, error) {
	user := &v1.User{}
	err := u.db.WithContext(ctx).Where("name = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, errors.New("database error: " + err.Error())
	}

	return user, nil
}

// List return all users.
func (u *users) List(ctx context.Context, opts metav1.ListOptions) (*v1.UserList, error) {
	ret := &v1.UserList{}

	// 设置默认的偏移量和限制
	offset := 0
	limit := 10

	if opts.Offset != nil {
		offset = int(*opts.Offset)
	}
	if opts.Limit != nil {
		limit = int(*opts.Limit)
	}

	// 执行查询
	query := u.db.WithContext(ctx).Model(&v1.User{})

	// 获取总数
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, errors.New("database error: " + err.Error())
	}
	ret.TotalCount = totalCount

	// 获取数据
	if err := query.Offset(offset).Limit(limit).Order("id desc").Find(&ret.Items).Error; err != nil {
		return nil, errors.New("database error: " + err.Error())
	}

	return ret, nil
}
