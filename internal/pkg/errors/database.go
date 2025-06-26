package errors

import "net/http"

// 数据库相关错误码 (15xxxx)
const (
	// ErrDatabase - 数据库错误
	ErrDatabase int = iota + 150000

	// 通用数据库错误
	// ErrDatabaseConnection - 数据库连接错误
	ErrDatabaseConnection
	// ErrDatabaseTimeout - 数据库超时
	ErrDatabaseTimeout
	// ErrDatabaseTransaction - 数据库事务错误
	ErrDatabaseTransaction
	// ErrDatabaseQuery - 数据库查询错误
	ErrDatabaseQuery
	// ErrDatabaseInsert - 数据库插入错误
	ErrDatabaseInsert
	// ErrDatabaseUpdate - 数据库更新错误
	ErrDatabaseUpdate
	// ErrDatabaseDelete - 数据库删除错误
	ErrDatabaseDelete
	// ErrDatabaseConstraint - 数据库约束错误
	ErrDatabaseConstraint
	// ErrDatabaseDuplicateKey - 数据库重复键错误
	ErrDatabaseDuplicateKey
	// ErrDatabaseForeignKey - 数据库外键错误
	ErrDatabaseForeignKey
	// ErrDatabaseDataTooLong - 数据库数据过长错误
	ErrDatabaseDataTooLong
	// ErrDatabaseSyntax - 数据库语法错误
	ErrDatabaseSyntax
	// ErrDatabasePermission - 数据库权限错误
	ErrDatabasePermission
	// ErrDatabaseMigration - 数据库迁移错误
	ErrDatabaseMigration
	// ErrDatabaseBackup - 数据库备份错误
	ErrDatabaseBackup
	// ErrDatabaseRestore - 数据库恢复错误
	ErrDatabaseRestore
	// ErrDatabaseLock - 数据库锁定错误
	ErrDatabaseLock
	// ErrDatabaseDeadlock - 数据库死锁错误
	ErrDatabaseDeadlock
	// ErrDatabaseCorrupted - 数据库损坏错误
	ErrDatabaseCorrupted
	// ErrDatabasePoolExhausted - 数据库连接池耗尽
	ErrDatabasePoolExhausted
	// ErrDatabaseSchemaVersion - 数据库版本错误
	ErrDatabaseSchemaVersion

	// MySQL相关错误
	// ErrMySQL - MySQL错误
	ErrMySQL
	// ErrMySQLConnection - MySQL连接错误
	ErrMySQLConnection
	// ErrMySQLTimeout - MySQL超时错误
	ErrMySQLTimeout
	// ErrMySQLSyntax - MySQL语法错误
	ErrMySQLSyntax
	// ErrMySQLDuplicateEntry - MySQL重复条目错误
	ErrMySQLDuplicateEntry
	// ErrMySQLConstraintViolation - MySQL约束违反错误
	ErrMySQLConstraintViolation

	// MongoDB相关错误
	// ErrMongoDB - MongoDB错误
	ErrMongoDB
	// ErrMongoDBConnection - MongoDB连接错误
	ErrMongoDBConnection
	// ErrMongoDBCollection - MongoDB集合错误
	ErrMongoDBCollection
	// ErrMongoDBIndex - MongoDB索引错误
	ErrMongoDBIndex
	// ErrMongoDBDocument - MongoDB文档错误
	ErrMongoDBDocument
	// ErrMongoDBQuery - MongoDB查询错误
	ErrMongoDBQuery
	// ErrMongoDBAggregate - MongoDB聚合错误
	ErrMongoDBAggregate
	// ErrMongoDBTransaction - MongoDB事务错误
	ErrMongoDBTransaction
	// ErrMongoDBDuplicateKey - MongoDB重复键错误
	ErrMongoDBDuplicateKey
	// ErrMongoDBValidation - MongoDB验证错误
	ErrMongoDBValidation

	// Redis相关错误
	// ErrRedis - Redis错误
	ErrRedis
	// ErrRedisConnection - Redis连接错误
	ErrRedisConnection
	// ErrRedisTimeout - Redis超时错误
	ErrRedisTimeout
	// ErrRedisKey - Redis键错误
	ErrRedisKey
	// ErrRedisValue - Redis值错误
	ErrRedisValue
	// ErrRedisExpiration - Redis过期错误
	ErrRedisExpiration
	// ErrRedisMemory - Redis内存错误
	ErrRedisMemory
	// ErrRedisCluster - Redis集群错误
	ErrRedisCluster
	// ErrRedisSentinel - Redis哨兵错误
	ErrRedisSentinel
	// ErrRedisScript - Redis脚本错误
	ErrRedisScript
)

