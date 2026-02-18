package reco

import (
	"time"
)

// OutputMode 定义日志输出格式
type OutputMode uint8

const (
	ModeText OutputMode = iota // 文本格式
	ModeJSON                   // JSON 格式
)

// Fields 类型用于结构化日志的附加字段
type Fields map[string]any

// LogEntry 代表一条日志记录
type LogEntry struct {
	Timestamp time.Time // 日志记录时间
	Level     Level     // 日志级别
	Message   string    // 日志消息
	Fields    Fields    // 结构化字段
	Caller    string    // 调用者信息
}
