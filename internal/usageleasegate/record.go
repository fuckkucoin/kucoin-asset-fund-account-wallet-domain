package usageleasegate

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// RecordAssert 提交占用。无租约/过期/窗口漂移/将超限一律失败且不计占用。
  func (e *Engine) RecordAssert(ctx context.Context, req RecordAssertReq) (RecordAssertResp, error) {
	  if err := e.require(); err != nil {
		   return RecordAssertResp{}, err
	   }
	   if strings.TrimSpace(req.LeaseID) == "" {
		   p := e.sealProof(ctx,
			   UsageLease{UserID: req.UserID, Kind: req.Kind, Asset: req.Asset, Dest: req.Dest},
			   WindowView{Today: "0"}, DriftMissingLease, req.Amount, "0", false, "record without usage_lease_id")
		   return RecordAssertResp{Committed: false, DriftProof: &p}, wrapDrift(DriftMissingLease, ErrMissingLease)
	  }
	   kind, uid, asset, dest, amount, err := NormalizeRecord(req)
	   if err != nil {
		  if errors.Is(err, ErrMissingLease) {
			   p := e.sealProof(ctx, UsageLease{UserID: req.UserID}, WindowView{}, DriftMissingLease, req.Amount, "0", false, "missing lease")
			   return RecordAssertResp{Committed: false,
				   DriftProof: &p}, wrapDrift(DriftMissingLease, err)
		  }
		   return RecordAssertResp{}, err
	  }
	   l,
		   err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   p := e.sealProof(ctx,
			   UsageLease{ID: req.LeaseID, UserID: uid, Kind: kind, Asset: asset, Dest: dest},
			   WindowView{}, DriftMissingLease, amount, "0", false, "lease not found")
		   return RecordAssertResp{LeaseID: req.LeaseID, DriftProof: &p}, wrapDrift(DriftMissingLease, ErrLeaseNotFound)
	  }
	  if err := BindMatch(l, kind, uid, asset, dest); err != nil {
		  dk := DriftBinding
		   if errors.Is(err, ErrOwnerMismatch) {
			  dk = DriftOwner
		   }
		  live := e.liveFromLease(l)
		   p := e.sealProof(ctx, l, live, dk, amount, "0", false, err.Error())
		  return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(dk, err)
	  }
	   if strings.TrimSpace(req.Amount) != "" && amount != l.Amount {
		   // 写入金额必须与预约金额一致，防止放大穿透。
		   live := e.liveFromLease(l)
		   p := e.sealProof(ctx, l, live, DriftBinding, amount, "0", false, "amount != reserved")
		   return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(DriftBinding, ErrBindingMismatch)
	  }
	   if l.State == LeaseCommitted && l.Idem != "" && l.Idem == strings.TrimSpace(req.Idem) {
		   return RecordAssertResp{LeaseID: l.ID, State: l.State, Committed: true, HitID: l.HitID, Reused: true}, nil
	   }
	  now := e.now()
	   if dk, ferr := e.failClosedLease(l, now); ferr != nil && !errors.Is(ferr, ErrLeaseNotReserved) {
		   live := e.liveFromLease(l)
		   p := e.sealProof(ctx, l, live, dk, amount, "0", false, ferr.Error())
		   return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(dk, ferr)
	   }
	  wk := WindowKey(l.Kind, l.UserID, l.Asset, l.Dest)
	   unlock := e.keys.lock(wk)
	  defer unlock()
	  // 过期预约自动释放后，本写者不得提交。
	  if l.ReservationID != "" {
		  r, rerr := e.store.GetReservation(ctx, l.ReservationID)
		   if rerr != nil || r.State != ResHeld {
			   live := e.liveFromLease(l)
			   p := e.sealProof(ctx, l, live, DriftReservationExpire, amount, "0", false, "reservation not held")
			  if CanExpire(l.State) {
				   l, _ = e.expireOrRevoke(ctx, l, LeaseEvExpire, "reservation_expired")
			   }
			  return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(DriftReservationExpire, ErrReservationExpired)
		  }
		  if !r.ExpiresAt.After(now) {
			  _, _ = e.releaseReservation(ctx, r, "expired")
			   live := e.liveFromLease(l)
			  p := e.sealProof(ctx, l, live, DriftReservationExpire, amount, r.Amount, false, "reservation ttl")
			  l, _ = e.expireOrRevoke(ctx, l, LeaseEvExpire, "reservation_expired")
			   return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(DriftReservationExpire, ErrReservationExpired)
		   }
	  } else {
		  live := e.liveFromLease(l)
		   p := e.sealProof(ctx, l, live, DriftNotReserved, amount, "0", false, "not reserved")
		   return RecordAssertResp{LeaseID: l.ID,
			   State: l.State, DriftProof: &p}, wrapDrift(DriftNotReserved, ErrLeaseNotReserved)
	  }
	   live := e.liveFromLease(l)
	  livePrint := ComputeViewPrint(live)
	   if !FingerprintsEqual(l.WindowPrint, livePrint) {
		  p := e.sealProof(ctx, l, live, DriftWindowChanged, amount, "0", false, "window hits changed after snapshot")
		   if r, err := e.store.GetReservation(ctx, l.ReservationID); err == nil {
			  _, _ = e.releaseReservation(ctx, r, "window_drift")
		   }
		  l, _ = e.expireOrRevoke(ctx, l, LeaseEvRevoke, "window_changed")
		  return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(DriftWindowChanged, ErrWindowDrift)
	  }
	  if !FingerprintsEqual(l.CapPrint, ComputeCapPrint(l.DailyCap, l.VelocityCap)) {
		   p := e.sealProof(ctx, l, live, DriftFingerprint, amount, "0", false, "cap fingerprint")
		  return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(DriftFingerprint, ErrWindowDrift)
	  }
	   capAmt, err := money.Parse(l.DailyCap)
	  if err != nil {
		   return RecordAssertResp{}, ErrCapInvalid
	   }
	  attempt, err := money.Parse(l.Amount)
	  if err != nil {
		   return RecordAssertResp{}, ErrAmountInvalid
	   }
	  today := e.todayAmt(l.Kind, l.UserID, l.Asset)
	   if WouldExceed(today, money.Zero(), attempt, capAmt) {
		   held, _ := e.store.ListHeldByWindow(ctx, wk)
		   others := HeldExcept(held, l.ID)
		  p := e.sealProof(ctx, l, live, DriftConcurrentOverCap, l.Amount, others.String(), true, "concurrent record would exceed cap")
		   if r, err := e.store.GetReservation(ctx, l.ReservationID); err == nil {
			  _, _ = e.releaseReservation(ctx, r, "over_cap")
		  }
		   l, _ = e.expireOrRevoke(ctx, l, LeaseEvRevoke, "concurrent_over_cap")
		   return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(DriftConcurrentOverCap, ErrWouldExceedCap)
	  }
	  if l.VelocityCap > 0 && live.Velocity24 >= l.VelocityCap {
		  p := e.sealProof(ctx, l, live, DriftVelocity, l.Amount, "0", true, "velocity would exceed")
		   if r, err := e.store.GetReservation(ctx, l.ReservationID); err == nil {
			  _, _ = e.releaseReservation(ctx, r, "velocity")
		  }
		   l, _ = e.expireOrRevoke(ctx, l, LeaseEvRevoke, "velocity")
		   return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, wrapDrift(DriftVelocity, ErrWouldExceedCap)
	   }
	   if e.commit == nil {
		  return RecordAssertResp{}, ErrUsageUnavailable
	  }
	   hitID, err := e.commit(ctx, l)
	  if err != nil {
		  // 提交失败不计占用：不推进 committed，释放预约。
		   if r, rerr := e.store.GetReservation(ctx, l.ReservationID); rerr == nil {
			  _, _ = e.releaseReservation(ctx, r, "commit_failed")
		  }
		  live = e.liveFromLease(l)
		  p := e.sealProof(ctx, l, live, DriftFingerprint, l.Amount, "0", false, err.Error())
		  return RecordAssertResp{LeaseID: l.ID, State: l.State, DriftProof: &p}, err
	   }
	   if r, rerr := e.store.GetReservation(ctx, l.ReservationID); rerr == nil {
		   _, _ = e.consumeReservation(ctx, r)
	   }
	  l, err = e.transit(ctx, l, LeaseEvCommit)
	   if err != nil {
		  return RecordAssertResp{LeaseID: l.ID},
			   err
	   }
	   now = e.now()
	   l.CommittedAt = &now
	  l.HitID = hitID
	   l.UpdatedAt = now
	  if _, err := e.store.SaveLease(ctx, l); err != nil {
		   return RecordAssertResp{}, err
	  }
	   todayAfter := e.todayAmt(l.Kind, l.UserID, l.Asset)
	  return RecordAssertResp{
		  LeaseID: l.ID, State: l.State, Committed: true, HitID: hitID, TodaySum: todayAfter.String(),
	   }, nil
  }

// RecordBare 裸写入失败即关，保持失败单不计占用。
  func (e *Engine) RecordBare(ctx context.Context, kind string, user, asset, dest, amount string) error {
	   _, err := e.RecordAssert(ctx, RecordAssertReq{
		  Kind: NormalizeKindOrEmpty(kind), UserID: user, Asset: asset, Dest: dest, Amount: amount,
	  })
	   return err
   }
