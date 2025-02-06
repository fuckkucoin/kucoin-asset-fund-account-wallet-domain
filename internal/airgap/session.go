package airgap

import (
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/validate"
)

var sessionGraph = map[Phase]map[Phase]struct{}{
	PhaseOpen: {PhaseCollect: {},
		PhaseAborted: {}},
	PhaseCollect:  {PhaseSealed: {}, PhaseAborted: {}},
	PhaseSealed:   {PhaseExported: {}, PhaseAborted: {}},
	PhaseExported: {PhaseImported: {}, PhaseAborted: {}},
	PhaseImported: {PhaseClosed: {}, PhaseAborted: {}},
}

   func canPhase(from, to Phase) error {
	  next, ok := sessionGraph[from]
	   if !ok {
		   return ErrPhase
	   }
	   if _, ok := next[to]; !ok {
		  return ErrPhase
	  }
	   return nil
   }

   func (s *Service) Open(actor, label string, shares, threshold int, p Payload) (Session, error) {
	  if s == nil {
		  return Session{}, ErrUnavailable
	   }
	   actor, err := validate.Actor(actor)
	  if err != nil {
		  return Session{}, ErrActor
	  }
	   label = strings.TrimSpace(label)
	   if label == "" {
		   return Session{}, ErrLabel
	  }
	  if threshold < 2 || shares < threshold {
		  return Session{}, ErrThreshold
	  }
	  hash, err := HashPayload(p)
	  if err != nil {
		   return Session{}, err
	   }
	   now := s.now()
	  row := &Session{
		  ID: uuid.NewString(),
		  Label: label, Phase: PhaseOpen,
		  Shares: shares, Threshold: threshold, PayloadHash: hash, Payload: p,
		   Starter: actor, LastActor: actor, Actors: []string{actor},
		   Note:      "airgap ceremony recorder; not a custody certificate",
		   CreatedAt: now, UpdatedAt: now,
	  }
	  s.mu.Lock()
	  s.rows[row.ID] = row
	   s.mu.Unlock()
	  return row.Copy(), nil
   }

// Sign 收集签名。同一人不得连签凑满阈值。
   func (s *Service) Sign(id, actor, note string) (Session, error) {
	  if s == nil {
		   return Session{}, ErrUnavailable
	  }
	   actor,
		  err := validate.Actor(actor)
	   if err != nil {
		   return Session{}, ErrActor
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Session{}, ErrSession
	  }
	   if cur.Phase != PhaseOpen && cur.Phase != PhaseCollect {
		  return cur.Copy(), ErrPhase
	  }
	   if HasActor(cur.Signatures, actor) {
		   return cur.Copy(), ErrSameActor
	  }
	   if err := CheckHash(cur.Payload, cur.PayloadHash); err != nil {
		   return cur.Copy(), err
	   }
	   if cur.Phase == PhaseOpen {
		  if err := canPhase(cur.Phase, PhaseCollect); err != nil {
			   return cur.Copy(), err
		   }
		   cur.Phase = PhaseCollect
	  }
	   cur.Signatures = append(cur.Signatures, Signature{
		  Actor: actor, Hash: cur.PayloadHash, Note: strings.TrimSpace(note), At: s.now(),
	   })
	   if !containsActor(cur.Actors, actor) {
		   cur.Actors = append(cur.Actors, actor)
	  }
	   cur.LastActor = actor
	  cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
  }

// Seal 阈值签名齐了才能封。starter 不得当唯一封口人。
   func (s *Service) Seal(id, actor string) (Session, error) {
	  if s == nil {
		   return Session{}, ErrUnavailable
	  }
	   actor,
		   err := validate.Actor(actor)
	   if err != nil {
		  return Session{}, ErrActor
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Session{}, ErrSession
	  }
	  if err := canPhase(cur.Phase, PhaseSealed); err != nil {
		   return cur.Copy(), err
	   }
	  if !QuorumMet(cur.Signatures, cur.Threshold) {
		   return cur.Copy(), ErrQuorum
	  }
	  if strings.EqualFold(actor, cur.Starter) && DistinctActors(cur.Signatures) < cur.Threshold {
		   return cur.Copy(), ErrSameActor
	   }
	   if strings.EqualFold(actor, cur.Starter) && DistinctActors(cur.Signatures) == 1 {
		   return cur.Copy(), ErrSameActor
	  }
	   cur.Phase = PhaseSealed
	   cur.LastActor = actor
	   cur.UpdatedAt = s.now()
	  if !containsActor(cur.Actors, actor) {
		  cur.Actors = append(cur.Actors, actor)
	  }
	   cur.Note = "airgap session sealed; signing via PKCS#11 or cloud KMS"
	   metrics.AirgapSessions.WithLabelValues(string(PhaseSealed)).Inc()
	   return cur.Copy(), nil
  }

   func (s *Service) Abort(id, actor, reason string) (Session, error) {
	  if s == nil {
		  return Session{}, ErrUnavailable
	  }
	  actor, err := validate.Actor(actor)
	  if err != nil {
		   return Session{}, ErrActor
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Session{}, ErrSession
	   }
	   if cur.Phase.Terminal() {
		   return cur.Copy(), ErrPhase
	   }
	   if err := canPhase(cur.Phase, PhaseAborted); err != nil {
		  return cur.Copy(), err
	  }
	   cur.Phase = PhaseAborted
	  cur.LastActor = actor
	   cur.Note = strings.TrimSpace(reason)
	   cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
   }

   func (s *Service) Close(id, actor string) (Session, error) {
	   if s == nil {
		   return Session{},
			   ErrUnavailable
	   }
	  actor, err := validate.Actor(actor)
	   if err != nil {
		   return Session{}, ErrActor
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Session{}, ErrSession
	  }
	   if err := canPhase(cur.Phase, PhaseClosed); err != nil {
		   return cur.Copy(), err
	  }
	  cur.Phase = PhaseClosed
	   cur.LastActor = actor
	  cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
   }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
   }
