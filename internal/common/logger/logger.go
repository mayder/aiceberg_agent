package logger

import (
	"log"
	"os"
)

type Logger interface {
	Info(msg string)
	Error(msg string)
	Fatal(msg string, kv ...any)
	Sync()
}

type std struct{}

func New(_ string) Logger { return &std{} }

func (s *std) Info(msg string)  { log.Println("[INFO] " + msg) }
func (s *std) Error(msg string) { log.Println("[ERROR] " + msg) }
func (s *std) Fatal(msg string, kv ...any) {
	log.Println("[FATAL] " + msg)
	os.Exit(1)
}
func (s *std) Sync() {}
