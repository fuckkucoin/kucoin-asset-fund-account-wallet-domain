package fiatrail

import (
	"strings"
	"time"
)

// BindSettleLease 授权成功后回写租约号。不推进 FSM。

  func (s *Service) BindSettleLease(id, leaseID, reservationID string) (Booking, error) {
	  if s == nil {
		   return Booking{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   b, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		  return Booking{}, ErrBooking
	   }
	  b.SettleLeaseID = strings.TrimSpace(leaseID)
	  if reservationID != "" {
		  b.LimitsReservationID = strings.TrimSpace(reservationID)
	  }
	  b.UpdatedAt = s.now()
	  return b.Copy(), nil
  }

// BindLimitsReservation 预约或授权占用限额后回写占用号。不推进 FSM。
   func (s *Service) BindLimitsReservation(id, reservationID string) (Booking, error) {
	  if s == nil {
		  return Booking{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   b,
		  ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		   return Booking{}, ErrBooking
	  }
	  b.LimitsReservationID = strings.TrimSpace(reservationID)
	  b.UpdatedAt = s.now()
	   return b.Copy(), nil
  }

// ApplySettled 租约闸已完成 Capture+postLedger 后回写预约。走既有 settled 边。
   func (s *Service) ApplySettled(id, ledgerID, actor, leaseID string) (Booking, error) {
	   if s == nil {
		  return Booking{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  b, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		  return Booking{}, ErrBooking
	  }
	   if b.State == StateSettled && b.LedgerID == ledgerID {
		  return b.Copy(), nil
	   }
	  if err := CanAdvance(b.State, StateSettled); err != nil {
		  return b.Copy(), err
	  }
	   b.State = StateSettled
	   b.LedgerID = ledgerID
	  b.Actor = actor
	  if leaseID != "" {
		   b.SettleLeaseID = leaseID
	  }
	  b.UpdatedAt = s.now()
	   return b.Copy(), nil
   }

// ApplyRefunded 租约闸完成 Void/Refund+反记账后回写。

   func (s *Service) ApplyRefunded(id, refundLedger, actor, reason string) (Booking, error) {
	   if s == nil {
		   return Booking{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  b, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		   return Booking{}, ErrBooking
	  }
	  if b.State == StateRefunded {
		  return b.Copy(), nil
	  }
	  if err := CanAdvance(b.State, StateRefunded); err != nil {
		  return b.Copy(), err
	  }
	   b.State = StateRefunded
	   b.RefundLedger = refundLedger
	  b.Actor = actor
	  b.Reason = reason
	  b.UpdatedAt = s.now()
	   return b.Copy(), nil
  }

// Peek 只读预约，供租约闸复验。
  func (s *Service) Peek(id string) (Booking, error) {
	  return s.Get(id)
  }

// Now 只读时钟。

  func (s *Service) Now() time.Time {
	   return s.now()
   }
