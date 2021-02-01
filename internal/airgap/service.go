package airgap

import (
	"strings"
	"sync"
	"time"
)

type Service struct {
	mu sync.Mutex
	rows      map[string]*Session
	envelopes map[string]Envelope
	sealed    map[string][]byte
	clock     func() time.Time
}

  func New() *Service {
	   return &Service{
		  rows: map[string]*Session{},
		   envelopes: map[string]Envelope{},
		   sealed: map[string][]byte{},
		   clock:  func() time.Time { return time.Now().UTC() },
	  }
   }

  func (s *Service) Get(id string) (Session, error) {
	  if s == nil {
		  return Session{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur,
		  ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		  return Session{}, ErrSession
	  }
	  return cur.Copy(), nil
  }

  func (s *Service) List(limit int) []Session {
	  if s == nil {
		  return nil
	  }
	  if limit <= 0 {
		   limit = 50
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Session, 0, limit)
	  for _, r := range s.rows {
		  out = append(out, r.Copy())
		  if len(out) >= limit {
			   break
		   }
	  }
	   return out
  }

  func (s *Service) dumpLocked() ([]Session, []Envelope, map[string][]byte) {
	   rows := make([]Session, 0, len(s.rows))
	  for _, r := range s.rows {
		  rows = append(rows, r.Copy())
	  }
	   envs := make([]Envelope,
		   0, len(s.envelopes))
	  for _, e := range s.envelopes {
		  envs = append(envs, e.Copy())
	  }
	  sealed := make(map[string][]byte, len(s.sealed))
	   for k, v := range s.sealed {
		  sealed[k] = append([]byte(nil), v...)
	   }
	   return rows, envs, sealed
   }
