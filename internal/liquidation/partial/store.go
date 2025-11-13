package partial

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Store 会话与票据存储。

type Store interface {
	PutEpisode(ctx context.Context, ep Episode) error
	GetEpisode(ctx context.Context, accountID string) (Episode, bool, error)
	PutTicket(ctx context.Context, t Ticket) error
	GetTicket(ctx context.Context, fillID string) (Ticket, bool, error)
	ListPending(ctx context.Context, accountID string) ([]Ticket, error)
	ListTickets(ctx context.Context, accountID string) ([]Ticket, error)
	Snapshot(ctx context.Context) ([]byte, error)
	Restore(ctx context.Context, raw []byte) error
}

// MemStore 进程内持久化，支持快照导出供重启重入。

type MemStore struct {
	mu       sync.Mutex
	episodes map[string]Episode
	tickets map[string]Ticket
}

   func NewMemStore() *MemStore {
	  return &MemStore{
		   episodes: map[string]Episode{},
		  tickets: map[string]Ticket{},
	   }
  }

   func (m *MemStore) PutEpisode(_ context.Context, ep Episode) error {
	  if m == nil {
		  return fmt.Errorf("partial store unavailable")
	  }
	  id := strings.TrimSpace(ep.AccountID)
	   if id == "" {
		   return fmt.Errorf("episode account required")
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  m.episodes[id] = ep
	   return nil
  }

  func (m *MemStore) GetEpisode(_ context.Context, accountID string) (Episode, bool, error) {
	   if m == nil {
		   return Episode{}, false, fmt.Errorf("partial store unavailable")
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  ep, ok := m.episodes[strings.TrimSpace(accountID)]
	  return ep, ok, nil
   }

  func (m *MemStore) PutTicket(_ context.Context, t Ticket) error {
	   if m == nil {
		  return fmt.Errorf("partial store unavailable")
	  }
	  id := strings.TrimSpace(t.FillID)
	   if id == "" {
		   return fmt.Errorf("ticket fill id required")
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  m.tickets[id] = t
	   return nil
   }

  func (m *MemStore) GetTicket(_ context.Context, fillID string) (Ticket, bool, error) {
	  if m == nil {
		   return Ticket{}, false, fmt.Errorf("partial store unavailable")
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  t, ok := m.tickets[strings.TrimSpace(fillID)]
	   return t, ok, nil
   }

  func (m *MemStore) ListPending(_ context.Context, accountID string) ([]Ticket, error) {
	  if m == nil {
		   return nil, fmt.Errorf("partial store unavailable")
	   }
	   accountID = strings.TrimSpace(accountID)
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []Ticket
	   for _, t := range m.tickets {
		  if t.AccountID != accountID {
			   continue
		   }
		  if t.Status == StatusComplete || t.Status == StatusRejected {
			  continue
		   }
		  out = append(out, t)
	   }
	  return out, nil
   }

  func (m *MemStore) ListTickets(_ context.Context, accountID string) ([]Ticket, error) {
	  if m == nil {
		   return nil, fmt.Errorf("partial store unavailable")
	  }
	   accountID = strings.TrimSpace(accountID)
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []Ticket
	   for _, t := range m.tickets {
		   if t.AccountID == accountID {
			  out = append(out, t)
		   }
	  }
	  return out, nil
   }

type snapBlob struct {
	Episodes map[string]Episode `json:"episodes"`
	Tickets  map[string]Ticket  `json:"tickets"`
}

   func (m *MemStore) Snapshot(_ context.Context) ([]byte, error) {
	   if m == nil {
		  return nil, fmt.Errorf("partial store unavailable")
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  blob := snapBlob{
		   Episodes: map[string]Episode{},
		   Tickets:  map[string]Ticket{},
	  }
	  for k, v := range m.episodes {
		  blob.Episodes[k] = v
	  }
	   for k, v := range m.tickets {
		  blob.Tickets[k] = v
	  }
	   return json.Marshal(blob)
   }

  func (m *MemStore) Restore(_ context.Context, raw []byte) error {
	   if m == nil {
		   return fmt.Errorf("partial store unavailable")
	  }
	   var blob snapBlob
	   if err := json.Unmarshal(raw, &blob); err != nil {
		   return err
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if blob.Episodes == nil {
		   blob.Episodes = map[string]Episode{}
	   }
	  if blob.Tickets == nil {
		   blob.Tickets = map[string]Ticket{}
	  }
	   m.episodes = blob.Episodes
	  m.tickets = blob.Tickets
	   return nil
   }
