package usecase

import (
	"strings"
	"sync"
	"time"
)

func NewCachedIdentityHeaderProvider(ttl time.Duration, build func() string) func() string {
	var mu sync.Mutex
	var cached string
	var expiresAt time.Time

	if ttl <= 0 {
		ttl = time.Hour
	}

	return func() string {
		if build == nil {
			return ""
		}
		now := time.Now()
		mu.Lock()
		defer mu.Unlock()
		if cached != "" && now.Before(expiresAt) {
			return cached
		}
		header := strings.TrimSpace(build())
		if header == "" {
			cached = ""
			expiresAt = time.Time{}
			return ""
		}
		cached = header
		expiresAt = now.Add(ttl)
		return cached
	}
}
