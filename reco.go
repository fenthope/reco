package reco // 包名保持与您提供的一致

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level 定义日志级别
type Level int32

// 日志级别常量
// 值从 0 开始 方便原子操作和数组索引
const (
	LevelDebug Level = iota // 调试级别
	LevelInfo               // 信息级别
	LevelWarn               // 警告级别
	LevelError              // 错误级别
	LevelFatal              // 致命错误级别 记录后将调用 os_Exit(1)
	LevelPanic              // Panic 级别 记录后将调用 panic
	levelNone  Level = 99   // 用于关闭日志输出
)

// 日志级别名称映射
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
// 如果无法解析 默认返回 LevelInfo
func ParseLevel(levelStr string) Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
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

// OutputMode 定义日志输出格式
type OutputMode uint8

const (
	ModeText OutputMode = iota // 文本格式
	ModeJSON                   // JSON 格式
)

// Fields 类型用于结构化日志的附加字段
type Fields map[string]interface{}

// LogEntry 代表一条日志记录
type LogEntry struct {
	Timestamp time.Time // 日志记录时间
	Level     Level     // 日志级别
	Message   string    // 日志消息
	Fields    Fields    // 结构化字段 (用于 JSON 格式)
	Caller    string    // 调用者信息 (文件名:行号)
}

// Config 用于配置 Logger
type Config struct {
	Level           Level      // 最低日志记录级别
	Mode            OutputMode // 输出模式 (Text 或 JSON)
	TimeFormat      string     // 时间戳格式 默认为 time_RFC3339Nano
	Output          io.Writer  // 日志输出目标 如果为 nil 且 FilePath 非空 则输出到文件
	FilePath        string     // 日志文件路径 (如果 Output 为 nil)
	EnableRotation  bool       // 是否启用日志轮转 (仅当 FilePath 有效时)
	MaxFileSizeMB   int64      // 单个日志文件最大大小 (MB) 默认 10MB
	MaxBackups      int        // 保留的旧日志文件数量 (不包括当前写入的文件) 默认 7
	CompressBackups bool       // 是否压缩备份的日志文件 默认 true
	Async           bool       // 是否启用异步写入 默认 true
	BufferSize      int        // 异步模式下的缓冲区大小 (条目数) 默认 8192
	CallerSkip      int        // runtime_Caller 的跳过层数 默认为 2 (适配直接调用实例方法)
	DefaultFields   Fields     // 每条日志都会附带的默认字段
	EnableCaller    bool       // 是否记录调用者信息 默认 false 开启会影响性能
}

// Logger 结构体
type Logger struct {
	config          Config
	level           atomic.Int32  // 当前日志级别
	outputMode      atomic.Uint32 // 当前输出模式
	mu              sync.Mutex    // 用于保护文件操作和 writer 切换等关键区段
	writer          io.Writer     // 最终的写入目标
	file            *os.File      // 如果输出到文件 这是文件句柄
	logChan         chan LogEntry // 异步日志通道
	shutdownChan    chan struct{} // 用于通知 worker 停止
	wg              sync.WaitGroup
	currentFileSize atomic.Int64 // 当前文件大小 用于轮转
	rotationTicker  *time.Ticker // 定期检查轮转的定时器
	droppedCount    atomic.Int64 // 记录因为缓冲区满而丢弃的日志数量
}

const (
	defaultTimeFormat      = time.RFC3339Nano // 默认时间格式使用纳秒精度
	defaultMaxFileSizeMB   = 10
	defaultMaxBackups      = 5
	defaultCompressBackups = true
	defaultAsync           = true
	defaultBufferSize      = 8192
	DefaultCallerSkip      = 2               // 默认跳过层数 指向logger实例方法的调用处
	rotationCheckInterval  = 5 * time.Minute // 日志轮转检查周期
)

