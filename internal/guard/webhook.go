package guard

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

type webhookQueue struct {
	g    *Guard
	ch   chan Signal
	stop chan struct{}
	once sync.Once
	mu   sync.Mutex
	last map[string]time.Time
	http *http.Client
}

func newWebhookQueue(g *Guard) *webhookQueue {
	q := &webhookQueue{
		g:    g,
		ch:   make(chan Signal, 256),
		stop: make(chan struct{}),
		last: make(map[string]time.Time),
		http: &http.Client{Timeout: 5 * time.Second},
	}
	go q.loop()
	return q
}

func (q *webhookQueue) Enqueue(s Signal) {
	if q == nil {
		return
	}
	select {
	case q.ch <- s:
	default:
		// The queue is full; dropping is safer than blocking the request path.
	}
}

func (q *webhookQueue) Close() {
	if q == nil {
		return
	}
	q.once.Do(func() { close(q.stop) })
}

func (q *webhookQueue) loop() {
	for {
		select {
		case <-q.stop:
			return
		case s := <-q.ch:
			q.send(s)
		}
	}
}

func (q *webhookQueue) send(s Signal) {
	cfg := q.g.cfg()
	if cfg == nil || !cfg.Guard.Alerts.Webhook.Enabled || cfg.Guard.Alerts.Webhook.URL == "" {
		return
	}
	key := s.Code + "|" + s.AuthID + "|" + s.Model
	cooldown := time.Duration(cfg.Guard.Alerts.CooldownSeconds) * time.Second
	q.mu.Lock()
	if t, ok := q.last[key]; ok && time.Since(t) < cooldown {
		q.mu.Unlock()
		return
	}
	q.last[key] = time.Now()
	if len(q.last) > 4096 {
		cut := time.Now().Add(-24 * time.Hour)
		for k, t := range q.last {
			if t.Before(cut) {
				delete(q.last, k)
			}
		}
	}
	q.mu.Unlock()

	payload, err := json.Marshal(s)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, cfg.Guard.Alerts.Webhook.URL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("relaywarden webhook: build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "relaywarden/"+Version)
	resp, err := q.http.Do(req)
	if err != nil {
		log.Printf("relaywarden webhook: post: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("relaywarden webhook: status %d", resp.StatusCode)
	}
}

// Version is the plugin version reported in metadata and webhooks.
const Version = "0.1.0"
