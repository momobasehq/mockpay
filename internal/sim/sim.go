// Package sim handles transaction simulation: random delays, failure injection,
// and asynchronous webhook delivery.
package sim

import (
	"bytes"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// Config controls simulation behaviour at runtime.
// Mutate via Update(); read via the accessor methods — all are goroutine-safe.
type Config struct {
	mu          sync.RWMutex
	FailureRate float64 // 0.0–1.0  (default 0.10 → 10 % of transactions fail)
	MinDelayMs  int     // lower bound for processing delay in milliseconds
	MaxDelayMs  int     // upper bound for processing delay in milliseconds (max 3000)
}

// Global is the default simulation config used by all handlers.
var Global = &Config{
	FailureRate: 0.10,
	MinDelayMs:  300,
	MaxDelayMs:  3000,
}

// Update atomically replaces the config values.
func (c *Config) Update(failureRate float64, minMs, maxMs int) {
	if maxMs > 3000 {
		maxMs = 3000
	}
	if minMs < 0 {
		minMs = 0
	}
	if failureRate < 0 {
		failureRate = 0
	}
	if failureRate > 1 {
		failureRate = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.FailureRate = failureRate
	c.MinDelayMs = minMs
	c.MaxDelayMs = maxMs
}

// Snapshot returns a copy of the current config values.
func (c *Config) Snapshot() (failureRate float64, minMs, maxMs int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.FailureRate, c.MinDelayMs, c.MaxDelayMs
}

// Delay returns a random duration in [MinDelayMs, MaxDelayMs].
func (c *Config) Delay() time.Duration {
	c.mu.RLock()
	lo, hi := c.MinDelayMs, c.MaxDelayMs
	c.mu.RUnlock()

	diff := hi - lo
	if diff <= 0 {
		return time.Duration(lo) * time.Millisecond
	}
	ms := lo + rand.Intn(diff+1)
	return time.Duration(ms) * time.Millisecond
}

// ShouldFail returns true with probability FailureRate.
// Pass force = "fail" or "success" to override the random outcome.
func (c *Config) ShouldFail(force string) bool {
	switch force {
	case "fail":
		return true
	case "success":
		return false
	}
	c.mu.RLock()
	rate := c.FailureRate
	c.mu.RUnlock()
	return rand.Float64() < rate
}

// FireWebhook asynchronously POSTs payload to callbackURL.
// It is a no-op when callbackURL is empty.
func FireWebhook(callbackURL string, payload any) {
	if callbackURL == "" {
		return
	}
	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Printf("[webhook] marshal error for %s: %v", callbackURL, err)
			return
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(callbackURL, "application/json", bytes.NewReader(data))
		if err != nil {
			log.Printf("[webhook] POST %s error: %v", callbackURL, err)
			return
		}
		defer resp.Body.Close()
		log.Printf("[webhook] POST %s → %d", callbackURL, resp.StatusCode)
	}()
}
