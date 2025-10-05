package pubsub

import (
	"github.com/ThreeDotsLabs/watermill"
	"github.com/fangcun-mount/qs-server/pkg/log"
)

// watermillLogger Watermill 日志适配器
type watermillLogger struct{}

func (l *watermillLogger) Error(msg string, err error, fields watermill.LogFields) {
	log.Errorf("%s: %v, fields: %v", msg, err, fields)
}

func (l *watermillLogger) Info(msg string, fields watermill.LogFields) {
	log.Infof("%s, fields: %v", msg, fields)
}

func (l *watermillLogger) Debug(msg string, fields watermill.LogFields) {
	log.Debugf("%s, fields: %v", msg, fields)
}

func (l *watermillLogger) Trace(msg string, fields watermill.LogFields) {
	log.Debugf("TRACE: %s, fields: %v", msg, fields)
}

func (l *watermillLogger) With(fields watermill.LogFields) watermill.LoggerAdapter {
	return l
}
