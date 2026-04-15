package apiserver

import "fmt"

// GetMongoSession 获取 MongoDB 会话兼容入口。
// Deprecated: Use GetMongoClient instead.
func (dm *DatabaseManager) GetMongoSession() (interface{}, error) {
	return nil, fmt.Errorf("mgo session compatibility is unsupported; use GetMongoClient instead")
}
