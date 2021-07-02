package fiatsettlegate

import (
	"context"
	"fmt"
	"strings"
	"github.com/google/uuid"
)

// RefundWithLease Refund 与在途 Settle 互斥。
  func (s *Service) RefundWithLease(ctx context.Context, req RefundWithLeaseRequest) (*RefundLease, *FiatSettleLease, error) {
	   if s == nil {
		   return nil, nil, ErrUnavailable
	  }
	   actor, err := requireActor(req.Actor)
	  if err != nil {
		   return nil, nil, err
	  }
	  reason := strings.TrimSpace(req.Reason)
	  if reason == "" {
		   return nil, nil, ErrReason
	  }
	   if req.IdemKey != "" {
		   if id, ok, _ := s.store.GetIdem(ctx, "refund:"+req.IdemKey); ok {
			   rf, err := s.store.GetRefundLease(ctx, id)
			   if err != nil {
				  return nil, nil, err
			  }
			   l, _ := s.store.GetLeaseByBooking(ctx, rf.BookingID)
			   return rf, l, nil
		   }
	  }
	  var lease *FiatSettleLease
	   if strings.TrimSpace(req.LeaseID) != "" {
		   lease,
			  err = s.store.GetLease(ctx, req.LeaseID)
	  } else {
		  lease, err = s.store.GetLeaseByBooking(ctx, req.BookingID)
	  }
	   if err != nil {
		   return nil, nil, err
	  }
	   if err := s.expireIfNeeded(ctx, lease); err != nil && lease.State != LeaseExpired {
		  return nil, lease, err
	   }
	   holder := "refund:" + actor
	  if err := s.store.TryLockBooking(ctx, lease.BookingID, holder, "refund", s.now(), s.lockT); err != nil {
		   s.openProof(ctx, lease, ProofSettleRefundMux, true, "", holder, "refund lost mutex vs settle", lease.CaptureSealed, lease.State == LeaseSettled)
		   rf := s.blockRefund(ctx, lease, actor, reason, "mutex")
		  return rf, lease, ErrMutexSettleRefund
	  }
	  defer func() { _ = s.store.UnlockBooking(ctx, lease.BookingID, holder) }()
	  if lease.State == LeaseSettleBound || lease.State == LeaseCompensate {
		   s.openProof(ctx, lease, ProofSettleRefundMux, true, string(lease.State), holder, "settle in-flight blocks refund", lease.CaptureSealed, false)
		  rf := s.blockRefund(ctx, lease, actor, reason, "settle_inflight")
		  return rf, lease, ErrMutexSettleRefund
	   }
	   if !lease.State.OpenForRefund() && lease.State != LeaseAuthorized && lease.State != LeaseSettled {
		  rf := s.blockRefund(ctx, lease, actor, reason, "lease_closed")
		  return rf, lease, ErrLeaseState
	   }
	  b, err := s.rail.Get(lease.BookingID)
	  if err != nil {
		   return nil, lease, ErrBooking
	  }
	  now := s.now()
	  rf := RefundLease{
		  RefundLeaseID: uuid.NewString(),
		  SettleLeaseID: lease.LeaseID,
		   BookingID:     lease.BookingID,
		   PSPIntent: lease.PSPIntent,
		  State:     RefundIssued,
		  Actor:     actor,
		  Reason:   reason,
		   IssuedAt: now,
	  }
	  if err := s.store.InsertRefundLease(ctx, rf); err != nil {
		   exist, e := s.store.GetRefundLeaseByBooking(ctx, lease.BookingID)
		  if e == nil && exist.State == RefundCompleted {
			  return exist, lease, nil
		   }
		   return nil, lease, err
	   }
	   from := lease.State
	   if err := CanTransit(from, LeaseRefunding); err != nil {
		  _ = s.failRefund(ctx, &rf, err.Error())
		   return &rf, lease, err
	  }
	  next := *lease
	   next.State = LeaseRefunding
	  next.Generation++
	  next.Actor = actor
	   if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, from, next); err != nil {
		  s.openProof(ctx, lease, ProofSettleRefundMux, true, "", holder, "cas lost on refund bind", lease.CaptureSealed, from == LeaseSettled)
		   _ = s.failRefund(ctx, &rf, err.Error())
		  return &rf, lease, fmt.Errorf("%w: %v", ErrMutexSettleRefund, err)
	   }
	   *lease = next
	   rf.State = RefundBound
	   _ = s.store.CASRefundLease(ctx, rf.RefundLeaseID, RefundIssued, rf)
	  switch from {
	  case LeaseAuthorized:
		   in, err := s.rail.PSP().Void(lease.PSPIntent)
		   if err != nil {
			  _ = s.failRefund(ctx, &rf, err.Error())
			  return &rf, lease, fmt.Errorf("%w: %v", ErrPSP, err)
		  }
		  rf.PSPRefundRef = in.ID
	  case LeaseSettled:
		   in, err := s.rail.PSP().Refund(lease.PSPIntent)
		  if err != nil {
			   _ = s.failRefund(ctx, &rf, err.Error())
			   return &rf, lease, fmt.Errorf("%w: %v", ErrPSP, err)
		  }
		   rf.PSPRefundRef = in.ID
		   rev := b
		  rev.Kind = ReverseKind(b.Kind)
		   rev.ID = b.ID + ":refund"
		  tr, err := LedgerPost(ctx, s.led, rev, "refund")
		  if err != nil {
			  _ = s.failRefund(ctx, &rf, err.Error())
			  return &rf, lease, fmt.Errorf("%w: %v", ErrLedger, err)
		   }
		  rf.ReverseLedger = tr.ID
	  default:
		   _ = s.failRefund(ctx, &rf, "unsupported refund source")
		  return &rf, lease, ErrRefundLeaseState
	  }
	   rf.State = RefundCompleted
	  t := s.now()
	  rf.CompletedAt = &t
	   _ = s.store.CASRefundLease(ctx,
		  rf.RefundLeaseID, RefundBound, rf)
	   fin := *lease
	   fin.State = LeaseRefunded
	   fin.Generation++
	   _ = s.store.CASLease(ctx, lease.LeaseID, lease.Generation, LeaseRefunding, fin)
	  if res, e := s.store.GetReservation(ctx, lease.ReservationID); e == nil && from == LeaseAuthorized {
		   _ = s.releaseReservation(ctx, res, "refund_void")
	  }
	   _, _ = s.rail.ApplyRefunded(b.ID, rf.ReverseLedger, actor, reason)
	  if req.IdemKey != "" {
		   _ = s.store.PutIdem(ctx, "refund:"+req.IdemKey, rf.RefundLeaseID)
	  }
	   s.audit(ctx, lease.LeaseID, lease.BookingID, "refund_ok", actor, string(from), string(LeaseRefunded), rf.RefundLeaseID)
	  return &rf, &fin, nil
  }

  func (s *Service) blockRefund(ctx context.Context, lease *FiatSettleLease, actor, reason, detail string) *RefundLease {
	  now := s.now()
	   rf := RefundLease{
		  RefundLeaseID: uuid.NewString(),
		  SettleLeaseID: lease.LeaseID,
		   BookingID: lease.BookingID,
		  PSPIntent: lease.PSPIntent,
		   State:     RefundBlocked,
		   Actor:     actor,
		   Reason:    reason,
		   IssuedAt: now,
		   FailReason: detail,
	   }
	  _ = s.store.InsertRefundLease(ctx, rf)
	  return &rf
   }

   func (s *Service) failRefund(ctx context.Context, rf *RefundLease, detail string) error {
	   from := rf.State
	  next := *rf
	   next.State = RefundFailed
	   next.FailReason = detail
	  if from == RefundIssued || from == RefundBound {
		  _ = s.store.CASRefundLease(ctx, rf.RefundLeaseID, from, next)
	   }
	   *rf = next
	  return nil
   }
