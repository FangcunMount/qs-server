package codeutil

import (
	"errors"
	"sync"
	"time"

	"github.com/mattheath/base62"
	"github.com/sony/sonyflake"
)

var (
	once      sync.Once
	flake     *sonyflake.Sonyflake
	initError error
)

// 初始化 Sonyflake 实例（只初始化一次）
func initSonyflake() {
	settings := sonyflake.Settings{
		StartTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), // 可自定义起始时间
	}
	flake = sonyflake.NewSonyflake(settings)
	if flake == nil {
		initError = errors.New("sonyflake initialization failed")
	}
}

// GenerateCode 生成 12 位以内的唯一字符串 ID
func GenerateCode() (string, error) {
	once.Do(initSonyflake)
	if initError != nil {
		return "", initError
	}
	id, err := flake.NextID()
	if err != nil {
		return "", err
	}
	return base62.EncodeInt64(int64(id)), nil
}