// New 创建并返回一个新的 Logger 实例
// 调用者应该在不再使用 Logger 时调用其 Close 方法
func New(cfg Config) (*Logger, error) {
	// 设置配置默认值
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = defaultTimeFormat
	}
	if cfg.MaxFileSizeMB <= 0 {
		cfg.MaxFileSizeMB = defaultMaxFileSizeMB
	}
	if cfg.MaxBackups < 0 {
		cfg.MaxBackups = defaultMaxBackups
	}
	// BufferSize 必须为正数
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = defaultBufferSize
	}
	// CallerSkip 至少为2才能跳过 NewLogger 本身和日志方法
	// 如果用户设置了EnableCaller但CallerSkip不合理 则使用默认值
	if cfg.EnableCaller && cfg.CallerSkip < 1 {
		cfg.CallerSkip = DefaultCallerSkip
	}

	l := &Logger{
		config:       cfg,
		shutdownChan: make(chan struct{}),
	}
	l.level.Store(int32(cfg.Level))
	l.outputMode.Store(uint32(cfg.Mode))

	// 初始化输出 writer
	if cfg.Output != nil {
		l.writer = cfg.Output
	} else if cfg.FilePath != "" {
		// 确保日志目录存在
		dir := filepath.Dir(cfg.FilePath)
		if dir != "." && dir != "" { // 避免对当前目录或空目录创建
			if err := os.MkdirAll(dir, 0755); err != nil { // 使用 0755 权限创建目录
				return nil, fmt.Errorf("failed to create log directory: %w", err)
			}
		}

		file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file '%s': %w", cfg.FilePath, err)
		}
		l.file = file
		l.writer = file // 初始 writer 指向文件
		info, err := file.Stat()
		if err == nil {
			l.currentFileSize.Store(info.Size())
		}
		// 如果启用了文件轮转
		if cfg.EnableRotation {
			l.rotationTicker = time.NewTicker(rotationCheckInterval)
			l.wg.Add(1) // 用于 monitorRotation goroutine
			go l.monitorRotation()
		}
	} else {
		// 如果两者都未提供 默认输出到 os_Stdout
		// fmt_Fprintln(os_Stderr "Warning: No output specified defaulting to os_Stdout")
		l.writer = os.Stdout
	}

	// 如果启用异步处理
	if cfg.Async {
		l.logChan = make(chan LogEntry, cfg.BufferSize)
		l.wg.Add(1) // 用于 worker goroutine
		go l.worker()
	}

	return l, nil
}

// worker 是处理日志条目的后台 goroutine
func (l *Logger) worker() {
	defer l.wg.Done()
	for {
		select {
		case entry, ok := <-l.logChan:
			if !ok { // 通道已关闭 且已空
				return
			}
			l.performWrite(entry)
		case <-l.shutdownChan: // 收到关闭信号
			// 处理通道中剩余的日志
			for { // 循环读取直到 logChan 关闭并为空
				select {
				case entry, ok := <-l.logChan:
					if !ok {
						return // logChan 已关闭且为空
					}
					l.performWrite(entry)
				default:
					return // logChan 为空
				}
			}
		}
	}
}

