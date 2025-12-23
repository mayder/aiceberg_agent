//go:build windows
// +build windows

package logger

import (
	"fmt"
	"log"
	"os"
	"sync"

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
	mu   sync.Mutex
	elog *eventlog.Log
}

func New(_ string) Logger {
	source := os.Getenv("AICEBERG_EVENT_SOURCE")
	if source == "" {
		source = defaultEventSource
	}
	elog, err := eventlog.Open(source)
	if err != nil {
		log.Println("[WARN] eventlog open failed: " + err.Error())
	}
	return &eventLogger{elog: elog}
}

func (l *eventLogger) Info(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.elog != nil {
		_ = l.elog.Info(1, msg)
		return
	}
	log.Println("[INFO] " + msg)
}

func (l *eventLogger) Error(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.elog != nil {
		_ = l.elog.Error(1, msg)
		return
	}
	log.Println("[ERROR] " + msg)
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
}
