//go:build !windows
// +build !windows

package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Logger interface {
	Info(msg string)
	Error(msg string)
	Fatal(msg string, kv ...any)
	Sync()
}

type std struct {
	mu         sync.Mutex
	out        *log.Logger
	file       *os.File
	filePath   string
	maxBytes   int64
	maxBackups int
}

func New(level string) Logger {
	writer := io.Writer(os.Stderr)
	var file *os.File
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
			writer = io.MultiWriter(os.Stderr, f)
		}
	}
	return &std{
		out:        log.New(writer, "", log.LstdFlags),
		file:       file,
		filePath:   filePath,
		maxBytes:   maxBytes,
		maxBackups: maxBackups,
	}
}

func (s *std) Info(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rotateIfNeededLocked()
	s.out.Println("[INFO] " + msg)
}

func (s *std) Error(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rotateIfNeededLocked()
	s.out.Println("[ERROR] " + msg)
}

func (s *std) Fatal(msg string, kv ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rotateIfNeededLocked()
	s.out.Println("[FATAL] " + msg + " " + fmt.Sprint(kv...))
	os.Exit(1)
}

func (s *std) Sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
}

func (s *std) rotateIfNeededLocked() {
	if s.file == nil || s.filePath == "" || s.maxBytes <= 0 {
		return
	}
	info, err := s.file.Stat()
	if err != nil || info.Size() < s.maxBytes {
		return
	}
	_ = s.file.Close()
	rotated := s.filePath + "." + time.Now().Format("20060102-150405")
	_ = os.Rename(s.filePath, rotated)
	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Println("[WARN] log file reopen failed: " + err.Error())
		s.file = nil
		s.out = log.New(os.Stderr, "", log.LstdFlags)
		return
	}
	s.file = f
	s.out = log.New(io.MultiWriter(os.Stderr, f), "", log.LstdFlags)
	pruneBackups(s.filePath, s.maxBackups)
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
