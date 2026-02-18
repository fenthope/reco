package reco

import (
	"io"
	"time"
)

// Config 用于配置 Logger
type Config struct {
	Level           Level      // 最低日志记录级别
	Mode            OutputMode // 输出模式 (Text 或 JSON)
	TimeFormat      string     // 时间戳格式
	Output          io.Writer  // 日志输出目标
	FilePath        string     // 日志文件路径
	EnableRotation  bool       // 是否启用日志轮转
	MaxFileSizeMB   int64      // 单个日志文件最大大小 (MB)
	MaxBackups      int        // 保留的旧日志文件数量
	CompressBackups bool       // 是否压缩备份
	Async           bool       // 是否启用异步写入
	BufferSize      int        // 异步模式下的缓冲区大小
	CallerSkip      int        // runtime.Caller 的跳过层数
	DefaultFields   Fields     // 每条日志都会附带的默认字段
	EnableCaller    bool       // 是否记录调用者信息
}

const (
	defaultTimeFormat      = time.RFC3339Nano
	defaultMaxFileSizeMB   = 10
	defaultMaxBackups      = 5
	defaultCompressBackups = true
	defaultAsync           = true
	defaultBufferSize      = 8192
	DefaultCallerSkip      = 2
	rotationCheckInterval  = 5 * time.Minute
)
