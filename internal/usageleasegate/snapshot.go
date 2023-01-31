package usageleasegate

import (
	"context"
	"errors"
	"github.com/google/uuid"
)

// SnapshotWithLease 预检窗口并签发租约。默认预约占用（同键串行）。

   func (e *Engine) SnapshotWithLease(ctx context.Context, req SnapshotWithLeaseReq) (SnapshotWithLeaseResp, error) {
	  if err := e.require(); err != nil {
		   return SnapshotWithLeaseResp{}, err
	   }
	   if !req.Reserve && e.autoRes {
		  req.Reserve = true
	  }
	   if !req.Reserve {
		  req.Reserve = true
	  }
	  n, err := NormalizeSnapshot(req, e.ttl)
	  if err != nil {
		  return SnapshotWithLeaseResp{}, err
	   }
	  if n.Idem != "" {
		   if old, err := e.store.GetLeaseByIdem(ctx, n.Idem); err == nil && old.ID != "" && leaseAlive(old, e.now()) {
			  return snapResp(old, false), nil
		   }
	   }
	  view := e.liveView(n)
	  breached, err := BreachDailyView(e.src, n.Kind, n.UserID, n.Asset, n.DailyCapRaw)
	   if err != nil {
		  return SnapshotWithLeaseResp{}, err
	   }
	   now := e.now()
	  l := UsageLease{
		   ID: uuid.NewString(), Kind: n.Kind, UserID: n.UserID, Asset: n.Asset, Dest: n.Dest,
		   Amount: n.AmountRaw, TodaySum: view.Today, Velocity1h: view.Velocity1h,
		  Velocity24: view.Velocity24, DestN: view.DestN, DailyCap: n.DailyCapRaw,
		   VelocityCap: n.VelocityCap,
		  WindowPrint: ComputeViewPrint(view),
		  CapPrint:    ComputeCapPrint(n.DailyCapRaw, n.VelocityCap),
		  State:       LeaseSnapshotted,
		   TTL: Duration{Duration: n.TTL},
		  IssuedAt: now, ExpiresAt: now.Add(n.TTL),
		  Idem: n.Idem, CreatedAt: now, UpdatedAt: now,
	  }
	  if _, err := e.store.SaveLease(ctx, l); err != nil {
		   return SnapshotWithLeaseResp{}, err
	  }
	  e.audit(ctx, l.ID, LeaseEvSnapshot, "", LeaseSnapshotted, "precheck", "snapshot")
	   if n.VelocityCap > 0 && view.Velocity24 >= n.VelocityCap {
		  p := e.sealProof(ctx, l, view, DriftVelocity, n.AmountRaw, "0", true, "velocity cap already reached")
		   l.ProofID = p.ID
		   l, _ = e.expireOrRevoke(ctx, l, LeaseEvRevoke, "velocity")
		  return snapResp(l,
			   breached), wrapDrift(DriftVelocity, ErrWouldExceedCap)
	   }
	  if breached {
		   p := e.sealProof(ctx, l, view, DriftAlreadyBreached, n.AmountRaw, "0", true, "daily cap already breached")
		   l.ProofID = p.ID
		   l, _ = e.expireOrRevoke(ctx, l, LeaseEvRevoke, "breached")
		   return snapResp(l, true), wrapDrift(DriftAlreadyBreached, ErrAlreadyBreached)
	  }
	   unlock := e.keys.lock(WindowKey(n.Kind, n.UserID, n.Asset, n.Dest))
	   defer unlock()
	  l,
		  err = e.reserveLocked(ctx, l, n, view)
	   if err != nil && !errors.Is(err, ErrWouldExceedCap) {
		  return snapResp(l, false), err
	  }
	  if errors.Is(err, ErrWouldExceedCap) {
		   return snapResp(l, false), err
	  }
	   return snapResp(l, false), nil
   }

// BreachDailyWithLease 日限额预检并签发租约。已超限失败即关。
   func (e *Engine) BreachDailyWithLease(ctx context.Context, req SnapshotWithLeaseReq) (SnapshotWithLeaseResp, error) {
	  return e.SnapshotWithLease(ctx, req)
   }

   func snapResp(l UsageLease, breached bool) SnapshotWithLeaseResp {
	  return SnapshotWithLeaseResp{
		  LeaseID: l.ID, Kind: l.Kind, UserID: l.UserID, Asset: l.Asset, Dest: l.Dest,
		  TodaySum: l.TodaySum, Velocity1h: l.Velocity1h, Velocity24: l.Velocity24,
		  DestN: l.DestN, WindowPrint: l.WindowPrint, CapPrint: l.CapPrint,
		   State: l.State, ReservationID: l.ReservationID, ReservationSeq: l.ReservationSeq,
		   ExpiresAt: l.ExpiresAt, Breached: breached, Reserved: l.State == LeaseReserved && l.ReservationID != "",
	   }
  }

  func (e *Engine) expireOrRevoke(ctx context.Context, l UsageLease, ev, reason string) (UsageLease, error) {
	  var err error
	   l, err = e.transit(ctx, l, ev)
	  if err != nil {
		   return l, err
	   }
	   now := e.now()
	  if ev == LeaseEvExpire {
		  l.ExpiredAt = &now
	   } else {
		   l.RevokedAt = &now
		   l.RevokeReason = reason
	   }
	   l.UpdatedAt = now
	   return e.store.SaveLease(ctx, l)
   }
