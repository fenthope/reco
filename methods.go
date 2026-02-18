package reco

import (
	"fmt"
	"io"
	"os"
)

func (l *Logger) Debugf(format string, args ...any) {
	if l.GetLevel() <= LevelDebug {
		l.submitEntry(LevelDebug, fmt.Sprintf(format, args...), nil)
	}
}
func (l *Logger) Infof(format string, args ...any) {
	if l.GetLevel() <= LevelInfo {
		l.submitEntry(LevelInfo, fmt.Sprintf(format, args...), nil)
	}
}
func (l *Logger) Warnf(format string, args ...any) {
	if l.GetLevel() <= LevelWarn {
		l.submitEntry(LevelWarn, fmt.Sprintf(format, args...), nil)
	}
}
func (l *Logger) Errorf(format string, args ...any) {
	if l.GetLevel() <= LevelError {
		l.submitEntry(LevelError, fmt.Sprintf(format, args...), nil)
	}
}
func (l *Logger) Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.submitEntry(LevelFatal, msg, nil)
	l.Close()
	os.Exit(1)
}
func (l *Logger) Panicf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.submitEntry(LevelPanic, msg, nil)
	l.Close()
	panic(msg)
}

func (l *Logger) Debug(message string, fields ...Fields) {
	if l.GetLevel() <= LevelDebug {
		l.submitEntry(LevelDebug, message, mergeFields(fields...))
	}
}
func (l *Logger) Info(message string, fields ...Fields) {
	if l.GetLevel() <= LevelInfo {
		l.submitEntry(LevelInfo, message, mergeFields(fields...))
	}
}
func (l *Logger) Warn(message string, fields ...Fields) {
	if l.GetLevel() <= LevelWarn {
		l.submitEntry(LevelWarn, message, mergeFields(fields...))
	}
}
func (l *Logger) Error(message string, fields ...Fields) {
	if l.GetLevel() <= LevelError {
		l.submitEntry(LevelError, message, mergeFields(fields...))
	}
}
func (l *Logger) Fatal(message string, fields ...Fields) {
	l.submitEntry(LevelFatal, message, mergeFields(fields...))
	l.Close()
	os.Exit(1)
}
func (l *Logger) Panic(message string, fields ...Fields) {
	l.submitEntry(LevelPanic, message, mergeFields(fields...))
	l.Close()
	panic(message)
}

func (l *Logger) Close() error {
	alreadyClosed := false
	select {
	case <-l.shutdownChan:
		alreadyClosed = true
	default:
		close(l.shutdownChan)
	}

	if alreadyClosed {
		l.wg.Wait()
		return fmt.Errorf("logger already closed or shutting down")
	}

	if l.config.Async && l.logChan != nil {
		close(l.logChan)
	}

	l.wg.Wait()

	l.mu.Lock()
	defer l.mu.Unlock()

	var closeError error
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			closeError = fmt.Errorf("failed to close log file: %w", err)
		}
		l.file = nil
		// 检查 writer 是否也是 file
		if w, ok := l.writer.(*os.File); ok && w != nil {
			l.writer = nil
		} else if _, ok := l.writer.(io.Closer); ok && l.config.Output == nil {
			l.writer = nil
		}
	}

	dropped := l.droppedCount.Load()
	if dropped > 0 {
		fmt.Fprintf(os.Stderr, "Logger: %d log entries dropped due to buffer overflow.\n", dropped)
	}
	return closeError
}
