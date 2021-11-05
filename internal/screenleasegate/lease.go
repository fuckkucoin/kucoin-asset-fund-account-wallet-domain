package screenleasegate

import (
	"context"
	"time"
)

type LeaseFSM struct {
	store Store
	secret string
	clock func() time.Time
}

   func newLeaseFSM(store Store, secret string, clock func() time.Time) *LeaseFSM {
	  if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &LeaseFSM{store: store, secret: secret, clock: clock}
}

  func (f *LeaseFSM) Issue(ctx context.Context, req IssueReq, snap ScreenSnap, win WindowSnap, gen ScreenGeneration) (ScreenLease, error) {
	   if snap.Allow == false {
		   return ScreenLease{}, ErrScreenDenied
	  }
	   user, err := normalizeUser(req.UserID)
	  if err != nil {
		   return ScreenLease{}, err
	   }
	  asset, err := normalizeAsset(req.Asset)
	  if err != nil {
		   return ScreenLease{}, err
	  }
	  amt, err := normalizeAmount(req.Amount)
	   if err != nil {
		  return ScreenLease{}, err
	  }
	  dest,
		   err := normalizeDest(req.Dest)
	   if err != nil {
		  return ScreenLease{}, err
	   }
	   nonce, err := normalizeNonce(req.Nonce)
	   if err != nil {
		   return ScreenLease{}, err
	  }
	  if err := rejectIsolationFields(req.Kind); err != nil {
		  return ScreenLease{}, err
	   }
	  if gen.HaltOn {
		  return ScreenLease{}, ErrScreenHalt
	   }
	   if existing, ok, err := f.store.FindLeaseByNonce(ctx, user, nonce); err != nil {
		  return ScreenLease{}, err
	  } else if ok {
		   if existing.Status.Terminal() {
			   return ScreenLease{}, ErrIdempotentNonce
		  }
		  return existing, nil
	  }
	   open, err := f.store.ListUserOpen(ctx, user)
	   if err != nil {
		   return ScreenLease{}, err
	   }
	  if len(open) >= MaxOpenLeasesPerUser {
		   return ScreenLease{},
			  ErrLeaseQuota
	   }
	   ttl := DefaultLeaseTTL
	  if req.TTLSeconds > 0 && req.TTLSeconds <= 3600 {
		   ttl = time.Duration(req.TTLSeconds) * time.Second
	  }
	  now := f.clock()
	  pack := normalizePack(firstNonEmpty(snap.Pack, req.Pack))
	   mode := normalizeMode(firstNonEmpty(snap.Mode, gen.Mode))
	  fp := gen.PackFP
	  if fp == "" {
		  fp = packFingerprint(f.secret, pack, mode, "riskgate-engine")
	   }
	   lease := ScreenLease{
		  ID:     newID("sl-", user, nonce, now.Format(time.RFC3339Nano)),
		   UserID: user,
		   Dest:   dest,
		   Asset: asset,
		  Amount: amt,
		   Kind: normalizeKind(req.Kind),
		   Pack:   pack,
		  Mode:   mode,
		   PackFP: fp,
		  HaltGen: gen.HaltGen,
		  PackGen: gen.PackGen,
		  WindowDigest: windowDigest(f.secret, win, user, dest, asset),
		  Velocity1h:   win.Velocity1h,
		  Velocity24h:  win.Velocity24h,
		   DestCount:    win.DestCount,
		  DestCount24h: win.DestCount24h,
		   Nonce:        nonce,
		   Status:       LeaseOpen,
		   IsolationMark: IsolationMark,
		  IssuedAt:      now,
		  ExpiresAt: now.Add(ttl),
	  }
	   if err := f.store.PutLease(ctx, lease); err != nil {
		   return ScreenLease{}, err
	  }
	   return lease, nil
  }

  func (f *LeaseFSM) Bind(ctx context.Context, req SubmitReq, gen ScreenGeneration, win WindowSnap) (ScreenLease, error) {
	   if stringsTrim(req.LeaseID) == "" {
		   return ScreenLease{}, ErrLeaseMissing
	  }
	  lease, err := f.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   return ScreenLease{}, err
	   }
	   now := f.clock()
	  if !lease.Alive(now) {
		  if lease.Status == LeaseRevoked {
			   if gen.HaltOn || lease.HaltGen != gen.HaltGen {
				  return lease, ErrHaltFlip
			  }
			   if lease.PackGen != gen.PackGen {
				   return lease, ErrPackFlip
			   }
			   return lease, ErrLeaseRevoked
		  }
		   if lease.Status == LeaseConsumed {
			   return lease, ErrLeaseConsumed
		  }
		   return lease, ErrLeaseExpired
	   }
	  if err := MatchLease(lease, gen); err != nil {
		  return lease, err
	   }
	  user, err := normalizeUser(req.UserID)
	   if err != nil {
		   return lease, err
	  }
	   if user != lease.UserID {
		   return lease, ErrLeaseUser
	   }
	   asset, err := normalizeAsset(req.Asset)
	  if err != nil {
		  return lease, err
	  }
	   if asset != lease.Asset {
		   return lease, ErrLeaseAsset
	  }
	   amt,
		  err := normalizeAmount(req.Amount)
	   if err != nil {
		   return lease, err
	  }
	  if amt != lease.Amount {
		   return lease, ErrLeaseAmount
	   }
	  dest, err := normalizeDest(req.Dest)
	  if err != nil {
		   return lease, err
	   }
	  if dest != lease.Dest {
		   return lease, ErrLeaseDest
	   }
	   ticket, err := normalizeTicket(req.Ticket)
	   if err != nil {
		  return lease, err
	   }
	   if _, err := normalizeActor(req.Actor); err != nil {
		  return lease, err
	   }
	   curDigest := windowDigest(f.secret, win, user, dest, asset)
	   if lease.WindowDigest != "" && curDigest != lease.WindowDigest {
		   // 窗摘要变化表示速度/目的地统计在 Allow 后漂移。
		  return lease, ErrWindowDrift
	   }
	   if !lease.Status.CanBind() && lease.Status != LeaseBound {
		   return lease, ErrLeaseNotOpen
	  }
	  if lease.Status == LeaseOpen {
		   lease.Status = LeaseBound
		  t := now
		   lease.BoundAt = &t
		   lease.BindTicket = ticket
		   if err := f.store.UpdateLease(ctx, lease); err != nil {
			   return ScreenLease{}, err
		   }
	   }
	  if req.Consume {
		   return f.consumeLocked(ctx,
			   lease, now)
	   }
	   return lease, nil
   }

   func (f *LeaseFSM) consumeLocked(ctx context.Context, lease ScreenLease, now time.Time) (ScreenLease, error) {
	  if lease.Status == LeaseConsumed {
		   return lease, nil
	  }
	  if !lease.Status.CanConsume() {
		  return lease, ErrLeaseTerminal
	   }
	   lease.Status = LeaseConsumed
	   t := now
	  lease.ConsumedAt = &t
	  lease.TerminalAt = &t
	  if err := f.store.UpdateLease(ctx, lease); err != nil {
		  return ScreenLease{}, err
	  }
	  return lease, nil
   }

   func (f *LeaseFSM) ExpireDue(ctx context.Context) (int, error) {
	   list, err := f.store.ListOpenLeases(ctx)
	   if err != nil {
		   return 0, err
	   }
	   now := f.clock()
	   n := 0
	   for _, l := range list {
		  if l.ExpiresAt.IsZero() || now.Before(l.ExpiresAt) {
			   continue
		   }
		  l.Status = LeaseExpired
		  t := now
		   l.TerminalAt = &t
		   if err := f.store.UpdateLease(ctx, l); err != nil {
			  return n, err
		   }
		   n++
	  }
	  return n, nil
  }

  func (f *LeaseFSM) RevokeOpen(ctx context.Context, reason string) (int, []ScreenLease, error) {
	   list, err := f.store.ListOpenLeases(ctx)
	  if err != nil {
		  return 0, nil, err
	  }
	  now := f.clock()
	  out := []ScreenLease{}
	   for _, l := range list {
		  if l.Status.Terminal() {
			   continue
		  }
		   l.Status = LeaseRevoked
		  l.Note = reason
		   t := now
		   l.TerminalAt = &t
		  if err := f.store.UpdateLease(ctx, l); err != nil {
			   return len(out), out, err
		  }
		   out = append(out, l)
	   }
	   return len(out), out, nil
   }

   func firstNonEmpty(a, b string) string {
	   if stringsTrim(a) != "" {
		   return a
	  }
	  return b
   }

  func stringsTrim(s string) string {
	   for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		   s = s[1:]
	   }
	   for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		  s = s[:len(s)-1]
	  }
	   return s
   }
