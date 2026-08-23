// Package sim handles transaction simulation: random delays, failure injection,
// and asynchronous webhook delivery.
package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
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
func (c *Config) ShouldFail() bool {
	c.mu.RLock()
	rate := c.FailureRate
	c.mu.RUnlock()
	return rand.Float64() < rate
}

// PendingWebhook describes a webhook delivery that is currently in flight.
type PendingWebhook struct {
	ID        string          `json:"id"`
	URL       string          `json:"url"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"createdAt"`
}

type pendingWebhookEntry struct {
	webhook PendingWebhook
	cancel  context.CancelFunc
}

var webhookQueue = struct {
	sync.RWMutex
	items map[string]pendingWebhookEntry
}{items: make(map[string]pendingWebhookEntry)}

// PendingWebhooks returns a snapshot of webhook deliveries currently in flight.
func PendingWebhooks() []PendingWebhook {
	webhookQueue.RLock()
	items := make([]PendingWebhook, 0, len(webhookQueue.items))
	for _, entry := range webhookQueue.items {
		item := entry.webhook
		item.Payload = append(json.RawMessage(nil), item.Payload...)
		items = append(items, item)
	}
	webhookQueue.RUnlock()

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

// ResetPendingWebhooks cancels and removes all webhook deliveries in flight.
func ResetPendingWebhooks() {
	webhookQueue.Lock()
	cancels := make([]context.CancelFunc, 0, len(webhookQueue.items))
	for _, entry := range webhookQueue.items {
		cancels = append(cancels, entry.cancel)
	}
	webhookQueue.items = make(map[string]pendingWebhookEntry)
	webhookQueue.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

// FireWebhook asynchronously POSTs payload to callbackURL.
// It is a no-op when callbackURL is empty.
func FireWebhook(callbackURL string, payload any) {
	if callbackURL == "" {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[webhook] marshal error for %s: %v", callbackURL, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	id := uuid.NewString()
	webhookQueue.Lock()
	webhookQueue.items[id] = pendingWebhookEntry{
		webhook: PendingWebhook{
			ID:        id,
			URL:       callbackURL,
			Payload:   append(json.RawMessage(nil), data...),
			CreatedAt: time.Now(),
		},
		cancel: cancel,
	}
	webhookQueue.Unlock()

	go func() {
		defer func() {
			cancel()
			webhookQueue.Lock()
			delete(webhookQueue.items, id)
			webhookQueue.Unlock()
		}()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(data))
		if err != nil {
			log.Printf("[webhook] create request for %s error: %v", callbackURL, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[webhook] POST %s error: %v", callbackURL, err)
			return
		}
		defer resp.Body.Close()
		log.Printf("[webhook] POST %s → %d", callbackURL, resp.StatusCode)
	}()
}
