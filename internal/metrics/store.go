// Package metrics aggregates compression and request counters.
package metrics

import (
	"sort"
	"sync"

	"github.com/Kyresn/relaywarden/internal/protocol"
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
	style    StyleCounter
	byStyle  map[string]*StyleCounter
	replay   ReplayCounter
}

// StyleCounter counts behaviour-prompt injections.
type StyleCounter struct {
	Injected int64 `json:"injected"`
	Stripped int64 `json:"stripped"`
	Failed   int64 `json:"failed"`
}

// NamedStyleCounter is one family:level bucket.
type NamedStyleCounter struct {
	Name string `json:"name"`
	StyleCounter
}

func New() *Store {
	return &Store{
		byFilter: make(map[string]*Counter),
		byModel:  make(map[string]*Counter),
		byFormat: make(map[string]*Counter),
		byStyle:  make(map[string]*StyleCounter),
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

// RecordStyle counts one behaviour-prompt pass. Buckets are keyed by
// family:level so the UI can show which level is doing work.
func (s *Store) RecordStyle(caveman, ponytail string, injected, stripped, failed bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bump := func(c *StyleCounter) {
		if injected {
			c.Injected++
		}
		if stripped {
			c.Stripped++
		}
		if failed {
			c.Failed++
		}
	}
	bump(&s.style)
	if caveman != "" && caveman != "off" {
		bump(styleFor(s.byStyle, "caveman:"+caveman))
	}
	if ponytail != "" && ponytail != "off" {
		bump(styleFor(s.byStyle, "ponytail:"+ponytail))
	}
}

// ReplayCounter counts reasoning-replay activity.
type ReplayCounter struct {
	Generated int64 `json:"generated"`
	Restored  int64 `json:"restored"`
	Invalid   int64 `json:"invalid"`
	Foreign   int64 `json:"foreign"`
}

// RecordReplay adds one replay pass to the counters.
func (s *Store) RecordReplay(generated, restored, invalid, foreign int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replay.Generated += int64(generated)
	s.replay.Restored += int64(restored)
	s.replay.Invalid += int64(invalid)
	s.replay.Foreign += int64(foreign)
}

func styleFor(m map[string]*StyleCounter, key string) *StyleCounter {
	c := m[key]
	if c == nil {
		c = &StyleCounter{}
		m[key] = c
	}
	return c
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
	Total    Counter             `json:"total"`
	Shadow   Counter             `json:"shadow"`
	ByFilter []NamedCounter      `json:"by_filter"`
	ByModel  []NamedCounter      `json:"by_model"`
	ByFormat []NamedCounter      `json:"by_format"`
	Style    StyleCounter        `json:"style"`
	ByStyle  []NamedStyleCounter `json:"by_style"`
	Replay   ReplayCounter       `json:"replay"`
}

func (s *Store) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{Total: s.total, Shadow: s.shadow, Style: s.style, Replay: s.replay}
	out.ByFilter = named(s.byFilter)
	out.ByModel = named(s.byModel)
	out.ByFormat = named(s.byFormat)
	out.ByStyle = namedStyle(s.byStyle)
	return out
}

func namedStyle(m map[string]*StyleCounter) []NamedStyleCounter {
	out := make([]NamedStyleCounter, 0, len(m))
	for k, v := range m {
		out = append(out, NamedStyleCounter{Name: k, StyleCounter: *v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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
