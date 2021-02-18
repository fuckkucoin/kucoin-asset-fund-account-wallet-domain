package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Entry struct {
	Seq int64 `json:"seq"`
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Actor string `json:"actor"`
	Subject string `json:"subject,omitempty"`
	Body string    `json:"body"`
	At   time.Time `json:"at"`
}

type Store struct {
	mu   sync.Mutex
	rows []Entry
	byID map[string]int
	clock func() time.Time
	kv    persist.KV
}

  func New() *Store {
	   return &Store{byID: map[string]int{}, clock: func() time.Time { return time.Now().UTC() }, kv: persist.NewMemKV()}
   }

   func (s *Store) WithClock(c func() time.Time) *Store {
	   if s != nil && c != nil {
		   s.clock = c
	   }
	   return s
   }

   var ErrImmutable = fmt.Errorf("audit log is append-only")

  // Append 落一条。kind/actor/body 空则拒。返回带 seq 的副本；调用方改副本不影响库。
   func (s *Store) Append(ctx context.Context, e Entry) (Entry, error) {
	   e.Kind = strings.TrimSpace(e.Kind)
	  e.Actor = strings.TrimSpace(e.Actor)
	  if e.Kind == "" || e.Actor == "" || strings.TrimSpace(e.Body) == "" {
		   return Entry{}, persist.ErrConflict
	  }
	  if e.ID == "" {
		   e.ID = uuid.NewString()
	   }
	  if e.At.IsZero() {
		   e.At = s.clock()
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if _, ok := s.byID[e.ID]; ok {
		   return Entry{}, persist.ErrConflict
	  }
	  e.Seq = int64(len(s.rows) + 1)
	   s.rows = append(s.rows, e)
	   s.byID[e.ID] = len(s.rows) - 1
	  if s.kv != nil {
		   body, _ := json.Marshal(e)
		   _ = s.kv.Put(ctx, persist.Row{Kind: "audit", Key: fmt.Sprintf("%020d", e.Seq), Body: body}, persist.Fence{})
	   }
	   return e, nil
  }

   func (s *Store) Get(id string) (Entry, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  i, ok := s.byID[id]
	   if !ok {
		   return Entry{}, persist.ErrNotFound
	   }
	   return s.rows[i], nil
  }

   func (s *Store) List(kind, subject string, limit int) []Entry {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	  }
	  out := make([]Entry, 0, limit)
	   for i := len(s.rows) - 1; i >= 0 && len(out) < limit; i-- {
		   e := s.rows[i]
		  if kind != "" && e.Kind != kind {
			   continue
		  }
		  if subject != "" && e.Subject != subject {
			   continue
		   }
		  out = append(out, e)
	   }
	   return out
   }

// Rewrite 明确拒绝。和调用方都不得绕过。
  func (s *Store) Rewrite(id string, body string) error {
	   return ErrImmutable
  }

   func (s *Store) Delete(id string) error { return ErrImmutable }

   func (s *Store) Len() int {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  return len(s.rows)
  }
