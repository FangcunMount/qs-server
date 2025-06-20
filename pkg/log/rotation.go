package log

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// getLumberjackWriter 创建 lumberjack 日志轮转写入器
func getLumberjackWriter(filename string, opts *Options) zapcore.WriteSyncer {
	// 确保日志目录存在
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		// 如果创建目录失败，返回 stderr
		return zapcore.AddSync(os.Stderr)
	}

	lumberJackLogger := &lumberjack.Logger{
		Filename:   filename,        // 日志文件位置
		MaxSize:    opts.MaxSize,    // 单个文件最大大小（MB）
		MaxBackups: opts.MaxBackups, // 保留旧文件的最大个数
		MaxAge:     opts.MaxAge,     // 保留旧文件的最大天数
		Compress:   opts.Compress,   // 是否压缩/归档旧文件
	}

	return zapcore.AddSync(lumberJackLogger)
}

// getOutputPaths 处理输出路径，为文件路径创建轮转写入器
func getOutputPaths(paths []string, opts *Options) []zapcore.WriteSyncer {
	var writers []zapcore.WriteSyncer

	for _, path := range paths {
		switch path {
		case "stdout":
			writers = append(writers, zapcore.AddSync(os.Stdout))
		case "stderr":
			writers = append(writers, zapcore.AddSync(os.Stderr))
		default:
			// 如果是文件路径，使用 lumberjack 轮转
			writers = append(writers, getLumberjackWriter(path, opts))
		}
	}

	return writers
}

// NewWithRotation 创建支持日志轮转的 logger
func NewWithRotation(opts *Options) *zap.Logger {
	if opts == nil {
		opts = NewOptions()
	}

	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(opts.Level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	encodeLevel := zapcore.CapitalLevelEncoder
	if opts.Format == consoleFormat && opts.EnableColor {
		encodeLevel = zapcore.CapitalColorLevelEncoder
	}

	encoderConfig := zapcore.EncoderConfig{
		MessageKey:     "message",
		LevelKey:       "level",
		TimeKey:        "timestamp",
		NameKey:        "logger",
		CallerKey:      "caller",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    encodeLevel,
		EncodeTime:     timeEncoder,
		EncodeDuration: milliSecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 处理输出路径
	outputWriters := getOutputPaths(opts.OutputPaths, opts)
	errorWriters := getOutputPaths(opts.ErrorOutputPaths, opts)

	// 创建核心
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(outputWriters...),
		zapLevel,
	)

	// 创建 logger
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	// 如果有错误输出路径，创建错误日志核心
	if len(errorWriters) > 0 {
		errorCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.NewMultiWriteSyncer(errorWriters...),
			zapcore.ErrorLevel,
		)
		logger = logger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core, errorCore)
		}))
	}

	return logger
}
