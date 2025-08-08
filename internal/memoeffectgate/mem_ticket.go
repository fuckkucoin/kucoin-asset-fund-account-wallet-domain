package memoeffectgate

import (
	"context"
	"strings"
	"sync"
	"github.com/google/uuid"
)

type MemTicketBook struct {
	mu sync.Mutex
	rows map[string]MemoTicketView
}

  func NewMemTicketBook() *MemTicketBook {
	  return &MemTicketBook{rows: map[string]MemoTicketView{}}
   }

  func (m *MemTicketBook) Put(t MemoTicketView) MemoTicketView {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if t.ID == "" {
		  t.ID = uuid.NewString()
	  }
	   if t.Status == "" {
		  t.Status = "pending"
	  }
	  m.rows[t.ID] = t
	   return t
   }

  func (m *MemTicketBook) GetMemoTicket(_ context.Context, id string) (MemoTicketView, error) {
	  if m == nil {
		   return MemoTicketView{}, ErrBookUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   t, ok := m.rows[strings.TrimSpace(id)]
	  if !ok {
		  return MemoTicketView{}, ErrTicketNotFound
	  }
	   return t, nil
   }

   func (m *MemTicketBook) AdvanceTicket(_ context.Context, id, status string) (MemoTicketView, error) {
	  if m == nil {
		  return MemoTicketView{}, ErrBookUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  t, ok := m.rows[strings.TrimSpace(id)]
	  if !ok {
		  return MemoTicketView{}, ErrTicketNotFound
	  }
	   t.Status = normalizeStatus(status)
	   m.rows[t.ID] = t
	  return t, nil
  }
