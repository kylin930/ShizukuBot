package core

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type LogEntry struct {
	Timestamp time.Time
	BotID     string
	Level     LogLevel
	Message   string
}

type LogManager struct {
	history     []LogEntry
	maxHistory  int
	subscribers []chan<- LogEntry
	mu          sync.RWMutex
	minLevel    LogLevel
}

var GlobalLogManager *LogManager

func NewLogManager(maxHistory int, minLevel LogLevel) *LogManager {
	mgr := &LogManager{
		maxHistory: maxHistory,
		minLevel:   minLevel,
	}
	GlobalLogManager = mgr
	return mgr
}

func (lm *LogManager) AddLog(botID string, level LogLevel, message string) {
	if level < lm.minLevel {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		BotID:     botID,
		Level:     level,
		Message:   message,
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.history = append(lm.history, entry)
	if len(lm.history) > lm.maxHistory {
		lm.history = lm.history[len(lm.history)-lm.maxHistory:]
	}

	for _, ch := range lm.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
}

func (lm *LogManager) Debug(botID, message string) {
	lm.AddLog(botID, LogLevelDebug, message)
}

func (lm *LogManager) Info(botID, message string) {
	lm.AddLog(botID, LogLevelInfo, message)
}

func (lm *LogManager) Warn(botID, message string) {
	lm.AddLog(botID, LogLevelWarn, message)
}

func (lm *LogManager) Error(botID, message string) {
	lm.AddLog(botID, LogLevelError, message)
}

func (lm *LogManager) GetHistory(botID string, limit int) []LogEntry {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	var result []LogEntry
	for i := len(lm.history) - 1; i >= 0 && len(result) < limit; i-- {
		if botID == "" || lm.history[i].BotID == botID {
			result = append(result, lm.history[i])
		}
	}

	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result
}

func (lm *LogManager) Subscribe(bufferSize int) <-chan LogEntry {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	ch := make(chan LogEntry, bufferSize)
	lm.subscribers = append(lm.subscribers, ch)
	return ch
}

func (lm *LogManager) Unsubscribe(ch <-chan LogEntry) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	chVal := reflect.ValueOf(ch)
	for i, subscriber := range lm.subscribers {
		subVal := reflect.ValueOf(subscriber)
		if chVal.Pointer() == subVal.Pointer() {
			lm.subscribers = append(lm.subscribers[:i], lm.subscribers[i+1:]...)
			close(subscriber)
			break
		}
	}
}

func (lm *LogManager) SaveLogsToFile() error {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	filename := filepath.Join(logDir, fmt.Sprintf("shizukubot_%s.log", time.Now().Format("20060102_150405")))
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer file.Close()

	for _, entry := range lm.history {
		botID := entry.BotID
		if botID == "" {
			botID = "-"
		}
		line := fmt.Sprintf("%s [%s] [%s] %s\n",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			botID,
			entry.Level.String(),
			entry.Message,
		)
		if _, err := file.WriteString(line); err != nil {
			return fmt.Errorf("failed to write log line: %w", err)
		}
	}

	return nil
}

type LogWriter struct {
	inner *os.File
}

func NewLogWriter(f *os.File) *LogWriter {
	return &LogWriter{inner: f}
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	message := string(p)
	if len(message) > 0 && message[len(message)-1] == '\n' {
		message = message[:len(message)-1]
	}
	if GlobalLogManager != nil {
		GlobalLogManager.Info("", message)
	}
	return w.inner.Write(p)
}