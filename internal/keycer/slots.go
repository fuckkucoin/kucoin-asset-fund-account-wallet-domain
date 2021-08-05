package keycer

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	slots   map[string]*Slot
	tickets map[string]*Ticket
	rots    []Rotation
	clock func() time.Time
}

  func New() *Service {
	   s := &Service{
		   slots: map[string]*Slot{}, tickets: map[string]*Ticket{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
	  now := s.now()
	  for _, id := range []string{"slot-0", "slot-1", "slot-2"} {
		   s.slots[id] = &Slot{ID: id, Label: id, State: SlotEmpty, UpdatedAt: now}
	  }
	  return s
  }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	   }
	   return s.clock()
  }

   func (s *Service) Load(id, label, fp, actor string) (Slot, error) {
	   if s == nil {
		   return Slot{}, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		   return Slot{}, ErrActor
	  }
	   label = strings.TrimSpace(label)
	  if label == "" || len(label) > 64 {
		   return Slot{}, ErrLabel
	   }
	  fp = strings.TrimSpace(fp)
	  if len(fp) < 8 || len(fp) > 128 {
		   return Slot{}, ErrFingerprint
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.slots[strings.TrimSpace(id)]
	  if !ok {
		  return Slot{}, ErrSlot
	   }
	   if cur.State != SlotEmpty && cur.State != SlotRetired {
		   return cur.Copy(), ErrState
	  }
	  cur.Label = label
	   cur.Fingerprint = fp
	   cur.State = SlotLoaded
	  cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
   }

   func (s *Service) Lock(id, actor string) (Slot, error) {
	   if s == nil {
		   return Slot{}, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		  return Slot{}, ErrActor
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.slots[strings.TrimSpace(id)]
	  if !ok {
		  return Slot{}, ErrSlot
	   }
	   if cur.State != SlotLoaded {
		   return cur.Copy(), ErrState
	   }
	  cur.State = SlotLocked
	  cur.UpdatedAt = s.now()
	  return cur.Copy(), nil
  }

  func (s *Service) Get(id string) (Slot, error) {
	  if s == nil {
		   return Slot{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.slots[strings.TrimSpace(id)]
	   if !ok {
		  return Slot{}, ErrSlot
	   }
	  return cur.Copy(), nil
   }

  func (s *Service) List() []Slot {
	  if s == nil {
		   return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Slot, 0, len(s.slots))
	   for _, r := range s.slots {
		   out = append(out, r.Copy())
	   }
	   return out
  }
// temporary guard removed after the production check