// monitorRotation 定期检查日志文件大小并执行轮转
func (l *Logger) monitorRotation() {
	defer l.wg.Done()
	if l.rotationTicker == nil { // ticker 为 nil 说明未启用文件轮转或非文件输出
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

// checkAndRotate 检查文件大小并执行轮转 (如果需要)
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

// SetLevel 动态修改日志级别
func (l *Logger) SetLevel(level Level) {
	l.level.Store(int32(level))
}

// GetLevel 获取当前日志级别
func (l *Logger) GetLevel() Level {
	return Level(l.level.Load())
}

// SetOutputMode 动态修改输出模式
func (l *Logger) SetOutputMode(mode OutputMode) {
	l.outputMode.Store(uint32(mode))
}

// GetOutputMode 获取当前输出模式
func (l *Logger) GetOutputMode() OutputMode {
	return OutputMode(l.outputMode.Load())
}

// getCallerInfo 获取调用者信息
func (l *Logger) getCallerInfo() string {
	_, file, line, ok := runtime.Caller(l.config.CallerSkip)
	if !ok {
		return "???:0" // 返回更明确的未知信息
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

// submitEntry 提交日志条目到处理流程 (同步或异步)
func (l *Logger) submitEntry(level Level, message string, fields Fields) {
	currentLevel := Level(l.level.Load())
	if level < currentLevel { // 低于设定级别 不记录
		return
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Fields:    make(Fields), // 创建副本以防外部修改或复用
	}

	if l.config.DefaultFields != nil {
		for k, v := range l.config.DefaultFields {
			entry.Fields[k] = v
		}
	}
	if fields != nil {
		for k, v := range fields {
			entry.Fields[k] = v
		}
	}

	if l.config.EnableCaller { // 如果配置了记录调用者信息
		entry.Caller = l.getCallerInfo()
	}

	if l.config.Async {
		select {
		case <-l.shutdownChan: // 如果已关闭 则不再接受新日志
			l.droppedCount.Add(1)
			return
		default:
		}

		select {
		case l.logChan <- entry:
		default: // channel 满了
			l.droppedCount.Add(1)
		}
	} else {
		l.performWrite(entry)
	}
}

// performWrite 实际将 LogEntry 写入 l_writer
func (l *Logger) performWrite(entry LogEntry) {
	l.mu.Lock()
	currentWriter := l.writer
	l.mu.Unlock()

	if currentWriter == nil {
		return
	}

	var formattedMsg []byte
	var err error
	currentOutputMode := OutputMode(l.outputMode.Load())

	switch currentOutputMode {
	case ModeJSON:
		jsonData := make(map[string]interface{})
		if entry.Fields != nil {
			for k, v := range entry.Fields {
				jsonData[k] = v
			}
		}
		jsonData["timestamp"] = entry.Timestamp.Format(l.config.TimeFormat)
		jsonData["level"] = entry.Level.String()
		jsonData["message"] = entry.Message
		if entry.Caller != "" {
			jsonData["caller"] = entry.Caller
		}
		formattedMsg, err = json.Marshal(jsonData)
		if err == nil {
			formattedMsg = append(formattedMsg, '\n')
		}
	case ModeText:
		fallthrough
	default:
		var sb strings.Builder
		sb.WriteString(entry.Timestamp.Format(l.config.TimeFormat))
		sb.WriteString(" [")
		sb.WriteString(entry.Level.String())
		sb.WriteString("]")
		if entry.Caller != "" {
			sb.WriteString(" (")
			sb.WriteString(entry.Caller)
			sb.WriteString(")")
		}
		sb.WriteString(" ")
		sb.WriteString(entry.Message)
		if len(entry.Fields) > 0 {
			sb.WriteString(" {")
			first := true
			keys := make([]string, 0, len(entry.Fields))
			for k := range entry.Fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if !first {
					sb.WriteString(", ")
				}
				sb.WriteString(k)
				sb.WriteString("=")
				sb.WriteString(fmt.Sprintf("%#v", entry.Fields[k]))
				first = false
			}
			sb.WriteString("}")
		}
		sb.WriteString("\n")
		formattedMsg = []byte(sb.String())
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger: failed to format log entry: %v\n", err)
		errorMsg := fmt.Sprintf("%s [%s] %s (formatting error: %v)\n",
			entry.Timestamp.Format(l.config.TimeFormat), entry.Level.String(), entry.Message, err)
		formattedMsg = []byte(errorMsg)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.writer == nil {
		return
	}
	n, writeErr := l.writer.Write(formattedMsg)
	if writeErr != nil {
		fmt.Fprintf(os.Stderr, "Logger: failed to write log entry: %v\n", writeErr)
	}
	if l.file != nil && n > 0 {
		l.currentFileSize.Add(int64(n))
	}
}

// Logf 方法的实现 (格式化)
func (l *Logger) Debugf(format string, args ...interface{}) {
	if l.GetLevel() <= LevelDebug {
		l.submitEntry(LevelDebug, fmt.Sprintf(format, args...), nil)
	}
}
func (l *Logger) Infof(format string, args ...interface{}) {
	if l.GetLevel() <= LevelInfo {
		l.submitEntry(LevelInfo, fmt.Sprintf(format, args...), nil)
	}
}
func (l *Logger) Warnf(format string, args ...interface{}) {
	if l.GetLevel() <= LevelWarn {
		l.submitEntry(LevelWarn, fmt.Sprintf(format, args...), nil)
	}
}
func (l *Logger) Errorf(format string, args ...interface{}) {
	if l.GetLevel() <= LevelError {
		l.submitEntry(LevelError, fmt.Sprintf(format, args...), nil)
	}
}
func (l *Logger) Fatalf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.submitEntry(LevelFatal, msg, nil)
	l.Close()
	os.Exit(1)
}
func (l *Logger) Panicf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.submitEntry(LevelPanic, msg, nil)
	l.Close()
	panic(msg)
}

// 带 Fields 的日志方法 (非格式化)
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

// mergeFields 合并多个 Fields 对象 后面的会覆盖前面的同名key
func mergeFields(fieldArgs ...Fields) Fields {
	if len(fieldArgs) == 0 {
		return nil
	}
	if len(fieldArgs) == 1 && fieldArgs[0] != nil {
		merged := make(Fields, len(fieldArgs[0]))
		for k, v := range fieldArgs[0] {
			merged[k] = v
		}
		return merged
	}
	result := make(Fields)
	for _, f := range fieldArgs {
		if f != nil {
			for k, v := range f {
				result[k] = v
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// Close 关闭 Logger 确保所有缓冲的日志都被写入 (如果异步)
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
		if l.writer == l.file { // 这种比较可能不准确 如果 writer 是包装类型
			l.writer = nil
		} else if _, ok := l.writer.(io.Closer); ok && l.config.Output == nil {
			l.writer = nil
		}

	}
	dropped := l.droppedCount.Load()
	if dropped > 0 {
		fmt.Fprintf(os.Stderr, "Logger: %d log entries dropped due to buffer overflow during runtime.\n", dropped)
	}
	return closeError
}

// rotateFile 执行日志轮转
func (l *Logger) rotateFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil || !l.config.EnableRotation {
		return fmt.Errorf("rotation not enabled or not a file logger")
	}

	currentFilePath := l.config.FilePath
	if err := l.file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Logger: error closing current log file '%s' for rotation: %v attempting to continue\n", currentFilePath, err)
	}

	ext := filepath.Ext(currentFilePath)
	base := strings.TrimSuffix(currentFilePath, ext)
	timestamp := time.Now().Format("20060102_150405_000")
	backupPath := fmt.Sprintf("%s.%s%s", base, timestamp, ext)

	if err := os.Rename(currentFilePath, backupPath); err != nil {
		fmt.Fprintf(os.Stderr, "Logger: error renaming log file from '%s' to '%s': %v\n", currentFilePath, backupPath, err)
	}

	newFile, err := os.OpenFile(currentFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger: CRITICAL - failed to create new log file '%s' after rotation: %v Logging may be interrupted\n", currentFilePath, err)
		if l.writer != os.Stderr {
			l.writer = os.Stderr
			l.file = nil
		}
		return fmt.Errorf("failed to create new log file '%s' after rotation: %w", currentFilePath, err)
	}
	l.file = newFile
	l.writer = newFile
	l.currentFileSize.Store(0)

	if l.config.MaxBackups >= 0 {
		go l.cleanupAndCompressBackups(filepath.Dir(currentFilePath), filepath.Base(base), ext, backupPath)
	}
	return nil
}

// backupFileEntry 用于排序备份文件
type backupFileEntry struct {
	Path         string
	ModTime      time.Time
	IsCompressed bool
}

// cleanupAndCompressBackups 清理旧的备份并压缩
func (l *Logger) cleanupAndCompressBackups(dir, baseName, ext, backupPathJustRotated string) {
	globPattern := filepath.Join(dir, fmt.Sprintf("%s.*%s*", baseName, ext))
	files, err := filepath.Glob(globPattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger: error finding old log files for cleanup: %v\n", err)
		return
	}

	var backups []backupFileEntry
	for _, file := range files {
		if filepath.Base(file) == filepath.Base(l.config.FilePath) {
			continue
		}
		info, statErr := os.Stat(file)
		if statErr != nil {
			continue
		}
		isCompressed := strings.HasSuffix(file, ".tar.gz")
		backups = append(backups, backupFileEntry{Path: file, ModTime: info.ModTime(), IsCompressed: isCompressed})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime.After(backups[j].ModTime)
	})

	if l.config.CompressBackups {
		for i := len(backups) - 1; i >= 0; i-- {
			entry := backups[i]
			if !entry.IsCompressed && entry.Path != backupPathJustRotated {
				if len(backups) > l.config.MaxBackups && i < (len(backups)-l.config.MaxBackups) {
					go func(filePathToCompress string) {
						if err := compressLogFile(filePathToCompress); err == nil {
							if removeErr := os.Remove(filePathToCompress); removeErr != nil {
								fmt.Fprintf(os.Stderr, "Logger: failed to remove original log file '%s' after compression: %v\n", filePathToCompress, removeErr)
							}
						} else {
							fmt.Fprintf(os.Stderr, "Logger: failed to compress log file '%s': %v\n", filePathToCompress, err)
						}
					}(entry.Path)
				}
			}
		}
	}

	files, _ = filepath.Glob(globPattern)
	var currentBackups []backupFileEntry
	for _, file := range files {
		if filepath.Base(file) == filepath.Base(l.config.FilePath) {
			continue
		}
		info, statErr := os.Stat(file)
		if statErr != nil {
			continue
		}
		currentBackups = append(currentBackups, backupFileEntry{Path: file, ModTime: info.ModTime()})
	}
	sort.Slice(currentBackups, func(i, j int) bool {
		return currentBackups[i].ModTime.Before(currentBackups[j].ModTime)
	})

	if l.config.MaxBackups >= 0 && len(currentBackups) > l.config.MaxBackups {
		numToDelete := len(currentBackups) - l.config.MaxBackups
		for i := 0; i < numToDelete; i++ {
			fileToDelete := currentBackups[i].Path
			if err := os.Remove(fileToDelete); err != nil {
				fmt.Fprintf(os.Stderr, "Logger: failed to remove old log backup '%s': %v\n", fileToDelete, err)
			}
		}
	}
}

// compressLogFile 压缩指定的日志文件
func compressLogFile(srcPath string) error {
	dstPath := srcPath + ".tar.gz"
	if _, err := os.Stat(dstPath); err == nil {
		return fmt.Errorf("compressed file '%s' already exists not overwriting", dstPath)
	}
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file for compression '%s': %w", srcPath, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination archive '%s': %w", dstPath, err)
	}
	var compressSuccessful bool = false
	defer func() {
		dstFile.Close()
		if !compressSuccessful {
			if removeErr := os.Remove(dstPath); removeErr != nil {
				fmt.Fprintf(os.Stderr, "Logger: failed to remove incomplete archive '%s' after compression error: %v\n", dstPath, removeErr)
			}
		}
	}()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	srcFileInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info for '%s': %w", srcPath, err)
	}
	header := &tar.Header{
		Name:    filepath.Base(srcPath),
		Size:    srcFileInfo.Size(),
		Mode:    int64(srcFileInfo.Mode().Perm()),
		ModTime: srcFileInfo.ModTime(),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header for '%s': %w", srcPath, err)
	}
	if _, err := io.Copy(tarWriter, srcFile); err != nil {
		return fmt.Errorf("failed to copy data to tar archive for '%s': %w", srcPath, err)
	}
	compressSuccessful = true
	return nil
}

