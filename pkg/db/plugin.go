package db

import (
	"time"

	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

const (
	callBackBeforeName = "core:before"
	callBackAfterName  = "core:after"
	startTime          = "_start_time"
)

// TracePlugin 定义了 gorm 插件，用于跟踪 sql。
type TracePlugin struct{}

// Name 返回 trace 插件的名称。
func (op *TracePlugin) Name() string {
	return "tracePlugin"
}

// Initialize 初始化 trace 插件。
func (op *TracePlugin) Initialize(db *gorm.DB) (err error) {
	// 开始前
	_ = db.Callback().Create().Before("gorm:before_create").Register(callBackBeforeName, before)
	_ = db.Callback().Query().Before("gorm:query").Register(callBackBeforeName, before)
	_ = db.Callback().Delete().Before("gorm:before_delete").Register(callBackBeforeName, before)
	_ = db.Callback().Update().Before("gorm:setup_reflect_value").Register(callBackBeforeName, before)
	_ = db.Callback().Row().Before("gorm:row").Register(callBackBeforeName, before)
	_ = db.Callback().Raw().Before("gorm:raw").Register(callBackBeforeName, before)

	// 结束后
	_ = db.Callback().Create().After("gorm:after_create").Register(callBackAfterName, after)
	_ = db.Callback().Query().After("gorm:after_query").Register(callBackAfterName, after)
	_ = db.Callback().Delete().After("gorm:after_delete").Register(callBackAfterName, after)
	_ = db.Callback().Update().After("gorm:after_update").Register(callBackAfterName, after)
	_ = db.Callback().Row().After("gorm:row").Register(callBackAfterName, after)
	_ = db.Callback().Raw().After("gorm:raw").Register(callBackAfterName, after)

	return
}

// before 在执行 sql 之前设置开始时间。
// 参数 db 是 *gorm.DB 类型，表示 gorm 数据库实例。
var _ gorm.Plugin = &TracePlugin{}

// before 在执行 sql 之前设置开始时间。
// 参数 db 是 *gorm.DB 类型，表示 gorm 数据库实例。
func before(db *gorm.DB) {
	db.InstanceSet(startTime, time.Now())
}

// after 在执行 sql 之后计算执行时间。
// 参数 db 是 *gorm.DB 类型，表示 gorm 数据库实例。
func after(db *gorm.DB) {
	_ts, isExist := db.InstanceGet(startTime)
	if !isExist {
		return
	}

	ts, ok := _ts.(time.Time)
	if !ok {
		return
	}
	// sql := db.Dialector.Explain(db.Statement.SQL.String(), db.Statement.Vars...)
	log.Infof("sql cost time: %fs", time.Since(ts).Seconds())
}
