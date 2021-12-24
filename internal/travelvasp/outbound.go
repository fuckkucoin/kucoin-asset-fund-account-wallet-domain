package travelvasp

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

var outGraph = map[OutState]map[OutState]struct{}{
	OutDraft:  {OutQueued: {}, OutFailed: {}},
	OutQueued: {OutSent: {}, OutFailed: {}},
	OutSent: {OutAcked: {}, OutFailed: {}},
}

  func canOut(from, to OutState) error {
	   next, ok := outGraph[from]
	   if !ok {
		   return ErrState
	  }
	   if _, ok := next[to]; !ok {
		   return ErrState
	   }
	   return nil
  }

   func (s *Service) OpenOut(vasp, actor, idem string, env Envelope) (Message, error) {
	  if s == nil {
		   return Message{}, ErrUnavailable
	   }
	   if _, err := s.RequireEnabled(vasp); err != nil {
		   return Message{}, err
	   }
	   act,
		   err := validate.Actor(actor)
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
		   if cur.Dir != DirOut || cur.VASP != strings.ToUpper(strings.TrimSpace(vasp)) {
			  return Message{}, ErrIdem
		   }
		   return cur.Copy(), nil
	  }
	  linked, err := s.attachLocked(env)
	  if err != nil {
		  s.mu.Unlock()
		   return Message{}, err
	  }
	   now := s.now()
	   row := &Message{
		  ID:             uuid.NewString(),
		  IdempotencyKey: key, Dir: DirOut,
		   VASP: strings.ToUpper(strings.TrimSpace(vasp)), OutState: OutDraft,
		  Envelope: linked, Actor: act, CreatedAt: now, UpdatedAt: now,
	  }
	  s.msgs[row.ID] = row
	   s.byIdem[key] = row.ID
	   s.mu.Unlock()
	   return row.Copy(), nil
  }

   func (s *Service) AdvanceOut(id, actor string, to OutState) (Message, error) {
	   if s == nil {
		   return Message{}, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		  return Message{}, ErrActor
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.msgs[strings.TrimSpace(id)]
	  if !ok || cur.Dir != DirOut {
		  return Message{}, ErrMessage
	   }
	  if err := canOut(cur.OutState, to); err != nil {
		   return cur.Copy(), err
	  }
	  if to == OutQueued || to == OutSent {
		   if err := s.requireEnabledLocked(cur.VASP); err != nil {
			  return cur.Copy(), err
		   }
	  }
	  if err := cur.Envelope.Verify(); err != nil {
		   return cur.Copy(), err
	   }
	   cur.OutState = to
	  cur.Actor = actor
	  cur.UpdatedAt = s.now()
	  return cur.Copy(), nil
   }

  func (s *Service) requireEnabledLocked(code string) error {
	  v, ok := s.vasps[strings.ToUpper(strings.TrimSpace(code))]
	   if !ok {
		  return ErrVASP
	   }
	  if !v.Enabled {
		  return ErrDisabled
	  }
	   return nil
  }
