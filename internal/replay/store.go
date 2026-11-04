package replay

import (
	"encoding/json"
	"sync"
	"time"
)

// Entry is one stored turn of reasoning items.
type Entry struct {
	Ref        string
	Owner      string
	FromFormat string
	Model      string
	Items      []json.RawMessage
	CreatedAt  time.Time
	ExpiresAt  time.Time
	WeakScope  bool
	Bytes      int64
}

// Stats reports the current store size.
type Stats struct {
	Entries int   `json:"entries"`
	Bytes   int64 `json:"bytes"`
}

// Store is the persistence seam. Postgres will implement the same interface in
// a later ticket; the engine never knows which one it holds.
type Store interface {
	Put(Entry)
	Get(ref string) (Entry, bool)
	Stats() Stats
}

// Memory is the process-local store. Entries are lost on restart. It enforces
// the configured byte cap by evicting the oldest entries first.
type Memory struct {
	mu       sync.Mutex
	entries  map[string]Entry
	total    int64
	maxBytes int64
	now      func() time.Time
}

// NewMemory returns an empty in-memory store. maxBytes <= 0 means unlimited.
func NewMemory(maxBytes int64) *Memory {
	return &Memory{entries: make(map[string]Entry), maxBytes: maxBytes, now: time.Now}
}

// Put stores e, replacing any entry with the same reference, then evicts the
// oldest entries until the store is back under its byte cap.
func (m *Memory) Put(e Entry) {
	if m == nil {
		return
	}
	if e.Bytes == 0 {
		e.Bytes = entryBytes(e)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked()
	if m.maxBytes > 0 && e.Bytes > m.maxBytes {
		// One entry larger than the whole budget would evict everything and
		// still not fit; drop it instead of thrashing the store.
		return
	}
	if old, ok := m.entries[e.Ref]; ok {
		m.total -= old.Bytes
	}
	m.entries[e.Ref] = e
	m.total += e.Bytes
	for m.maxBytes > 0 && m.total > m.maxBytes {
		oldest := ""
		var oldestAt time.Time
		for ref, item := range m.entries {
			if ref == e.Ref {
				continue
			}
			if oldest == "" || item.CreatedAt.Before(oldestAt) {
				oldest = ref
				oldestAt = item.CreatedAt
			}
		}
		if oldest == "" {
			break
		}
		m.total -= m.entries[oldest].Bytes
		delete(m.entries, oldest)
	}
}

// Get returns a live entry. An expired entry is removed and reported missing.
func (m *Memory) Get(ref string) (Entry, bool) {
	if m == nil {
		return Entry{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[ref]
	if !ok {
		return Entry{}, false
	}
	if !e.ExpiresAt.IsZero() && m.now().After(e.ExpiresAt) {
		m.total -= e.Bytes
		delete(m.entries, ref)
		return Entry{}, false
	}
	return e, true
}

// Stats counts live entries and their bytes.
func (m *Memory) Stats() Stats {
	if m == nil {
		return Stats{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked()
	return Stats{Entries: len(m.entries), Bytes: m.total}
}

func (m *Memory) sweepLocked() {
	now := m.now()
	for ref, e := range m.entries {
		if !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt) {
			m.total -= e.Bytes
			delete(m.entries, ref)
		}
	}
}

func entryBytes(e Entry) int64 {
	n := int64(len(e.Ref) + len(e.Owner) + len(e.FromFormat) + len(e.Model))
	for _, item := range e.Items {
		n += int64(len(item))
	}
	return n
}
