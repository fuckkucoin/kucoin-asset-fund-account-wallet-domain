package fiatsettlegate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/fiatrail"
)

// SettleAssert Capture+postLedger 必须消费未过期租约。
   func (s *Service) SettleAssert(ctx context.Context, req SettleAssertRequest) (*FiatSettleLease, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   actor, err := requireActor(req.Actor)
	   if err != nil {
		  return nil, err
	   }
	   if req.IdemKey != "" {
		  if id, ok, _ := s.store.GetIdem(ctx, "settle:"+req.IdemKey); ok {
			  return s.store.GetLease(ctx, id)
		  }
	  }
	  lease, err := s.store.GetLease(ctx, strings.TrimSpace(req.LeaseID))
	  if err != nil {
		   return nil, err
	   }
	   if req.BookingID != "" && req.BookingID != lease.BookingID {
		  return nil, ErrBooking
	   }
	  if err := s.expireIfNeeded(ctx, lease); err != nil {
		   return lease, err
	  }
	  if lease.State.Terminal() && lease.State != LeaseSettled {
		   return lease, ErrLeaseConsumed
	  }
	   if lease.State == LeaseSettled {
		   return lease, nil
	   }
	  if !lease.State.OpenForSettle() {
		  s.openProof(ctx, lease, ProofSettleRefundMux, true, string(lease.State), "settle", "lease not open for settle", lease.CaptureSealed, false)
		  return lease, ErrLeaseState
	   }
	   holder := "settle:" + actor
	   if err := s.store.TryLockBooking(ctx, lease.BookingID, holder, "settle", s.now(), s.lockT); err != nil {
		  s.openProof(ctx, lease, ProofSettleRefundMux, true, "", holder, "settle lost mutex vs refund", lease.CaptureSealed, false)
		   return lease, err
	  }
	   defer func() { _ = s.store.UnlockBooking(ctx, lease.BookingID, holder) }()
	  if h, purpose, ok := s.store.GetBookingLock(ctx, lease.BookingID); ok && purpose == "refund" && h != holder {
		  s.openProof(ctx, lease, ProofSettleRefundMux, true, h, holder, "refund in-flight", lease.CaptureSealed, false)
		   return lease, ErrMutexSettleRefund
	   }
	  b, err := s.rail.Get(lease.BookingID)
	  if err != nil {
		  return lease, ErrBooking
	  }
	   if err := s.revalidateFingerprints(ctx, lease, b); err != nil {
		   next := *lease
		  next.State = LeaseRevoked
		   next.Generation++
		   t := s.now()
		  next.RevokedAt = &t
		   next.Note = err.Error()
		   _ = s.store.CASLease(ctx, lease.LeaseID, lease.Generation, lease.State, next)
		   if res, e := s.store.GetReservation(ctx, lease.ReservationID); e == nil {
			  _ = s.releaseReservation(ctx, res, "fingerprint_drift")
		  }
		  return &next, err
	  }
	   from := lease.State
	   if from == LeaseAuthorized {
		  if err := CanTransit(from, LeaseSettleBound); err != nil {
			  return lease, err
		   }
		   next := *lease
		  next.State = LeaseSettleBound
		   next.Generation++
		  t := s.now()
		   next.BoundAt = &t
		   next.Actor = actor
		  if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, from, next); err != nil {
			  s.openProof(ctx, lease, ProofSettleRefundMux, true, "", actor, "cas lost on settle bind", lease.CaptureSealed, false)
			  return lease, fmt.Errorf("%w: %v", ErrMutexSettleRefund, err)
		   }
		  *lease = next
		  s.audit(ctx, lease.LeaseID, lease.BookingID, "settle_bind", actor, string(from), string(LeaseSettleBound), "")
	   }
	   if err := s.captureOnce(ctx, lease); err != nil {
		   return lease, err
	   }
	  tr, err := LedgerPost(ctx, s.led, b, "")
	  if err != nil {
		  return s.enterCompensate(ctx, lease, err)
	  }
	   next := *lease
	   if err := CanTransit(next.State, LeaseSettled); err != nil && next.State != LeaseCompensate {
		  if next.State != LeaseSettleBound && next.State != LeaseCompensate {
			  return lease, err
		  }
	  }
	   from = next.State
	  next.State = LeaseSettled
	   next.Generation++
	  next.LedgerID = tr.ID
	   t := s.now()
	  next.SettledAt = &t
	  next.Actor = actor
	   if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, from, next); err != nil {
		  return lease, err
	  }
	   if res, e := s.store.GetReservation(ctx, lease.ReservationID); e == nil {
		   _ = s.recordReservation(ctx, res)
	   }
	  s.rail.Limits().Record(b.UserID, b.Amount, b.Kind)
	   if _, err := s.rail.ApplySettled(b.ID, tr.ID, actor, lease.LeaseID); err != nil {
		   return &next, err
	   }
	   if req.IdemKey != "" {
		   _ = s.store.PutIdem(ctx, "settle:"+req.IdemKey, lease.LeaseID)
	   }
	   s.audit(ctx, lease.LeaseID, lease.BookingID, "settle_ok", actor, string(from), string(LeaseSettled), tr.ID)
	   return &next, nil
  }

  func (s *Service) captureOnce(ctx context.Context, lease *FiatSettleLease) error {
	   if lease.CaptureSealed {
		  in, err := s.rail.PSP().Get(lease.PSPIntent)
		   if err != nil {
			   return ErrPSP
		   }
		   if in.State == fiatrail.IntentCaptured || in.State == fiatrail.IntentRefunded {
			   s.openProof(ctx, lease, ProofCompensateRetry, true, lease.CaptureRef, "", "retry skipped second capture", true, false)
			   return nil
		   }
	   }
	  in, err := s.rail.PSP().Capture(lease.PSPIntent)
	   if err != nil {
		   if lease.CaptureSealed {
			   s.openProof(ctx, lease, ProofDoubleCapture, true, lease.CaptureRef, "", "second capture blocked", true, false)
			  return ErrCaptureOnce
		  }
		   return fmt.Errorf("%w: %v", ErrPSP, err)
	  }
	  next := *lease
	  next.CaptureSealed = true
	   next.CaptureRef = in.ID
	   next.Generation++
	   if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, lease.State, next); err != nil {
		  return err
	   }
	   *lease = next
	   s.audit(ctx, lease.LeaseID, lease.BookingID, "capture_sealed", lease.Actor, string(lease.State), string(lease.State), in.ID)
	   return nil
  }

   func (s *Service) enterCompensate(ctx context.Context, lease *FiatSettleLease, cause error) (*FiatSettleLease, error) {
	   from := lease.State
	  next := *lease
	  next.State = LeaseCompensate
	  next.Generation++
	  next.CompensateReason = cause.Error()
	   next.Note = "capture_ok_ledger_fail"
	   if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, from, next); err != nil {
		  return lease, err
	   }
	  s.openProof(ctx, &next, ProofCaptureOrphan, true, next.CaptureRef, "",
		  fmt.Sprintf("capture sealed; ledger failed: %v; booking must not stay settled", cause), true, false)
	   s.audit(ctx, next.LeaseID, next.BookingID, "compensate_open", next.Actor, string(from), string(LeaseCompensate), cause.Error())
	  b, _ := s.rail.Get(next.BookingID)
	  if b.State == fiatrail.StateSettled {
		   return &next, ErrSettledForbidden
	  }
	   return &next, fmt.Errorf("%w: %v", ErrCompensateOpen, cause)
   }

