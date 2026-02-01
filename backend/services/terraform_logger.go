package services

import (
	"fmt"
	"iac-platform/internal/models"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	LogLevelTrace LogLevel = iota // 0 - 跟踪信息（最详细）
	LogLevelDebug                 // 1 - 调试信息
	LogLevelInfo                  // 2 - 一般信息
	LogLevelWarn                  // 3 - 警告信息
	LogLevelError                 // 4 - 错误信息
)

// TerraformLogger Terraform执行日志记录器
type TerraformLogger struct {
	stream          *OutputStream
	fullOutput      *strings.Builder
	fullOutputMutex sync.Mutex
	logLevel        LogLevel
	lineNum         int
	lineNumMutex    sync.Mutex
	printToConsole  bool // 是否打印到控制台（Agent 模式）
}

// NewTerraformLogger 创建日志记录器
func NewTerraformLogger(stream *OutputStream) *TerraformLogger {
	return &TerraformLogger{
		stream:     stream,
		fullOutput: &strings.Builder{},
		logLevel:   getLogLevelFromEnv(),
		lineNum:    0,
	}
}

// NewTerraformLoggerWithWorkspace 创建日志记录器（从workspace读取日志级别）
func NewTerraformLoggerWithWorkspace(stream *OutputStream, workspace interface{}) *TerraformLogger {
	return &TerraformLogger{
		stream:     stream,
		fullOutput: &strings.Builder{},
		logLevel:   getLogLevelFromWorkspace(workspace),
		lineNum:    0,
	}
}

// NewTerraformLoggerWithLevel 创建日志记录器（直接指定日志级别字符串）
func NewTerraformLoggerWithLevel(stream *OutputStream, levelStr string) *TerraformLogger {
	// 默认不打印到控制台（Local 模式）
	printToConsole := false

	return &TerraformLogger{
		stream:         stream,
		fullOutput:     &strings.Builder{},
		logLevel:       parseLogLevel(levelStr),
		lineNum:        0,
		printToConsole: printToConsole,
	}
}

// NewTerraformLoggerWithLevelAndMode 创建日志记录器（指定日志级别和执行模式）
func NewTerraformLoggerWithLevelAndMode(stream *OutputStream, levelStr string, isAgentMode bool) *TerraformLogger {
	// Agent 模式下始终打印到控制台
	printToConsole := isAgentMode

	return &TerraformLogger{
		stream:         stream,
		fullOutput:     &strings.Builder{},
		logLevel:       parseLogLevel(levelStr),
		lineNum:        0,
		printToConsole: printToConsole,
	}
}

// getLogLevelFromEnv 从环境变量获取日志级别
func getLogLevelFromEnv() LogLevel {
	level := os.Getenv("TF_LOG")
	switch strings.ToLower(level) {
	case "trace":
		return LogLevelTrace
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn":
		return LogLevelWarn
	case "error":
		return LogLevelError
	default:
		return LogLevelInfo // 默认INFO级别
	}
}

// getLogLevelFromWorkspace 从workspace配置获取日志级别
func getLogLevelFromWorkspace(workspace interface{}) LogLevel {
	// 类型断言
	ws, ok := workspace.(map[string]interface{})
	if !ok {
		// 尝试从models.Workspace类型读取
		if wsModel, ok := workspace.(*models.Workspace); ok {
			if wsModel.SystemVariables != nil {
				if tfLog, ok := wsModel.SystemVariables["TF_LOG"].(string); ok {
					return parseLogLevel(tfLog)
				}
			}
		}
		return getLogLevelFromEnv() // 回退到环境变量
	}

	// 从map读取
	if sysVars, ok := ws["system_variables"].(map[string]interface{}); ok {
		if tfLog, ok := sysVars["TF_LOG"].(string); ok {
			return parseLogLevel(tfLog)
		}
	}

	return getLogLevelFromEnv() // 回退到环境变量
}

// parseLogLevel 解析日志级别字符串
func parseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "trace":
		return LogLevelTrace
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn":
		return LogLevelWarn
	case "error":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

// log 记录日志
func (l *TerraformLogger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.logLevel {
		return // 跳过低于当前级别的日志
	}

	prefix := ""
	switch level {
	case LogLevelTrace:
		prefix = "[TRACE]"
	case LogLevelDebug:
		prefix = "[DEBUG]"
	case LogLevelInfo:
		prefix = "[INFO]"
	case LogLevelWarn:
		prefix = "[WARN]"
	case LogLevelError:
		prefix = "[ERROR]"
	}

	timestamp := time.Now().Format("15:04:05.000")
	message := fmt.Sprintf(format, args...)
	logLine := fmt.Sprintf("[%s] %s %s", timestamp, prefix, message)

	l.lineNumMutex.Lock()
	l.lineNum++
	currentLineNum := l.lineNum
	l.lineNumMutex.Unlock()

	// 广播到WebSocket
	if l.stream != nil {
		l.stream.Broadcast(OutputMessage{
			Type:      "output",
			Line:      logLine,
			Timestamp: time.Now(),
			LineNum:   currentLineNum,
		})
	}

	// 打印到控制台（Agent 模式）
	if l.printToConsole {
		fmt.Println(logLine)
	}

	// 保存到完整输出
	l.fullOutputMutex.Lock()
	l.fullOutput.WriteString(logLine)
	l.fullOutput.WriteString("\n")
	l.fullOutputMutex.Unlock()
}

// Trace 记录TRACE级别日志
func (l *TerraformLogger) Trace(format string, args ...interface{}) {
	l.log(LogLevelTrace, format, args...)
}

