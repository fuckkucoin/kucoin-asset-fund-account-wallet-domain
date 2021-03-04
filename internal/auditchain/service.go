package auditchain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

var (
	ErrUnavailable = fmt.Errorf("audit chain unavailable")
	ErrActor       = fmt.Errorf("audit chain actor invalid")
	ErrKind   = fmt.Errorf("audit chain kind invalid")
	ErrBody   = fmt.Errorf("audit chain body invalid")
	ErrTamper = fmt.Errorf("audit chain tamper")
	ErrImmutable = fmt.Errorf("audit chain append only")
	ErrHalt      = fmt.Errorf("audit chain halted")
)

type Entry struct {
	Seq int64 `json:"seq"`
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Actor string `json:"actor"`
	Body string `json:"body"`
	Prev string `json:"prev"`
	Hash string `json:"hash"`
	At time.Time `json:"at"`
}

type Service struct {
	mu    sync.Mutex
	halt  bool
	rows  []Entry
	clock func() time.Time
}

  func New() *Service {
	  return &Service{rows: []Entry{}, clock: func() time.Time { return time.Now().UTC() }}
   }

   func (s *Service) SetHalt(on bool) {
	  if s == nil {
		   return
	   }
	   s.mu.Lock()
	   s.halt = on
	   s.mu.Unlock()
  }

   func hashOf(prev, kind, actor, body, id string, seq int64, at time.Time) string {
	   h := sha256.New()
	   _, _ = h.Write([]byte(prev + "|" + kind + "|" + actor + "|" + body + "|" + id + "|" + fmt.Sprintf("%d|%d", seq, at.UnixNano())))
	   return hex.EncodeToString(h.Sum(nil))
   }

   func (s *Service) Append(kind, actor, body string) (Entry, error) {
	  if s == nil {
		  return Entry{}, ErrUnavailable
	  }
	   if s.halt {
		   return Entry{}, ErrHalt
	   }
	  kind = strings.TrimSpace(kind)
	   actor = strings.TrimSpace(actor)
	   body = strings.TrimSpace(body)
	  if kind == "" {
		  return Entry{}, ErrKind
	  }
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return Entry{}, ErrActor
	   }
	  if body == "" {
		  return Entry{}, ErrBody
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  prev := ""
	  if n := len(s.rows); n > 0 {
		   prev = s.rows[n-1].Hash
	   }
	  e := Entry{
		  Seq: int64(len(s.rows) + 1),
		   ID: uuid.NewString(), Kind: kind, Actor: actor, Body: body,
		   Prev: prev, At: s.clock(),
	  }
	   e.Hash = hashOf(e.Prev, e.Kind, e.Actor, e.Body, e.ID, e.Seq, e.At)
	   s.rows = append(s.rows, e)
	   return e, nil
   }

   func (s *Service) Verify() error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   prev := ""
	   for i, e := range s.rows {
		  if e.Prev != prev {
			   return ErrTamper
		  }
		  want := hashOf(e.Prev, e.Kind, e.Actor, e.Body, e.ID, e.Seq, e.At)
		   if e.Hash != want || e.Seq != int64(i+1) {
			   return ErrTamper
		   }
		   prev = e.Hash
	  }
	   return nil
  }

   func (s *Service) Rewrite(_ int, _ Entry) error { return ErrImmutable }

  func (s *Service) Delete(_ string) error { return ErrImmutable }

  func (s *Service) List() []Entry {
	  if s == nil {
		   return nil
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Entry, len(s.rows))
	   copy(out, s.rows)
	  return out
   }

   func (s *Service) TamperForTest(i int, body string) {
	   s.mu.Lock()
	  if i >= 0 && i < len(s.rows) {
		  s.rows[i].Body = body
	  }
	  s.mu.Unlock()
  }