// DefaultLogger 是一个方便使用的默认 logger 实例
var DefaultLogger *Logger
var defaultLoggerOnce sync.Once

// GetDefaultLogger 返回一个默认配置的 Logger 实例 输出到 os_Stdout
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
			CallerSkip:   DefaultCallerSkip + 1, // 适配全局函数调用链 从GetDefaultLogger的调用处开始计算
		}
		var err error
		DefaultLogger, err = New(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "CRITICAL: Failed to initialize default logger: %v. Logging will be impaired.\n", err)
			DefaultLogger, _ = New(Config{Output: os.Stderr, Level: LevelError, EnableCaller: true, CallerSkip: DefaultCallerSkip + 1})
		}
	})
	return DefaultLogger
}

// InitDefaultLoggerWithConfig 允许用户自定义配置 DefaultLogger
func InitDefaultLoggerWithConfig(cfg Config) (*Logger, error) {
	var initError error
	if cfg.EnableCaller && cfg.CallerSkip < (DefaultCallerSkip+1) { // 确保全局函数调用适配
		cfg.CallerSkip = DefaultCallerSkip + 1
	}
	defaultLoggerOnce.Do(func() {
		var err error
		DefaultLogger, err = New(cfg)
		if err != nil {
			initError = fmt.Errorf("failed to initialize default logger with custom config: %w", err)
			fmt.Fprintf(os.Stderr, "%s\n", initError.Error())
			DefaultLogger, _ = New(Config{Output: os.Stderr, Level: LevelError, EnableCaller: true, CallerSkip: DefaultCallerSkip + 1})
		}
	})
	// 如果Do块已经被其他调用执行 cfg可能未被使用 此时initError为nil
	// 但DefaultLogger.config与传入的cfg可能不同
	// 这是一个设计上的权衡 一旦初始化后 DefaultLogger 不再改变
	return DefaultLogger, initError
}

