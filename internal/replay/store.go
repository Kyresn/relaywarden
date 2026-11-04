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

// Memory is the process-local store. Entries are lost on restart.
type Memory struct {
	mu      sync.Mutex
	entries map[string]Entry
	now     func() time.Time
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{entries: make(map[string]Entry), now: time.Now}
}

// Put stores e, replacing any entry with the same reference.
func (m *Memory) Put(e Entry) {
	if m == nil {
		return
	}
	if e.Bytes == 0 {
		e.Bytes = entryBytes(e)
	}
	m.mu.Lock()
	m.entries[e.Ref] = e
	m.mu.Unlock()
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
	out := Stats{}
	now := m.now()
	for ref, e := range m.entries {
		if !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt) {
			delete(m.entries, ref)
			continue
		}
		out.Entries++
		out.Bytes += e.Bytes
	}
	return out
}

func entryBytes(e Entry) int64 {
	n := int64(len(e.Ref) + len(e.Owner) + len(e.FromFormat))
	for _, item := range e.Items {
		n += int64(len(item))
	}
	return n
}
