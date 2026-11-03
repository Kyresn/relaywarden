// Package metrics aggregates compression and request counters.
package metrics

import (
	"sort"
	"sync"

	"cpa-guard/internal/protocol"
)

type Counter struct {
	Requests    int64 `json:"requests"`
	Blocks      int64 `json:"blocks"`
	BytesBefore int64 `json:"bytes_before"`
	BytesAfter  int64 `json:"bytes_after"`
}

func (c Counter) SavedBytes() int64 { return c.BytesBefore - c.BytesAfter }

func (c Counter) SavingsPercent() float64 {
	if c.BytesBefore <= 0 {
		return 0
	}
	return float64(c.SavedBytes()) / float64(c.BytesBefore) * 100
}

func (c Counter) SavedTokensEstimate() int64 { return c.SavedBytes() / 4 }

type NamedCounter struct {
	Name string `json:"name"`
	Counter
}

type Store struct {
	mu       sync.Mutex
	total    Counter
	shadow   Counter
	byFilter map[string]*Counter
	byModel  map[string]*Counter
	byFormat map[string]*Counter
}

func New() *Store {
	return &Store{
		byFilter: make(map[string]*Counter),
		byModel:  make(map[string]*Counter),
		byFormat: make(map[string]*Counter),
	}
}

// RecordRequest counts one intercepted request that reached the plugin.
func (s *Store) RecordRequest() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.total.Requests++
	s.mu.Unlock()
}

// Record adds one compression result. shadow means the compressed body was
// measured but the original was sent.
func (s *Store) Record(model, format string, res protocol.Result, shadow bool) {
	if s == nil || len(res.Hits) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	add := func(c *Counter, before, after int64, blocks int64) {
		c.Blocks += blocks
		c.BytesBefore += before
		c.BytesAfter += after
	}
	if shadow {
		add(&s.shadow, int64(res.Before), int64(res.After), int64(len(res.Hits)))
		return
	}
	add(&s.total, int64(res.Before), int64(res.After), int64(len(res.Hits)))
	for _, h := range res.Hits {
		add(counterFor(s.byFilter, h.Filter), int64(h.Before), int64(h.After), 1)
	}
	add(counterFor(s.byModel, model), int64(res.Before), int64(res.After), int64(len(res.Hits)))
	add(counterFor(s.byFormat, format), int64(res.Before), int64(res.After), int64(len(res.Hits)))
}

func counterFor(m map[string]*Counter, key string) *Counter {
	if key == "" {
		key = "unknown"
	}
	c := m[key]
	if c == nil {
		c = &Counter{}
		m[key] = c
	}
	return c
}

type Snapshot struct {
	Total    Counter        `json:"total"`
	Shadow   Counter        `json:"shadow"`
	ByFilter []NamedCounter `json:"by_filter"`
	ByModel  []NamedCounter `json:"by_model"`
	ByFormat []NamedCounter `json:"by_format"`
}

func (s *Store) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{Total: s.total, Shadow: s.shadow}
	out.ByFilter = named(s.byFilter)
	out.ByModel = named(s.byModel)
	out.ByFormat = named(s.byFormat)
	return out
}

func named(m map[string]*Counter) []NamedCounter {
	out := make([]NamedCounter, 0, len(m))
	for k, v := range m {
		out = append(out, NamedCounter{Name: k, Counter: *v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BytesBefore == out[j].BytesBefore {
			return out[i].Name < out[j].Name
		}
		return out[i].BytesBefore > out[j].BytesBefore
	})
	return out
}
