package keyrotatefence

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// MemStore 内存实现，供单测与非夹具。
type MemStore struct {
	mu sync.RWMutex
	byID map[string]*Window
}

   func NewMemStore() *MemStore {
	   return &MemStore{byID: map[string]*Window{}}
   }

  func (m *MemStore) Save(_ context.Context, w *Window) error {
	   if m == nil || w == nil {
		  return ErrUnavailable
	   }
	   id := strings.TrimSpace(w.TicketID)
	  if id == "" {
		   return ErrTicketRequired
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.byID[id] = w.clone()
	  return nil
   }

   func (m *MemStore) Get(_ context.Context, ticketID string) (*Window, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	  }
	  id := strings.TrimSpace(ticketID)
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  w, ok := m.byID[id]
	   if !ok {
		   return nil, ErrWindowNotFound
	   }
	   return w.clone(), nil
   }

   func (m *MemStore) Snapshot(_ context.Context) ([]byte, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   rows := make([]*Window, 0, len(m.byID))
	   for _, w := range m.byID {
		   rows = append(rows, w.clone())
	  }
	   return json.Marshal(rows)
   }

   func (m *MemStore) Restore(_ context.Context, raw []byte) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	   var rows []*Window
	   if len(raw) > 0 {
		   if err := json.Unmarshal(raw, &rows); err != nil {
			  return ErrSnapshotCorrupt
		  }
	  }
	   next := make(map[string]*Window, len(rows))
	  for _, w := range rows {
		   if w == nil || strings.TrimSpace(w.TicketID) == "" {
			  return ErrSnapshotCorrupt
		  }
		   next[strings.TrimSpace(w.TicketID)] = w.clone()
	   }
	  m.mu.Lock()
	  m.byID = next
	  m.mu.Unlock()
	   return nil
   }
