package incdef

import (
	"strings"
	"time"
)

// RecordCall 记一次依赖调用。失败推熔断；打开后 AllowCall 直接拒。
  func (s *Service) RecordCall(name string, ok bool) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	  if err := s.GuardWrite("ops-inc"); err != nil && err != ErrDegraded {
		  return err
	  }
	   b := s.Breaker(name)
	  if b == nil {
		  s.mu.Lock()
		  s.brks[name] = NewBreaker(name, 5, 3*time.Second)
		   s.mu.Unlock()
		   b = s.Breaker(name)
	  }
	  if ok {
		  b.Success()
		  return nil
	   }
	   b.Failure()
	  if snap := b.Snap(); snap.State == string(CircuitOpen) {
		  s.Raise(Alert{Level: "warn", Kind: "circuit", Subject: name, Body: "circuit open"})
	  }
	  return nil
  }

// CascadeKill 停写并打开全部熔断。unkill 不自动关熔断，须 Success。
   func (s *Service) CascadeKill(actor string, reason string) error {
	  if err := s.Kill(actor, true); err != nil {
		  return err
	  }
	   s.mu.Lock()
	   for _, b := range s.brks {
		  if b == nil {
			   continue
		   }
		  for i := 0; i < b.threshold; i++ {
			   b.Failure()
		  }
	   }
	  s.appendLocked("cascade_kill", actor, "", reason)
	  s.mu.Unlock()
	   s.Raise(Alert{Level: "crit", Kind: "kill", Subject: actor, Body: strings.TrimSpace(reason)})
	  return nil
   }

   func (s *Service) ResetCircuits(actor string) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  if err := validateActor(actor); err != nil {
		   return err
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if s.killed {
		  return ErrKilled
	   }
	  for _, b := range s.brks {
		  b.Success()
	   }
	   s.appendLocked("circuit_reset", actor, "", "off")
	   return nil
   }

   func validateActor(actor string) error {
	   if strings.TrimSpace(actor) == "" || strings.EqualFold(strings.TrimSpace(actor), "system") {
		   return ErrActor
	   }
	  return nil
   }

// TripOnPartition 分区时打开 rpc/outbox 并降级。
  func (s *Service) TripOnPartition(actor string) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	  if err := s.Degrade(actor, true); err != nil {
		  return err
	  }
	   _ = s.RecordCall("rpc", false)
	   _ = s.RecordCall("rpc", false)
	  _ = s.RecordCall("rpc", false)
	   _ = s.RecordCall("rpc", false)
	   _ = s.RecordCall("rpc", false)
	   _ = s.RecordCall("outbox", false)
	   _ = s.RecordCall("outbox", false)
	  _ = s.RecordCall("outbox", false)
	  _ = s.RecordCall("outbox", false)
	   _ = s.RecordCall("outbox", false)
	  return nil
  }
