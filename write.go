package reco

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-json-experiment/json"
)

func (l *Logger) getCallerInfo() string {
	_, file, line, ok := runtime.Caller(l.config.CallerSkip)
	if !ok {
		return "???:0"
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

func (l *Logger) submitEntry(level Level, message string, fields Fields) {
	if level < l.GetLevel() {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Fields:    make(Fields),
	}

	if l.config.DefaultFields != nil {
		for k, v := range l.config.DefaultFields {
			entry.Fields[k] = v
		}
	}
	if fields != nil {
		maps.Copy(entry.Fields, fields)
	}

	if l.config.EnableCaller {
		entry.Caller = l.getCallerInfo()
	}

	if l.config.Async {
		select {
		case <-l.shutdownChan:
			l.droppedCount.Add(1)
			return
		default:
		}

		select {
		case l.logChan <- entry:
		default:
			l.droppedCount.Add(1)
		}
	} else {
		l.performWrite(entry)
	}
}

func (l *Logger) performWrite(entry LogEntry) {
	var formattedMsg []byte
	var err error
	currentOutputMode := l.GetOutputMode()

	switch currentOutputMode {
	case ModeJSON:
		formattedMsg, err = l.formatJSON(entry)
	case ModeText:
		fallthrough
	default:
		formattedMsg = l.formatText(entry)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger: failed to format log entry: %v\n", err)
		errorMsg := fmt.Sprintf("%s [%s] %s (formatting error: %v)\n",
			entry.Timestamp.Format(l.config.TimeFormat), entry.Level.String(), entry.Message, err)
		formattedMsg = []byte(errorMsg)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	writer, ok := l.writer.(io.Writer)
	if !ok || writer == nil {
		return
	}

	n, writeErr := writer.Write(formattedMsg)
	if writeErr != nil {
		fmt.Fprintf(os.Stderr, "Logger: failed to write log entry: %v\n", writeErr)
	}
	if l.file != nil && n > 0 {
		l.currentFileSize.Add(int64(n))
	}
}

func (l *Logger) formatJSON(entry LogEntry) ([]byte, error) {
	data := make(map[string]any, len(entry.Fields)+4)
	for k, v := range entry.Fields {
		if d, ok := v.(time.Duration); ok {
			data[k] = formatDuration(d)
		} else {
			data[k] = v
		}
	}
	data["timestamp"] = entry.Timestamp.Format(l.config.TimeFormat)
	data["level"] = entry.Level.String()
	data["message"] = entry.Message
	if entry.Caller != "" {
		data["caller"] = entry.Caller
	}

	b, err := json.Marshal(data)
	if err == nil {
		b = append(b, '\n')
	}
	return b, err
}

func (l *Logger) formatText(entry LogEntry) []byte {
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
		keys := make([]string, 0, len(entry.Fields))
		for k := range entry.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(k)
			sb.WriteString("=")
			val := entry.Fields[k]
			if d, ok := val.(time.Duration); ok {
				sb.WriteString(formatDuration(d))
			} else {
				sb.WriteString(fmt.Sprintf("%#v", val))
			}
		}
		sb.WriteString("}")
	}
	sb.WriteString("\n")
	return []byte(sb.String())
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%.2fµs", float64(d.Nanoseconds())/1000)
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1000000)
	case d < time.Minute:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%.2fm", d.Minutes())
	default:
		return fmt.Sprintf("%.2fh", d.Hours())
	}
}

func mergeFields(fieldArgs ...Fields) Fields {
	if len(fieldArgs) == 0 {
		return nil
	}
	if len(fieldArgs) == 1 {
		return fieldArgs[0]
	}
	result := make(Fields)
	for _, f := range fieldArgs {
		if f != nil {
			maps.Copy(result, f)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
