package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	maxLogSizeMB  = 10
	maxLogBackups = 5
	maxLogAgeDays = 30
)

// Logger owns the application log file and its zap logger.
type Logger struct {
	Zap     *zap.Logger
	Tailer  *Tailer
	rotator *lumberjack.Logger
}

func New(filePath string) (*Logger, error) {
	probe, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	_ = probe.Close()

	rotator := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    maxLogSizeMB,
		MaxBackups: maxLogBackups,
		MaxAge:     maxLogAgeDays,
		Compress:   true,
		LocalTime:  true,
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stack",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	sink := zapcore.AddSync(rotator)
	if os.Getenv("AUDIOUI_LOG_STDERR") == "1" {
		sink = zapcore.NewMultiWriteSyncer(sink, zapcore.Lock(os.Stderr))
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		sink,
		zap.InfoLevel,
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))

	return &Logger{
		Zap:     logger,
		Tailer:  &Tailer{path: filePath},
		rotator: rotator,
	}, nil
}

func (l *Logger) Sync() {
	if l == nil || l.Zap == nil {
		return
	}
	_ = l.Zap.Sync()
}

// Close flushes the logger and releases the log file handle. On Windows the
// handle must be closed before a temporary directory can be removed.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.Sync()
	if l.rotator == nil {
		return nil
	}
	return l.rotator.Close()
}