// 数据库错误码注册
func init() {
	register(ErrDatabase, http.StatusInternalServerError, "数据库错误", "")

	// 通用数据库错误
	register(ErrDatabaseConnection, http.StatusServiceUnavailable, "数据库连接失败", "")
	register(ErrDatabaseTimeout, http.StatusGatewayTimeout, "数据库操作超时", "")
	register(ErrDatabaseTransaction, http.StatusInternalServerError, "数据库事务错误", "")
	register(ErrDatabaseQuery, http.StatusInternalServerError, "数据库查询错误", "")
	register(ErrDatabaseInsert, http.StatusInternalServerError, "数据库插入失败", "")
	register(ErrDatabaseUpdate, http.StatusInternalServerError, "数据库更新失败", "")
	register(ErrDatabaseDelete, http.StatusInternalServerError, "数据库删除失败", "")
	register(ErrDatabaseConstraint, http.StatusBadRequest, "数据库约束冲突", "")
	register(ErrDatabaseDuplicateKey, http.StatusConflict, "数据库重复键冲突", "")
	register(ErrDatabaseForeignKey, http.StatusBadRequest, "数据库外键约束违反", "")
	register(ErrDatabaseDataTooLong, http.StatusBadRequest, "数据过长", "")
	register(ErrDatabaseSyntax, http.StatusInternalServerError, "数据库语法错误", "")
	register(ErrDatabasePermission, http.StatusForbidden, "数据库权限不足", "")
	register(ErrDatabaseMigration, http.StatusInternalServerError, "数据库迁移失败", "")
	register(ErrDatabaseBackup, http.StatusInternalServerError, "数据库备份失败", "")
	register(ErrDatabaseRestore, http.StatusInternalServerError, "数据库恢复失败", "")
	register(ErrDatabaseLock, http.StatusConflict, "数据库锁定冲突", "")
	register(ErrDatabaseDeadlock, http.StatusConflict, "数据库死锁", "")
	register(ErrDatabaseCorrupted, http.StatusInternalServerError, "数据库损坏", "")
	register(ErrDatabasePoolExhausted, http.StatusServiceUnavailable, "数据库连接池耗尽", "")
	register(ErrDatabaseSchemaVersion, http.StatusInternalServerError, "数据库版本不匹配", "")

	// MySQL相关错误
	register(ErrMySQL, http.StatusInternalServerError, "MySQL数据库错误", "")
	register(ErrMySQLConnection, http.StatusServiceUnavailable, "MySQL连接失败", "")
	register(ErrMySQLTimeout, http.StatusGatewayTimeout, "MySQL操作超时", "")
	register(ErrMySQLSyntax, http.StatusInternalServerError, "MySQL语法错误", "")
	register(ErrMySQLDuplicateEntry, http.StatusConflict, "MySQL重复条目", "")
	register(ErrMySQLConstraintViolation, http.StatusBadRequest, "MySQL约束违反", "")

	// MongoDB相关错误
	register(ErrMongoDB, http.StatusInternalServerError, "MongoDB数据库错误", "")
	register(ErrMongoDBConnection, http.StatusServiceUnavailable, "MongoDB连接失败", "")
	register(ErrMongoDBCollection, http.StatusInternalServerError, "MongoDB集合错误", "")
	register(ErrMongoDBIndex, http.StatusInternalServerError, "MongoDB索引错误", "")
	register(ErrMongoDBDocument, http.StatusInternalServerError, "MongoDB文档错误", "")
	register(ErrMongoDBQuery, http.StatusInternalServerError, "MongoDB查询错误", "")
	register(ErrMongoDBAggregate, http.StatusInternalServerError, "MongoDB聚合错误", "")
	register(ErrMongoDBTransaction, http.StatusInternalServerError, "MongoDB事务错误", "")
	register(ErrMongoDBDuplicateKey, http.StatusConflict, "MongoDB重复键冲突", "")
	register(ErrMongoDBValidation, http.StatusBadRequest, "MongoDB验证失败", "")

	// Redis相关错误
	register(ErrRedis, http.StatusInternalServerError, "Redis缓存错误", "")
	register(ErrRedisConnection, http.StatusServiceUnavailable, "Redis连接失败", "")
	register(ErrRedisTimeout, http.StatusGatewayTimeout, "Redis操作超时", "")
	register(ErrRedisKey, http.StatusBadRequest, "Redis键错误", "")
	register(ErrRedisValue, http.StatusBadRequest, "Redis值错误", "")
	register(ErrRedisExpiration, http.StatusGone, "Redis键已过期", "")
	register(ErrRedisMemory, http.StatusInsufficientStorage, "Redis内存不足", "")
	register(ErrRedisCluster, http.StatusServiceUnavailable, "Redis集群错误", "")
	register(ErrRedisSentinel, http.StatusServiceUnavailable, "Redis哨兵错误", "")
	register(ErrRedisScript, http.StatusInternalServerError, "Redis脚本执行错误", "")
}
