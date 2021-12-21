package warroom

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	sw      Switch
	tickets map[string]*Ticket
	events  []Event
	frozen  map[string]bool
	led     *ledger.Service
	clock   func() time.Time
}

   func New() *Service {
	   return &Service{
		  tickets: map[string]*Ticket{}, frozen: map[string]bool{},
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (s *Service) BindLedger(led *ledger.Service) *Service {
	  if s != nil {
		  s.led = led
	   }
	   return s
   }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	   }
	  return s.clock()
  }

   func (s *Service) SetClock(fn func() time.Time) {
	   if s != nil && fn != nil {
		  s.clock = fn
	   }
   }

   func (s *Service) Current() Switch {
	  if s == nil {
		  return Switch{}
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   return s.sw.Copy()
  }

   func (s *Service) FrozenUsers() []string {
	  if s == nil {
		   return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]string, 0, len(s.frozen))
	   for u, on := range s.frozen {
		   if on {
			  out = append(out, u)
		   }
	   }
	   return out
  }

  func parseScope(s string) (Scope, error) {
	  switch Scope(strings.ToLower(strings.TrimSpace(s))) {
	  case ScopeUser, "":
		   return ScopeUser, nil
	  case ScopeGlobal:
		   return ScopeGlobal, nil
	  default:
		  return "", ErrScope
	  }
   }

   func (s *Service) noteLocked(kind, actor, user, detail string) {
	  s.events = append(s.events, Event{
		  ID: fmtID(len(s.events) + 1), Kind: kind, Actor: actor,
		  UserID: user,
		   Detail: detail, At: s.now(),
	   })
   }

   func fmtID(n int) string {
	   return time.Unix(0, int64(n)).UTC().Format("20060102150405") + "-" + itoa(n)
   }

   func itoa(n int) string {
	   if n == 0 {
		  return "0"
	  }
	   var b [16]byte
	   i := len(b)
	   for n > 0 {
		  i--
		   b[i] = byte('0' + n%10)
		   n /= 10
	  }
	  return string(b[i:])
  }

   func requireActor(s string) (string, error) {
	  a, err := validate.Actor(s)
	  if err != nil {
		  return "", ErrActor
	  }
	   return a, nil
  }

   func requireReason(s string) (string, error) {
	   r, err := validate.Reason(s)
	  if err != nil {
		   return "", ErrReason
	  }
	   return r, nil
   }

  func requireUser(s string) (string, error) {
	   u, err := validate.UserID(s)
	  if err != nil {
		   return "", ErrUser
	   }
	   if u == "system" {
		   return "", ErrSystem
	   }
	   return u, nil
  }
