package reco

import (
	"fmt"
	"os"
	"sync"
)

var (
	DefaultLogger     *Logger
	defaultLoggerOnce sync.Once
)

func GetDefaultLogger() *Logger {
	defaultLoggerOnce.Do(func() {
		cfg := Config{
			Level:        LevelInfo,
			Mode:         ModeText,
			TimeFormat:   defaultTimeFormat,
			Output:       os.Stdout,
			Async:        defaultAsync,
			BufferSize:   1024,
			EnableCaller: true,
			CallerSkip:   DefaultCallerSkip + 1,
		}
		var err error
		DefaultLogger, err = New(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "CRITICAL: Failed to initialize default logger: %v\n", err)
			DefaultLogger, _ = New(Config{Output: os.Stderr, Level: LevelError, EnableCaller: true, CallerSkip: DefaultCallerSkip + 1})
		}
	})
	return DefaultLogger
}

func InitDefaultLoggerWithConfig(cfg Config) (*Logger, error) {
	var initError error
	if cfg.EnableCaller && cfg.CallerSkip < (DefaultCallerSkip+1) {
		cfg.CallerSkip = DefaultCallerSkip + 1
	}
	defaultLoggerOnce.Do(func() {
		var err error
		DefaultLogger, err = New(cfg)
		if err != nil {
			initError = fmt.Errorf("failed to initialize default logger: %w", err)
			DefaultLogger, _ = New(Config{Output: os.Stderr, Level: LevelError, EnableCaller: true, CallerSkip: DefaultCallerSkip + 1})
		}
	})
	return DefaultLogger, initError
}

func SetDefaultLogLevel(level Level) {
	GetDefaultLogger().SetLevel(level)
}

func Debugf(format string, args ...any) { GetDefaultLogger().Debugf(format, args...) }
func Infof(format string, args ...any)  { GetDefaultLogger().Infof(format, args...) }
func Warnf(format string, args ...any)  { GetDefaultLogger().Warnf(format, args...) }
func Errorf(format string, args ...any) { GetDefaultLogger().Errorf(format, args...) }
func Fatalf(format string, args ...any) { GetDefaultLogger().Fatalf(format, args...) }
func Panicf(format string, args ...any) { GetDefaultLogger().Panicf(format, args...) }

func Debug(message string, fields ...Fields) { GetDefaultLogger().Debug(message, fields...) }
func Info(message string, fields ...Fields)  { GetDefaultLogger().Info(message, fields...) }
func Warn(message string, fields ...Fields)  { GetDefaultLogger().Warn(message, fields...) }
func Error(message string, fields ...Fields) { GetDefaultLogger().Error(message, fields...) }
func Fatal(message string, fields ...Fields) { GetDefaultLogger().Fatal(message, fields...) }
func Panic(message string, fields ...Fields) { GetDefaultLogger().Panic(message, fields...) }

func Close() error {
	l := DefaultLogger
	if l != nil {
		return l.Close()
	}
	return nil
}
