package travelvasp

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

var inGraph = map[InState]map[InState]struct{}{
	InReceived: {InScreened: {}, InRefused: {}},
	InScreened: {InAccepted: {}, InRefused: {}},
}

  func canIn(from, to InState) error {
	  next, ok := inGraph[from]
	  if !ok {
		   return ErrState
	  }
	   if _, ok := next[to]; !ok {
		  return ErrState
	   }
	  return nil
   }

   func (s *Service) OpenIn(vasp, actor, idem string, env Envelope) (Message, error) {
	  if s == nil {
		   return Message{}, ErrUnavailable
	   }
	  if _, err := s.RequireEnabled(vasp); err != nil {
		  return Message{}, err
	   }
	   act, err := validate.Actor(actor)
	  if err != nil {
		   return Message{}, ErrActor
	   }
	  key, err := validate.Idem(idem)
	  if err != nil {
		  return Message{}, ErrIdem
	   }
	  if err := env.Verify(); err != nil {
		  return Message{}, err
	   }
	   s.mu.Lock()
	   if id, ok := s.byIdem[key]; ok {
		   cur := s.msgs[id]
		  s.mu.Unlock()
		  return cur.Copy(), nil
	  }
	   linked, err := s.attachLocked(env)
	  if err != nil {
		  s.mu.Unlock()
		  return Message{}, err
	   }
	   now := s.now()
	  row := &Message{
		  ID: uuid.NewString(), IdempotencyKey: key, Dir: DirIn,
		   VASP: strings.ToUpper(strings.TrimSpace(vasp)), InState: InReceived,
		  Envelope: linked, Actor: act, CreatedAt: now, UpdatedAt: now,
	   }
	  s.msgs[row.ID] = row
	   s.byIdem[key] = row.ID
	  s.mu.Unlock()
	  return row.Copy(), nil
  }

   func (s *Service) AdvanceIn(id, actor string, to InState) (Message, error) {
	  if s == nil {
		   return Message{}, ErrUnavailable
	  }
	   act, err := validate.Actor(actor)
	   if err != nil {
		  return Message{}, ErrActor
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.msgs[strings.TrimSpace(id)]
	   if !ok || cur.Dir != DirIn {
		  return Message{}, ErrMessage
	   }
	  if err := canIn(cur.InState, to); err != nil {
		   return cur.Copy(), err
	   }
	  if (to == InAccepted) && strings.EqualFold(act, cur.Actor) {
		   return cur.Copy(), ErrActor
	  }
	  if err := cur.Envelope.Verify(); err != nil {
		   return cur.Copy(), err
	  }
	  cur.InState = to
	  cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
  }

   func (s *Service) Get(id string) (Message, error) {
	  if s == nil {
		   return Message{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.msgs[strings.TrimSpace(id)]
	  if !ok {
		   return Message{}, ErrMessage
	   }
	   return cur.Copy(), nil
  }

  func (s *Service) List(dir string, limit int) []Message {
	  if s == nil {
		  return nil
	   }
	  if limit <= 0 {
		   limit = 50
	  }
	   dir = strings.TrimSpace(dir)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Message, 0, limit)
	  for _, m := range s.msgs {
		   if dir != "" && string(m.Dir) != dir {
			  continue
		  }
		  out = append(out, m.Copy())
		   if len(out) >= limit {
			   break
		  }
	  }
	  return out
  }