// RetryCompensate 补偿泵：禁止二次 Capture，仅重试 ledger。

  func (s *Service) RetryCompensate(ctx context.Context, leaseID, actor string) (*FiatSettleLease, error) {
	  lease, err := s.store.GetLease(ctx, leaseID)
	   if err != nil {
		   return nil, err
	   }
	   if lease.State != LeaseCompensate {
		  return lease, ErrLeaseState
	   }
	   if !lease.CaptureSealed {
		  return lease, ErrCaptureRequired
	  }
	   b, err := s.rail.Get(lease.BookingID)
	  if err != nil {
		  return lease, ErrBooking
	  }
	  if err := s.revalidateFingerprints(ctx, lease, b); err != nil {
		  return lease, err
	   }
	   if err := s.captureOnce(ctx, lease); err != nil && !errors.Is(err, ErrCaptureOnce) {
		   if errors.Is(err, ErrCaptureOnce) {
			  return lease, err
		  }
		  // captureOnce on sealed path returns nil
	  }
	   tr, err := LedgerPost(ctx, s.led, b, "")
	   if err != nil {
		   s.openProof(ctx, lease, ProofCaptureOrphan, true, lease.CaptureRef, "", "compensate retry ledger still fail", true, false)
		   return lease, fmt.Errorf("%w: %v", ErrCompensateOpen, err)
	  }
	   from := lease.State
	  next := *lease
	   next.State = LeaseSettled
	  next.Generation++
	  next.LedgerID = tr.ID
	  t := s.now()
	   next.SettledAt = &t
	  next.Actor = actor
	  if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, from, next); err != nil {
		   return lease, err
	   }
	  if res, e := s.store.GetReservation(ctx, lease.ReservationID); e == nil {
		  _ = s.recordReservation(ctx, res)
	   }
	   s.rail.Limits().Record(b.UserID, b.Amount, b.Kind)
	  _, _ = s.rail.ApplySettled(b.ID, tr.ID, actor, lease.LeaseID)
	  s.audit(ctx, lease.LeaseID, lease.BookingID, "compensate_settled", actor, string(from), string(LeaseSettled), tr.ID)
	  return &next, nil
   }

   func (s *Service) revokeCompensate(ctx context.Context, lease *FiatSettleLease, reason string) error {
	   from := lease.State
	   next := *lease
	  next.State = LeaseRevoked
	  next.Generation++
	   t := s.now()
	  next.RevokedAt = &t
	  next.Note = reason
	  if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, from, next); err != nil {
		   return err
	   }
	  if res, e := s.store.GetReservation(ctx, lease.ReservationID); e == nil {
		   _ = s.releaseReservation(ctx, res, reason)
	   }
	  return nil
  }
// temporary guard removed after the production check
