package airgap

import (
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Export 封存后导出气隙信封。只带哈希和签名，不写分片明文。
   func (s *Service) Export(id, actor string) (Envelope, error) {
	   if s == nil {
		  return Envelope{}, ErrUnavailable
	  }
	  actor, err := validate.Actor(actor)
	  if err != nil {
		  return Envelope{}, ErrActor
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		  return Envelope{}, ErrSession
	  }
	   if err := canPhase(cur.Phase, PhaseExported); err != nil {
		  return Envelope{}, err
	   }
	  body, err := json.Marshal(cur.Payload)
	   if err != nil {
		  return Envelope{}, ErrEnvelope
	   }
	   env := Envelope{
		  SessionID:   cur.ID,
		  PayloadHash: cur.PayloadHash, Body: string(body),
		   Signatures: append([]Signature(nil), cur.Signatures...),
		   ExportedAt: s.now(),
		   Note: "airgap export envelope; not a custody certificate",
	   }
	   cur.Phase = PhaseExported
	   cur.LastActor = actor
	  cur.UpdatedAt = s.now()
	  s.envelopes[cur.ID] = env
	  return env.Copy(), nil
   }

// Import 核哈希和签名人数。人数不够或哈希不对失败即关。
   func (s *Service) Import(env Envelope, actor string) (Session, error) {
	   if s == nil {
		  return Session{}, ErrUnavailable
	  }
	   actor, err := validate.Actor(actor)
	   if err != nil {
		   return Session{}, ErrActor
	   }
	  if strings.TrimSpace(env.SessionID) == "" || strings.TrimSpace(env.PayloadHash) == "" {
		   return Session{}, ErrEnvelope
	  }
	   var p Payload
	  if err := json.Unmarshal([]byte(env.Body), &p); err != nil {
		  return Session{}, ErrEnvelope
	  }
	  if err := CheckHash(p, env.PayloadHash); err != nil {
		  return Session{}, err
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cur, ok := s.rows[env.SessionID]
	   if !ok {
		   return Session{}, ErrSession
	   }
	   if err := canPhase(cur.Phase, PhaseImported); err != nil {
		  return cur.Copy(), err
	  }
	   if env.PayloadHash != cur.PayloadHash {
		   return cur.Copy(), ErrHash
	  }
	  if !QuorumMet(env.Signatures, cur.Threshold) {
		  return cur.Copy(), ErrQuorum
	  }
	   cur.Phase = PhaseImported
	  cur.LastActor = actor
	  cur.UpdatedAt = s.now()
	   s.envelopes[cur.ID] = env.Copy()
	  return cur.Copy(), nil
  }

   func (s *Service) Envelope(id string) (Envelope, error) {
	  if s == nil {
		   return Envelope{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  env, ok := s.envelopes[strings.TrimSpace(id)]
	   if !ok {
		   return Envelope{}, ErrEnvelope
	   }
	  return env.Copy(), nil
  }
