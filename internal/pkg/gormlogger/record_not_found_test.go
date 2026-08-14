package gormlogger

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"
)

type recordingLogger struct {
	level      gormlog.LogLevel
	traceCalls int
	traceErr   error
	infoCalls  int
	warnCalls  int
	errorCalls int
}

func (l *recordingLogger) LogMode(level gormlog.LogLevel) gormlog.Interface {
	copy := *l
	copy.level = level
	return &copy
}

func (l *recordingLogger) Info(context.Context, string, ...interface{})  { l.infoCalls++ }
func (l *recordingLogger) Warn(context.Context, string, ...interface{})  { l.warnCalls++ }
func (l *recordingLogger) Error(context.Context, string, ...interface{}) { l.errorCalls++ }
func (l *recordingLogger) Trace(_ context.Context, _ time.Time, _ func() (string, int64), err error) {
	l.traceCalls++
	l.traceErr = err
}

func TestIgnoreRecordNotFoundSuppressesExpectedLookupMiss(t *testing.T) {
	delegate := &recordingLogger{}
	filtered := IgnoreRecordNotFound(delegate)
	sqlCalls := 0
	filtered.Trace(t.Context(), time.Now(), func() (string, int64) {
		sqlCalls++
		return "SELECT 1", 0
	}, gorm.ErrRecordNotFound)

	if delegate.traceCalls != 0 {
		t.Fatalf("trace calls = %d, want 0", delegate.traceCalls)
	}
	if sqlCalls != 0 {
		t.Fatalf("SQL formatter calls = %d, want 0", sqlCalls)
	}
}

func TestIgnoreRecordNotFoundSuppressesWrappedLookupMiss(t *testing.T) {
	delegate := &recordingLogger{}
	filtered := IgnoreRecordNotFound(delegate)
	filtered.Trace(t.Context(), time.Now(), func() (string, int64) {
		return "SELECT 1", 0
	}, errors.Join(errors.New("lookup failed"), gorm.ErrRecordNotFound))

	if delegate.traceCalls != 0 {
		t.Fatalf("trace calls = %d, want 0", delegate.traceCalls)
	}
}

func TestIgnoreRecordNotFoundPreservesRealErrorsAndLoggerMethods(t *testing.T) {
	delegate := &recordingLogger{}
	filtered := IgnoreRecordNotFound(delegate)
	wantErr := errors.New("connection reset")
	filtered.Trace(t.Context(), time.Now(), func() (string, int64) {
		return "SELECT 1", 0
	}, wantErr)
	filtered.Info(t.Context(), "info")
	filtered.Warn(t.Context(), "warn")
	filtered.Error(t.Context(), "error")

	if delegate.traceCalls != 1 || !errors.Is(delegate.traceErr, wantErr) {
		t.Fatalf("trace = (%d, %v), want (1, %v)", delegate.traceCalls, delegate.traceErr, wantErr)
	}
	if delegate.infoCalls != 1 || delegate.warnCalls != 1 || delegate.errorCalls != 1 {
		t.Fatalf("method calls = info:%d warn:%d error:%d, want 1 each", delegate.infoCalls, delegate.warnCalls, delegate.errorCalls)
	}
}

func TestIgnoreRecordNotFoundPreservesLogModeFiltering(t *testing.T) {
	delegate := &recordingLogger{}
	filtered := IgnoreRecordNotFound(delegate).LogMode(gormlog.Warn)
	wrapped, ok := filtered.(*recordNotFoundFilter)
	if !ok {
		t.Fatalf("LogMode() type = %T, want *recordNotFoundFilter", filtered)
	}
	reconfigured, ok := wrapped.delegate.(*recordingLogger)
	if !ok {
		t.Fatalf("delegate type = %T, want *recordingLogger", wrapped.delegate)
	}
	if reconfigured.level != gormlog.Warn {
		t.Fatalf("delegate level = %v, want %v", reconfigured.level, gormlog.Warn)
	}
}

func TestIgnoreRecordNotFoundAcceptsNilDelegate(t *testing.T) {
	if got := IgnoreRecordNotFound(nil); got != nil {
		t.Fatalf("IgnoreRecordNotFound(nil) = %T, want nil", got)
	}
}
