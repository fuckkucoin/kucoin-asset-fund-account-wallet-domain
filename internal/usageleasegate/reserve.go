package usageleasegate

import (
	"context"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func (e *Engine) reserveLocked(ctx context.Context, l UsageLease, n NormalizedSnap, view WindowView) (UsageLease, error) {
	   wk := WindowKey(n.Kind,
		  n.UserID, n.Asset, n.Dest)
	   today := e.todayAmt(n.Kind, n.UserID, n.Asset)
	  // 预检只已提交占用；并发预约串行拿序号，超限在 Record 提交时封堵。
	   if WouldExceed(today, money.Zero(), n.Amount, n.DailyCap) {
		   held, _ := e.store.ListHeldByWindow(ctx, wk)
		  reserved := SumHeld(held)
		  p := e.sealProof(ctx, l, view, DriftWouldExceedCap, n.AmountRaw, reserved.String(), true, "snapshot amount already exceeds remaining cap")
		   l.ProofID = p.ID
		  l, _ = e.expireOrRevoke(ctx, l, LeaseEvRevoke, "would_exceed_cap")
		   return l, wrapDrift(DriftWouldExceedCap, ErrWouldExceedCap)
	   }
	  seq, err := e.store.NextSeq(ctx, wk)
	   if err != nil {
		  return l, err
	  }
	   now := e.now()
	   r := UsageReservation{
		  ID: uuid.NewString(), LeaseID: l.ID, WindowKey: wk,
		   Kind:   n.Kind,
		   UserID: n.UserID, Asset: n.Asset, Dest: n.Dest,
		   Amount: n.AmountRaw, State: ResHeld, Seq: seq,
		   HeldAt:    now,
		  ExpiresAt: l.ExpiresAt, CreatedAt: now, UpdatedAt: now,
	   }
	  if _, err := e.store.SaveReservation(ctx, r); err != nil {
		   return l, err
	  }
	   l, err = e.transit(ctx, l, LeaseEvReserve)
	  if err != nil {
		   return l, err
	  }
	  l.ReservationID = r.ID
	  l.ReservationSeq = seq
	   l.ReservedAt = &now
	  l.UpdatedAt = now
	   return e.store.SaveLease(ctx, l)
   }

   func (e *Engine) releaseReservation(ctx context.Context, r UsageReservation, reason string) (UsageReservation, error) {
	   if !CanReleaseReservation(r.State) {
		   return r, nil
	   }
	  now := e.now()
	   r.State = ResReleased
	   r.ReleasedAt = &now
	  r.Reason = reason
	  r.UpdatedAt = now
	  return e.store.SaveReservation(ctx,
		  r)
  }

  func (e *Engine) consumeReservation(ctx context.Context, r UsageReservation) (UsageReservation, error) {
	  if !CanConsumeReservation(r.State) {
		  return r, ErrReservationMissing
	  }
	  now := e.now()
	   r.State = ResConsumed
	  r.ConsumedAt = &now
	   r.UpdatedAt = now
	   return e.store.SaveReservation(ctx, r)
  }

   func (e *Engine) heldSum(ctx context.Context, wk string) (money.Amount, error) {
	   held, err := e.store.ListHeldByWindow(ctx, wk)
	  if err != nil {
		  return money.Zero(), err
	  }
	   return SumHeld(held), nil
  }

// OpsRelease 运维释放未提交预约。已提交占用禁止释放，避免抹占用。

   func (e *Engine) OpsRelease(ctx context.Context, req OpsReleaseReq) (OpsReleaseResp, error) {
	   if err := e.require(); err != nil {
		  return OpsReleaseResp{}, err
	  }
	   l, err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		  return OpsReleaseResp{}, err
	  }
	   if l.State == LeaseCommitted {
		  return OpsReleaseResp{LeaseID: l.ID, State: l.State}, ErrOpsReleaseDenied
	   }
	  if l.ReservationID != "" {
		  if r, err := e.store.GetReservation(ctx, l.ReservationID); err == nil {
			  _,
				  _ = e.releaseReservation(ctx, r, req.Reason)
		  }
	   }
	  l, err = e.expireOrRevoke(ctx, l, LeaseEvRevoke, req.Reason)
	   if err != nil && l.State != LeaseExpired && l.State != LeaseRevoked {
		  return OpsReleaseResp{}, err
	   }
	   e.audit(ctx, l.ID, "ops_release", l.State, l.State, req.Actor, req.Reason)
	   return OpsReleaseResp{LeaseID: l.ID, State: l.State, Released: true, Reason: req.Reason}, nil
  }
