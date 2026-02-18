package reco

import (
	"time"
	"os"
	"testing"
)

func TestLogger(t *testing.T) {
	// 测试默认 Logger
	Info("测试默认 Info 日志")
	Debug("这条不应该显示")

	SetDefaultLogLevel(LevelDebug)
	Debug("设置级别后显示 Debug 日志")

	// 测试自定义 Logger (JSON)
	cfg := Config{
		Level: LevelDebug,
		Mode:  ModeJSON,
		Async: false, // 同步方便测试
	}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("创建 Logger 失败: %v", err)
	}

	logger.Info("测试 JSON 日志", Fields{"key": "value", "count": 42})

	// 测试文件输出
	tmpFile := "test_reco.log"
	defer os.Remove(tmpFile)

	fileCfg := Config{
		FilePath: tmpFile,
		Level:    LevelInfo,
		Async:    false,
	}
	fileLogger, err := New(fileCfg)
	if err != nil {
		t.Fatalf("创建文件 Logger 失败: %v", err)
	}

	fileLogger.Info("记录到文件")
	fileLogger.Close()

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("读取测试日志文件失败: %v", err)
	}
	if len(content) == 0 {
		t.Error("日志文件内容为空")
	}
}

func TestMergeFields(t *testing.T) {
	f1 := Fields{"a": 1}
	f2 := Fields{"b": 2, "a": 3}
	merged := mergeFields(f1, f2)

	if merged["a"] != 3 {
		t.Errorf("预期 a=3, 得到 %v", merged["a"])
	}
	if merged["b"] != 2 {
		t.Errorf("预期 b=2, 得到 %v", merged["b"])
	}
}

func TestRotationInit(t *testing.T) {
	// 仅测试初始化逻辑
	cfg := Config{
		FilePath:       "test_rotate.log",
		EnableRotation: true,
	}
	defer os.Remove("test_rotate.log")
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("创建带轮转的 Logger 失败: %v", err)
	}
	l.Close()
}

func TestDurationAdaptiveFormatting(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{500 * time.Nanosecond, "500ns"},
		{500 * time.Microsecond, "500.00µs"},
		{500 * time.Millisecond, "500.00ms"},
		{5 * time.Second, "5.00s"},
		{5 * time.Minute, "5.00m"},
		{5 * time.Hour, "5.00h"},
	}

	for _, tt := range tests {
		actual := formatDuration(tt.d)
		if actual != tt.expected {
			t.Errorf("formatDuration(%v) = %v; want %v", tt.d, actual, tt.expected)
		}
	}
}
