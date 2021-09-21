package opsaudit

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// MemStore 内存实现。

type MemStore struct {
	mu sync.RWMutex
	rows []Record
	byID map[string]int
	byIdem map[string]int
}

   func NewMemStore() *MemStore {
	  return &MemStore{byID: map[string]int{}, byIdem: map[string]int{}}
  }

   func (m *MemStore) Append(_ context.Context, rec Record) (Record, error) {
	  if m == nil {
		   return Record{}, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if rec.ID == "" {
		  return Record{}, ErrUnavailable
	  }
	  if _, ok := m.byID[rec.ID]; ok {
		  return Record{}, ErrIdemConflict
	   }
	   if rec.IdemKey != "" {
		  if i, ok := m.byIdem[rec.IdemKey]; ok {
			   return m.rows[i].clone(), nil
		  }
	  }
	  m.rows = append(m.rows, rec)
	   idx := len(m.rows) - 1
	  m.byID[rec.ID] = idx
	  if rec.IdemKey != "" {
		   m.byIdem[rec.IdemKey] = idx
	  }
	   return rec.clone(), nil
   }

  func (m *MemStore) GetByIdem(_ context.Context, idem string) (Record, error) {
	  if m == nil {
		  return Record{}, ErrUnavailable
	   }
	  idem = strings.TrimSpace(idem)
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   i, ok := m.byIdem[idem]
	   if !ok {
		  return Record{}, ErrRecordNotFound
	   }
	  return m.rows[i].clone(), nil
  }

  func (m *MemStore) List(_ context.Context) ([]Record, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   out := make([]Record, len(m.rows))
	  copy(out, m.rows)
	   return out, nil
  }

   func (m *MemStore) ListByTicket(_ context.Context, ticket string) ([]Record, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  ticket = strings.TrimSpace(ticket)
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   var out []Record
	  for _, r := range m.rows {
		  if r.TicketID == ticket {
			   out = append(out, r.clone())
		   }
	  }
	   return out, nil
   }

  func (m *MemStore) Last(_ context.Context) (Record, error) {
	   if m == nil {
		  return Record{}, ErrUnavailable
	  }
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  if len(m.rows) == 0 {
		  return Record{}, ErrRecordNotFound
	   }
	   return m.rows[len(m.rows)-1].clone(),
		   nil
  }

  func (m *MemStore) Snapshot(_ context.Context) ([]byte, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   return json.Marshal(m.rows)
   }

   func (m *MemStore) Restore(_ context.Context, raw []byte) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	   var rows []Record
	  if len(raw) > 0 {
		   if err := json.Unmarshal(raw, &rows); err != nil {
			  return ErrSnapshotCorrupt
		   }
	   }
	   byID := make(map[string]int, len(rows))
	   byIdem := make(map[string]int)
	  for i, r := range rows {
		   if strings.TrimSpace(r.ID) == "" || r.Seq <= 0 {
			   return ErrSnapshotCorrupt
		   }
		   byID[r.ID] = i
		  if r.IdemKey != "" {
			  byIdem[r.IdemKey] = i
		   }
	   }
	  m.mu.Lock()
	   m.rows = rows
	  m.byID = byID
	   m.byIdem = byIdem
	   m.mu.Unlock()
	  return nil
   }
