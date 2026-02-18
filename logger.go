package reco

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Logger 结构体
type Logger struct {
	config          Config
	level           atomic.Int32
	outputMode      atomic.Uint32
	mu              sync.Mutex
	writer          any // 使用 any 以便之后灵活处理 io.Writer
	file            *os.File
	logChan         chan LogEntry
	shutdownChan    chan struct{}
	wg              sync.WaitGroup
	currentFileSize atomic.Int64
	rotationTicker  *time.Ticker
	droppedCount    atomic.Int64
}

// New 创建并返回一个新的 Logger 实例
func New(cfg Config) (*Logger, error) {
	applyDefaults(&cfg)

	l := &Logger{
		config:       cfg,
		shutdownChan: make(chan struct{}),
	}
	l.level.Store(int32(cfg.Level))
	l.outputMode.Store(uint32(cfg.Mode))

	if err := l.initOutput(); err != nil {
		return nil, err
	}

	if cfg.Async {
		l.logChan = make(chan LogEntry, cfg.BufferSize)
		l.wg.Add(1)
		go l.worker()
	}

	return l, nil
}

func applyDefaults(cfg *Config) {
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = defaultTimeFormat
	}
	if cfg.MaxFileSizeMB <= 0 {
		cfg.MaxFileSizeMB = defaultMaxFileSizeMB
	}
	if cfg.MaxBackups < 0 {
		cfg.MaxBackups = defaultMaxBackups
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = defaultBufferSize
	}
	if cfg.EnableCaller && cfg.CallerSkip < 1 {
		cfg.CallerSkip = DefaultCallerSkip
	}
}

func (l *Logger) initOutput() error {
	cfg := l.config
	if cfg.Output != nil {
		l.writer = cfg.Output
		return nil
	}

	if cfg.FilePath != "" {
		dir := filepath.Dir(cfg.FilePath)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create log directory: %w", err)
			}
		}

		file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("failed to open log file '%s': %w", cfg.FilePath, err)
		}
		l.file = file
		l.writer = file
		info, err := file.Stat()
		if err == nil {
			l.currentFileSize.Store(info.Size())
		}

		if cfg.EnableRotation {
			l.rotationTicker = time.NewTicker(rotationCheckInterval)
			l.wg.Add(1)
			go l.monitorRotation()
		}
		return nil
	}

	l.writer = os.Stdout
	return nil
}

func (l *Logger) worker() {
	defer l.wg.Done()
	for {
		select {
		case entry, ok := <-l.logChan:
			if !ok {
				return
			}
			l.performWrite(entry)
		case <-l.shutdownChan:
			// Drain remaining logs
			for {
				select {
				case entry, ok := <-l.logChan:
					if !ok {
						return
					}
					l.performWrite(entry)
				default:
					return
				}
			}
		}
	}
}

func (l *Logger) monitorRotation() {
	defer l.wg.Done()
	if l.rotationTicker == nil {
		return
	}
	defer l.rotationTicker.Stop()

	for {
		select {
		case <-l.rotationTicker.C:
			l.checkAndRotate()
		case <-l.shutdownChan:
			return
		}
	}
}

func (l *Logger) checkAndRotate() {
	if !l.config.EnableRotation || l.file == nil {
		return
	}
	maxBytes := l.config.MaxFileSizeMB * 1024 * 1024
	if l.currentFileSize.Load() >= maxBytes {
		if err := l.rotateFile(); err != nil {
			fmt.Fprintf(os.Stderr, "Logger: failed to rotate log file: %v\n", err)
		}
	}
}

func (l *Logger) SetLevel(level Level) {
	l.level.Store(int32(level))
}

func (l *Logger) GetLevel() Level {
	return Level(l.level.Load())
}

func (l *Logger) SetOutputMode(mode OutputMode) {
	l.outputMode.Store(uint32(mode))
}

func (l *Logger) GetOutputMode() OutputMode {
	return OutputMode(l.outputMode.Load())
}