// SetDefaultLogLevel 设置默认 logger 的级别
func SetDefaultLogLevel(level Level) {
	GetDefaultLogger().SetLevel(level)
}

// 全局日志函数 使用 DefaultLogger
func Debugf(format string, args ...interface{}) { GetDefaultLogger().Debugf(format, args...) }
func Infof(format string, args ...interface{})  { GetDefaultLogger().Infof(format, args...) }
func Warnf(format string, args ...interface{})  { GetDefaultLogger().Warnf(format, args...) }
func Errorf(format string, args ...interface{}) { GetDefaultLogger().Errorf(format, args...) }
func Fatalf(format string, args ...interface{}) { GetDefaultLogger().Fatalf(format, args...) }
func Panicf(format string, args ...interface{}) { GetDefaultLogger().Panicf(format, args...) }

func Debug(message string, fields ...Fields) { GetDefaultLogger().Debug(message, fields...) }
func Info(message string, fields ...Fields)  { GetDefaultLogger().Info(message, fields...) }
func Warn(message string, fields ...Fields)  { GetDefaultLogger().Warn(message, fields...) }
func Error(message string, fields ...Fields) { GetDefaultLogger().Error(message, fields...) }
func Fatal(message string, fields ...Fields) { GetDefaultLogger().Fatal(message, fields...) }
func Panic(message string, fields ...Fields) { GetDefaultLogger().Panic(message, fields...) }

// Close 关闭并刷新 DefaultLogger (如果已初始化)
// 建议在应用程序退出前调用此函数 以确保所有缓冲的日志都被处理完毕
func Close() error {
	// 直接访问包级变量 DefaultLogger
	// 如果 GetDefaultLogger() 或 InitDefaultLoggerWithConfig() 从未被有效调用过
	// DefaultLogger 将保持其零值 nil
	// 如果它们被调用并且成功初始化了 DefaultLogger 则 DefaultLogger 会指向一个实例
	loggerInstanceToClose := DefaultLogger

	if loggerInstanceToClose != nil {
		return loggerInstanceToClose.Close()
	}
	// 如果 DefaultLogger 为 nil (未初始化或初始化失败) 则无需操作
	return nil
}
