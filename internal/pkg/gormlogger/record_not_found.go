package gormlogger

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

// IgnoreRecordNotFound keeps the configured GORM logger for real failures and
// slow queries while suppressing the expected not-found control-flow result.
// GORM repositories routinely use ErrRecordNotFound to express an empty
// lookup; logging it as an error amplifies normal traffic and obscures actual
// database failures.
func IgnoreRecordNotFound(delegate gormlog.Interface) gormlog.Interface {
	if delegate == nil {
		return nil
	}
	return &recordNotFoundFilter{delegate: delegate}
}

type recordNotFoundFilter struct {
	delegate gormlog.Interface
}

func (l *recordNotFoundFilter) LogMode(level gormlog.LogLevel) gormlog.Interface {
	return IgnoreRecordNotFound(l.delegate.LogMode(level))
}

func (l *recordNotFoundFilter) Info(ctx context.Context, msg string, data ...interface{}) {
	l.delegate.Info(ctx, msg, data...)
}

func (l *recordNotFoundFilter) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.delegate.Warn(ctx, msg, data...)
}

func (l *recordNotFoundFilter) Error(ctx context.Context, msg string, data ...interface{}) {
	l.delegate.Error(ctx, msg, data...)
}

func (l *recordNotFoundFilter) Trace(
	ctx context.Context,
	begin time.Time,
	fc func() (sql string, rowsAffected int64),
	err error,
) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	l.delegate.Trace(ctx, begin, fc, err)
}
