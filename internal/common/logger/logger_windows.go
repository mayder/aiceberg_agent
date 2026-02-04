//go:build windows
// +build windows

package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc/eventlog"
)

const defaultEventSource = "AIcebergAgent"

type Logger interface {
	Info(msg string)
	Error(msg string)
	Fatal(msg string, kv ...any)
	Sync()
}

type eventLogger struct {
	mu         sync.Mutex
	elog       *eventlog.Log
	file       *os.File
	fileLog    *log.Logger
	filePath   string
	maxBytes   int64
	maxBackups int
}

func New(level string) Logger {
	source := os.Getenv("AICEBERG_EVENT_SOURCE")
	if source == "" {
		source = defaultEventSource
	}
	elog, err := eventlog.Open(source)
	if err != nil {
		log.Println("[WARN] eventlog open failed: " + err.Error())
	}
	var file *os.File
	var fileLog *log.Logger
	var filePath string
	var maxBytes int64
	var maxBackups int
	if strings.EqualFold(level, "debug") {
		path := os.Getenv("LOG_FILE_PATH")
		if path == "" {
			path = "./data/agent.debug.log"
		}
		filePath = path
		maxBytes = int64(intEnv("LOG_FILE_MAX_MB", 10)) * 1024 * 1024
		maxBackups = intEnv("LOG_FILE_MAX_BACKUPS", 3)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Println("[WARN] log file open failed: " + err.Error())
		} else {
			file = f
			fileLog = log.New(f, "", log.LstdFlags)
		}
	}
	return &eventLogger{
		elog:       elog,
		file:       file,
		fileLog:    fileLog,
		filePath:   filePath,
		maxBytes:   maxBytes,
		maxBackups: maxBackups,
	}
}

func (l *eventLogger) Info(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.elog != nil {
		_ = l.elog.Info(1, msg)
	} else {
		log.Println("[INFO] " + msg)
	}
	if l.fileLog != nil {
		l.rotateIfNeededLocked()
		l.fileLog.Println("[INFO] " + msg)
	}
}

func (l *eventLogger) Error(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.elog != nil {
		_ = l.elog.Error(1, msg)
	} else {
		log.Println("[ERROR] " + msg)
	}
	if l.fileLog != nil {
		l.rotateIfNeededLocked()
		l.fileLog.Println("[ERROR] " + msg)
	}
}

func (l *eventLogger) Fatal(msg string, kv ...any) {
	if len(kv) > 0 {
		l.Error(msg + " " + fmt.Sprint(kv...))
	} else {
		l.Error(msg)
	}
	os.Exit(1)
}

func (l *eventLogger) Sync() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.elog != nil {
		_ = l.elog.Close()
		l.elog = nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	l.fileLog = nil
}

func (l *eventLogger) rotateIfNeededLocked() {
	if l.file == nil || l.filePath == "" || l.maxBytes <= 0 {
		return
	}
	info, err := l.file.Stat()
	if err != nil || info.Size() < l.maxBytes {
		return
	}
	_ = l.file.Close()
	rotated := l.filePath + "." + time.Now().Format("20060102-150405")
	_ = os.Rename(l.filePath, rotated)
	f, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Println("[WARN] log file reopen failed: " + err.Error())
		l.file = nil
		l.fileLog = nil
		return
	}
	l.file = f
	l.fileLog = log.New(f, "", log.LstdFlags)
	pruneBackups(l.filePath, l.maxBackups)
}

func pruneBackups(path string, maxBackups int) {
	if maxBackups <= 0 {
		return
	}
	matches, err := filepath.Glob(path + ".*")
	if err != nil || len(matches) <= maxBackups {
		return
	}
	type item struct {
		path string
		mod  time.Time
	}
	items := make([]item, 0, len(matches))
	for _, p := range matches {
		if info, err := os.Stat(p); err == nil {
			items = append(items, item{path: p, mod: info.ModTime()})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.Before(items[j].mod) })
	if len(items) <= maxBackups {
		return
	}
	for _, it := range items[:len(items)-maxBackups] {
		_ = os.Remove(it.path)
	}
}

func intEnv(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
