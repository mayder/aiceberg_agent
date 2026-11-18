package logger

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type discardLog struct{}

func (discardLog) Info(string)          {}
func (discardLog) Error(string)         {}
func (discardLog) Fatal(string, ...any) {}
func (discardLog) Sync()                {}

func TestSupportBufferBoundsRedactionAndRetention(t *testing.T) {
	b := WithSupportBuffer(discardLog{})
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	b.Info("initial service log")
	b.Error(strings.Repeat("sensitive-prefix", 2000) + " password=hidden token=other")
	text, _ := b.SupportLog()
	if strings.Contains(text, "sensitive-prefix") || strings.Contains(text, "hidden") || !strings.Contains(text, "[redacted sensitive line]") {
		t.Fatal("unsafe redaction")
	}
	for i := 0; i < 300; i++ {
		b.Info(fmt.Sprintf("entry-%03d %s", i, strings.Repeat("x", 300)))
	}
	text, truncated := b.SupportLog()
	if !truncated || len(text) > SupportLogBytes || len(b.entries) > SupportLogLines || strings.Contains(text, "initial service log") || !strings.Contains(text, "entry-299") {
		t.Fatal("buffer limits/ordering")
	}
	now = now.Add(SupportLogAge + time.Second)
	text, _ = b.SupportLog()
	if text != "" || b.bytes != 0 {
		t.Fatal("expired logs retained")
	}
}

func TestSupportBufferConcurrentReadersAndWriters(t *testing.T) {
	b := WithSupportBuffer(discardLog{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				b.Info("service record")
				text, _ := b.SupportLog()
				if len(text) > SupportLogBytes {
					t.Error("unbounded output")
				}
			}
		}()
	}
	wg.Wait()
}