// Debug 记录DEBUG级别日志
func (l *TerraformLogger) Debug(format string, args ...interface{}) {
	l.log(LogLevelDebug, format, args...)
}

// Info 记录INFO级别日志
func (l *TerraformLogger) Info(format string, args ...interface{}) {
	l.log(LogLevelInfo, format, args...)
}

// Warn 记录WARN级别日志
func (l *TerraformLogger) Warn(format string, args ...interface{}) {
	l.log(LogLevelWarn, format, args...)
}

// Error 记录ERROR级别日志
func (l *TerraformLogger) Error(format string, args ...interface{}) {
	l.log(LogLevelError, format, args...)
}

// RawOutput 输出原始内容（不加前缀，用于terraform命令输出）
func (l *TerraformLogger) RawOutput(line string) {
	l.lineNumMutex.Lock()
	l.lineNum++
	currentLineNum := l.lineNum
	l.lineNumMutex.Unlock()

	// 广播到WebSocket
	if l.stream != nil {
		l.stream.Broadcast(OutputMessage{
			Type:      "output",
			Line:      line,
			Timestamp: time.Now(),
			LineNum:   currentLineNum,
		})
	}

	// 打印到控制台（Agent 模式）
	if l.printToConsole {
		fmt.Println(line)
	}

	// 保存到完整输出
	l.fullOutputMutex.Lock()
	l.fullOutput.WriteString(line)
	l.fullOutput.WriteString("\n")
	l.fullOutputMutex.Unlock()
}

// StageBegin 记录阶段开始
func (l *TerraformLogger) StageBegin(stage string) {
	timestamp := time.Now()
	marker := fmt.Sprintf("========== %s BEGIN at %s ==========",
		strings.ToUpper(stage),
		timestamp.Format("2006-01-02 15:04:05.000"))

	l.lineNumMutex.Lock()
	l.lineNum++
	currentLineNum := l.lineNum
	l.lineNumMutex.Unlock()

	// 广播阶段标记
	if l.stream != nil {
		l.stream.Broadcast(OutputMessage{
			Type:      "stage_marker",
			Line:      marker,
			Timestamp: timestamp,
			Stage:     stage,
			Status:    "begin",
			LineNum:   currentLineNum,
		})
	}

	// 打印到控制台（Agent 模式）
	if l.printToConsole {
		fmt.Println(marker)
	}

	// 保存到完整输出
	l.fullOutputMutex.Lock()
	l.fullOutput.WriteString(marker)
	l.fullOutput.WriteString("\n")
	l.fullOutputMutex.Unlock()
}

// StageEnd 记录阶段结束
func (l *TerraformLogger) StageEnd(stage string) {
	timestamp := time.Now()
	marker := fmt.Sprintf("========== %s END at %s ==========",
		strings.ToUpper(stage),
		timestamp.Format("2006-01-02 15:04:05.000"))

	l.lineNumMutex.Lock()
	l.lineNum++
	currentLineNum := l.lineNum
	l.lineNumMutex.Unlock()

	// 广播阶段标记
	if l.stream != nil {
		l.stream.Broadcast(OutputMessage{
			Type:      "stage_marker",
			Line:      marker,
			Timestamp: timestamp,
			Stage:     stage,
			Status:    "end",
			LineNum:   currentLineNum,
		})
	}

	// 打印到控制台（Agent 模式）
	if l.printToConsole {
		fmt.Println(marker)
	}

	// 保存到完整输出
	l.fullOutputMutex.Lock()
	l.fullOutput.WriteString(marker)
	l.fullOutput.WriteString("\n")
	l.fullOutputMutex.Unlock()
}

// LogError 记录详细错误
func (l *TerraformLogger) LogError(
	stage string,
	err error,
	context map[string]interface{},
	retryInfo *RetryInfo,
) {
	l.Error("========== %s FAILED at %s ==========",
		strings.ToUpper(stage),
		time.Now().Format("2006-01-02 15:04:05.000"))

	l.Error("Failed to %s", stage)
	l.Error("Error: %v", err)
	l.Error("")

	// 错误堆栈
	if stack := getStackTrace(); stack != "" {
		l.Error("Stack trace:")
		for _, line := range strings.Split(stack, "\n") {
			if line != "" {
				l.Error("  %s", line)
			}
		}
		l.Error("")
	}

	// 系统状态
	if context != nil && len(context) > 0 {
		l.Error("System state:")
		for key, value := range context {
			l.Error("  - %s: %v", key, value)
		}
		l.Error("")
	}

	// 重试信息
	if retryInfo != nil {
		l.Error("Retry information:")
		l.Error("  - Current attempt: %d/%d", retryInfo.CurrentAttempt, retryInfo.MaxRetries)
		l.Error("  - Next retry in: %v", retryInfo.NextRetryDelay)
		l.Error("  - Retry strategy: %s", retryInfo.Strategy)
	}

	l.Error("========== %s FAILED END ==========", strings.ToUpper(stage))
}

// GetFullOutput 获取完整输出
func (l *TerraformLogger) GetFullOutput() string {
	l.fullOutputMutex.Lock()
	defer l.fullOutputMutex.Unlock()
	return l.fullOutput.String()
}

// RetryInfo 重试信息
type RetryInfo struct {
	CurrentAttempt int
	MaxRetries     int
	NextRetryDelay time.Duration
	Strategy       string
}

// getStackTrace 获取堆栈跟踪
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}
