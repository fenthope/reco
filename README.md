

## 许可证

基于 WJQSERVER-STUDIO/logger 构建, 继承许可证

[![GoDoc](https://godoc.org/github.com/fenthope/reco?status.svg)](https://godoc.org/github.com/fenthope/reco) ![Go Version](https://img.shields.io/badge/go-%3E%3D1.18-brightgreen.svg) ![License](https://img.shields.io/badge/license-MPL-blue.svg)

`reco` 是一个为 Go 应用程序设计的高性能、可配置的日志库。它支持多种日志级别、文本和 JSON 格式输出、文件轮转、日志压缩以及异步写入等功能，旨在提供灵活且高效的日志解决方案。

## 特性概览

*   **多级别日志：** 支持 `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`, `PANIC` 等日志级别。
*   **两种输出模式：** 可选择传统的**文本格式**或更适合机器解析的**JSON 格式**。
*   **文件输出与轮转：** 可将日志写入指定文件，并支持按文件大小自动轮转，防止单个日志文件过大。
*   **备份压缩：** 轮转后的旧日志文件可选择自动压缩为 `tar.gz` 格式，节省磁盘空间。
*   **异步写入：** 默认启用异步写入机制，将日志写入操作从主业务逻辑中分离，减少对应用性能的影响。
*   **结构化日志：** 支持通过 `Fields` 类型添加任意键值对，使日志信息更丰富、易于查询（尤其在 JSON 模式下）。
*   **调用者信息：** 可配置记录日志调用发生时的文件名和行号，方便定位问题。
*   **默认字段：** 支持为所有日志条目添加统一的默认字段。
*   **全局默认 Logger：** 提供一个易于上手的默认 `Logger` 实例，方便快速集成。
*   **安全关闭：** 提供 `Close()` 方法，确保在程序退出前所有缓冲的日志都被写入，避免数据丢失。

## 快速开始

### 安装

```bash
go get github.com/fenthope/reco
```

### 基本使用

#### 使用默认 Logger

`reco` 提供了一个全局的 `DefaultLogger` 实例，方便快速进行日志记录。

```go
package main

import (
	"fmt"
	"time"

	"github.com/fenthope/reco"
)

func main() {
	// 在程序退出前调用 Close() 确保所有日志被写入
	// 对于异步日志，这尤其重要
	defer reco.Close() 

	// 使用默认的 INFO 级别文本日志输出到 os.Stdout
	reco.Info("这是一个默认的信息日志")
	reco.Debug("默认日志级别是 INFO，所以这条调试日志不会显示。")
	reco.Warnf("这是一个格式化的警告日志：请求ID %d", 123)

	// 动态修改默认 Logger 的级别
	reco.SetDefaultLogLevel(reco.LevelDebug)
	reco.Debug("现在调试日志可以显示了。")

	// 记录带结构化字段的日志
	reco.Info("用户登录成功", reco.Fields{"user_id": "alice", "ip_address": "192.168.1.100"})

	// 模拟一些工作，让异步日志有时间处理
	time.Sleep(100 * time.Millisecond)

	fmt.Println("日志记录完成。")
}
```

#### 创建自定义 Logger 实例

您可以根据需求创建多个独立的 `Logger` 实例，每个实例拥有自己的配置。

```go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/fenthope/reco"
)

func main() {
	// 1. 配置 Logger
	cfg := reco.Config{
		Level:           reco.LevelDebug,       // 最低记录级别为 DEBUG
		Mode:            reco.ModeJSON,         // 输出 JSON 格式
		FilePath:        "application.log",     // 日志文件路径
		EnableRotation:  true,                  // 启用文件轮转
		MaxFileSizeMB:   5,                     // 单个文件最大 5MB
		MaxBackups:      3,                     // 保留 3 个旧备份
		CompressBackups: true,                  // 压缩旧备份
		Async:           true,                  // 启用异步写入 (默认)
		BufferSize:      4096,                  // 异步缓冲区大小
		EnableCaller:    true,                  // 启用调用者信息
		CallerSkip:      2,                     // 调整调用栈跳过层数，以指向实际调用日志方法的位置
		DefaultFields:   reco.Fields{"service": "my-app", "env": "dev"}, // 默认字段
	}

	// 2. 创建 Logger 实例
	logger, err := reco.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close() // 确保在程序退出时关闭 logger

	// 3. 使用自定义 Logger 记录日志
	logger.Debug("这是一个调试信息", reco.Fields{"component": "database"})
	logger.Info("应用程序启动成功", reco.Fields{"version": "1.0.0"})
	logger.Warnf("配置项 '%s' 已被废弃", "old_feature")
	logger.Error("处理请求失败", reco.Fields{"request_id": "abc-123", "error_code": 500})

	// 模拟一些工作，让异步日志有时间处理
	time.Sleep(200 * time.Millisecond)

	fmt.Println("自定义日志记录完成。")
}
```

## 配置选项 (reco.Config)

| 字段名           | 类型          | 默认值           | 描述                                                                               |
| :--------------- | :------------ | :--------------- | :--------------------------------------------------------------------------------- |
| `Level`          | `reco.Level`  | `reco.LevelInfo` | 最低日志记录级别。低于此级别的日志将不会被输出。                                   |
| `Mode`           | `reco.OutputMode` | `reco.ModeText`  | 日志输出格式：`reco.ModeText` (文本) 或 `reco.ModeJSON` (JSON)。                   |
| `TimeFormat`     | `string`      | `time.RFC3339Nano` | 时间戳格式。遵循 Go `time.Format` 格式字符串规范。                               |
| `Output`         | `io.Writer`   | `nil`            | 日志的直接输出目标。如果设置，`FilePath` 将被忽略。若 `FilePath` 也为空，则默认输出到 `os.Stdout`。 |
| `FilePath`       | `string`      | `""`             | 日志文件路径。当 `Output` 为 `nil` 时生效。                                       |
| `EnableRotation` | `bool`        | `false`          | 是否启用日志文件轮转。仅当 `FilePath` 有效时生效。                                |
| `MaxFileSizeMB`  | `int64`       | `10`             | 单个日志文件的最大大小（MB）。达到此大小将触发轮转。                               |
| `MaxBackups`     | `int`         | `5`              | 保留的旧日志文件数量（不包括当前正在写入的文件）。设置为 `0` 表示不保留备份。    |
| `CompressBackups`| `bool`        | `true`           | 轮转后的旧日志文件是否压缩为 `tar.gz` 格式。                                       |
| `Async`          | `bool`        | `true`           | 是否启用异步写入。启用后日志写入操作将在后台 goroutine 中进行。                      |
| `BufferSize`     | `int`         | `8192`           | 异步模式下日志缓冲区的最大容量（日志条目数量）。缓冲区满时，新的日志会被丢弃。  |
| `CallerSkip`     | `int`         | `2`              | `runtime.Caller` 的跳过层数，用于定位实际调用日志方法的代码行。对于直接调用 `logger.Info` 等方法，通常设置为 `2`。对于通过全局函数 `reco.Info` 调用的情况，需要设置为 `3`。 |
| `DefaultFields`  | `reco.Fields` | `nil`            | 每次日志记录都会自动附带的默认结构化字段。                                       |
| `EnableCaller`   | `bool`        | `false`          | 是否在日志中记录调用者（文件名:行号）信息。开启会带来轻微性能开销。                 |

## 日志级别 (reco.Level)

*   `reco.LevelDebug`: 调试信息，用于开发阶段的详细输出。
*   `reco.LevelInfo`: 普通信息，记录应用程序的常规运行状态。
*   `reco.LevelWarn`: 警告信息，可能表示潜在问题或非预期情况。
*   `reco.LevelError`: 错误信息，指示程序运行时发生了错误，但可能不影响程序的继续执行。
*   `reco.LevelFatal`: 致命错误，记录后会调用 `os.Exit(1)` 导致程序终止。
*   `reco.LevelPanic`: Panic 错误，记录后会调用 `panic()` 导致程序崩溃（可被 recover 捕获）。
*   `reco.levelNone`: 特殊级别，设置为此级别将关闭所有日志输出。

## 输出模式 (reco.OutputMode)

*   `reco.ModeText`: 传统的文本格式日志。
    ```
    2023-10-27T10:30:00.123456789+08:00 [INFO] (main.go:42) 应用程序启动成功 {version="1.0.0"}
    ```
*   `reco.ModeJSON`: JSON 格式日志，方便日志聚合系统解析。
    ```json
    {"timestamp":"2023-10-27T10:30:00.123456789+08:00","level":"INFO","message":"应用程序启动成功","caller":"main.go:42","version":"1.0.0"}
    ```

## 高级用法

### 动态调整日志级别和输出模式

在程序运行时，可以随时更改 `Logger` 的日志级别和输出模式。

```go
package main

import (
	"fmt"
	"time"

	"github.com/fenthope/reco"
)

func main() {
	logger, _ := reco.New(reco.Config{Output: os.Stdout, Level: reco.LevelInfo})
	defer logger.Close()

	logger.Info("初始级别：INFO")
	logger.Debug("这条不会显示")

	logger.SetLevel(reco.LevelDebug)
	logger.Debug("级别调整为 DEBUG，这条显示了！")

	logger.SetOutputMode(reco.ModeJSON)
	logger.Info("现在是 JSON 格式了", reco.Fields{"format": "json"})

	time.Sleep(100 * time.Millisecond)
}
```

### 日志文件轮转和清理

当 `EnableRotation` 为 `true` 且 `FilePath` 有效时，`reco` 会在后台定期检查日志文件大小，并触发轮转。轮转后的文件会根据 `MaxBackups` 和 `CompressBackups` 进行清理和压缩。

备份文件名格式为 `[原始文件名].[时间戳].log[.tar.gz]`，例如 `application.log.20231027_103000_123.log.tar.gz`。

### 优雅关闭

在应用程序退出前，**务必调用 `Logger.Close()` 方法**，特别是当您使用异步写入时。`Close()` 会确保日志缓冲区中的所有待处理日志都被写入到目标 `io.Writer` 或文件中，防止日志丢失。

```go
func main() {
    logger, _ := reco.New(...)
    defer logger.Close() // 确保在 main 函数退出时调用
    // ... 你的应用逻辑
}
```

或者使用全局 `reco.Close()` 函数：

```go
func main() {
    defer reco.Close() // 关闭 DefaultLogger
    // ...
}
```

## 注意事项

*   **性能考量：**
    *   启用 `EnableCaller` 会在每次日志记录时增加少量的反射开销，可能会对高并发场景下的性能产生轻微影响。
    *   异步写入 (`Async: true`) 显著提高了日志记录的吞吐量，但如果日志量超出 `BufferSize` 且处理速度跟不上，会导致日志被丢弃（`droppedCount` 会记录）。
    *   `DefaultFields` 和 `Fields` 的添加会增加日志格式化的开销，尤其是在文本模式下。
*   **`Fatal`/`Panic` 级别：** 这两个级别会强制终止程序。`Fatal` 会调用 `os.Exit(1)`，`Panic` 会调用 `panic()`。在调用这些方法之前，它们会尝试关闭 `Logger` 以刷新日志。
*   **`CallerSkip` 配置：** 这是一个比较棘手的参数。它决定了 `runtime.Caller` 函数在调用栈中跳过多少层来获取正确的调用者信息。
    *   当**直接调用 `logger.Debug(...)` 等方法**时，`CallerSkip` 通常需要设置为 `2`。
    *   当通过**全局函数 `reco.Debug(...)` 等方法**调用时，由于多了一层函数调用，`CallerSkip` 需要设置为 `3`。
    *   `reco.New()` 默认设置 `CallerSkip` 为 `2`。
    *   `reco.GetDefaultLogger()` 和 `reco.InitDefaultLoggerWithConfig()` 会根据内部逻辑自动调整 `CallerSkip` 为 `3`。如果您自定义 `Config.CallerSkip`，请确保理解其含义。
*   **并发安全：** `reco` 库本身是并发安全的，可以在多个 goroutine 中同时使用 `Logger` 实例进行日志记录。

## 许可证

基于 [WJQSERVER-STUDIO/logger](https://github.com/WJQSERVER-STUDIO/logger) 改进而来, 继承MPL 2.0许可证