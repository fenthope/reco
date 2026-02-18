package reco

import (
	"fmt"
	"os"
	"strings"
)

// Level 定义日志级别
type Level int32

// 日志级别常量
const (
	LevelDebug Level = iota // 调试级别
	LevelInfo               // 信息级别
	LevelWarn               // 警告级别
	LevelError              // 错误级别
	LevelFatal              // 致命错误级别
	LevelPanic              // Panic 级别
	levelNone  Level = 99   // 用于关闭日志输出
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
	LevelFatal: "FATAL",
	LevelPanic: "PANIC",
}

// String 返回日志级别的文本表示
func (l Level) String() string {
	if name, ok := levelNames[l]; ok {
		return name
	}
	return "UNKNOWN"
}

// ParseLevel 将字符串解析为 Level
func ParseLevel(levelStr string) Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG", "DUMP":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	case "PANIC":
		return LevelPanic
	case "NONE":
		return levelNone
	default:
		fmt.Fprintf(os.Stderr, "Unknown log level '%s', defaulting to INFO\n", levelStr)
		return LevelInfo
	}
}
