package scoreleasegate

import (
	"context"
	"time"
)

type LeaseFSM struct {
	store  Store
	secret string
	clock func() time.Time
}

   func newLeaseFSM(store Store, secret string, clock func() time.Time) *LeaseFSM {
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &LeaseFSM{store: store, secret: secret, clock: clock}
}

   func (f *LeaseFSM) Issue(ctx context.Context, req IssueReq, snap EvalSnap, gen ScoreGeneration) (ScoreLease, error) {
	   if !snap.Allow {
		  return ScoreLease{}, ErrEvalDenied
	   }
	   if snap.Mode == "shadow" || normalizeMode(snap.Mode) == "shadow" {
		  return ScoreLease{}, ErrShadowLease
	   }
	  if snap.Hold {
		  return ScoreLease{}, ErrHoldReview
	   }
	  user, err := normalizeUser(firstNonEmpty(req.UserID, snap.UserID))
	   if err != nil {
		   return ScoreLease{}, err
	  }
	  asset, err := normalizeAsset(firstNonEmpty(req.Asset, snap.Asset))
	   if err != nil {
		   return ScoreLease{}, err
	  }
	   chain, err := normalizeChain(firstNonEmpty(req.Chain, snap.Chain))
	  if err != nil {
		   return ScoreLease{}, err
	   }
	   amt, err := normalizeAmount(firstNonEmpty(req.Amount, snap.Amount))
	   if err != nil {
		   return ScoreLease{}, err
	  }
	   dest, err := normalizeDest(firstNonEmpty(req.Dest, snap.Dest))
	   if err != nil {
		  return ScoreLease{}, err
	   }
	   nonce, err := normalizeNonce(req.Nonce)
	   if err != nil {
		  return ScoreLease{}, err
	  }
	  if err := rejectIsolationFields(req.Kind + req.Nonce + req.Note); err != nil {
		   return ScoreLease{}, err
	  }
	  if gen.HaltOn {
		   return ScoreLease{}, ErrEvalHalt
	   }
	  if existing, ok, err := f.store.FindLeaseByNonce(ctx, user, nonce); err != nil {
		   return ScoreLease{}, err
	   } else if ok {
		  if existing.Status.Terminal() {
			  return ScoreLease{}, ErrIdempotentNonce
		  }
		  return existing, nil
	  }
	   open, err := f.store.ListUserOpen(ctx, user)
	  if err != nil {
		   return ScoreLease{}, err
	  }
	   if len(open) >= MaxOpenLeasesPerUser {
		  return ScoreLease{}, ErrLeaseQuota
	  }
	   ttl := DefaultLeaseTTL
	  if req.TTLSeconds > 0 && req.TTLSeconds <= 3600 {
		  ttl = time.Duration(req.TTLSeconds) * time.Second
	  }
	   now := f.clock()
	   pack := normalizePack(firstNonEmpty(snap.Pack, req.Pack, gen.Pack))
	  mode := normalizeMode(firstNonEmpty(snap.Mode, gen.Mode))
	   if mode != "enforce" {
		  return ScoreLease{}, ErrShadowLease
	  }
	  fp := gen.PackFP
	   if fp == "" {
		  fp = packFingerprint(f.secret, pack, mode, "riskscore-engine")
	   }
	  win := snap.Window
	  if win.Digest == "" {
		  win.Digest = windowDigest(f.secret, win)
	   }
	   lease := ScoreLease{
		   ID: newID("scl-", user, nonce, now.Format(time.RFC3339Nano)),
		   UserID: user,
		   Chain:  chain,
		   Dest:   dest,
		  Asset:  asset,
		   Amount: amt,
		  Kind:   normalizeKind(req.Kind),
		   Pack:   pack,
		   Mode: mode,
		  PackFP: fp,
		  HaltGen: gen.HaltGen,
		   PackGen: gen.PackGen,
		  ModeGen: gen.ModeGen,
		   WindowGen:    gen.WindowGen,
		  WindowDigest: win.Digest,
		  Velocity1h:   win.Velocity1h,
		  Velocity24h:  win.Velocity24h,
		  DestCount:    win.DestCount,
		   DestCount24h: win.DestCount24h,
		   ClusterSize: win.ClusterSize,
		   Correlated: win.Correlated,
		   NewDests: win.NewDests,
		  Fingerprint: assetChainDestFP(f.secret,
			   asset, chain, dest),
		   Nonce:  nonce,
		  Status: LeaseIssued,
		  IsolationMark: IsolationMark,
		  Note:      req.Note,
		  IssuedAt:  now,
		   ExpiresAt: now.Add(ttl),
		   EvalAllow:  true,
		   EvalReason: snap.Reason,
	  }
	  if err := f.store.PutLease(ctx, lease); err != nil {
		  return ScoreLease{}, err
	  }
	  return lease, nil
   }

  func (f *LeaseFSM) Bind(ctx context.Context, lease ScoreLease, ticket string, now time.Time) (ScoreLease, error) {
	  if lease.Status.Terminal() {
		  if lease.Status == LeaseExpired {
			  return lease, ErrLeaseExpired
		   }
		  if lease.Status == LeaseRevoked {
			  return lease, ErrLeaseRevoked
		  }
		   if lease.Status == LeaseConsumed {
			   return lease, ErrLeaseConsumed
		   }
		   return lease, ErrLeaseTerminal
	   }
	   if !lease.Status.CanBind() && lease.Status != LeaseBound {
		   return lease, ErrLeaseNotOpen
	   }
	   if lease.ExpiredAt(now) {
		   t := now
		  lease.Status = LeaseExpired
		   lease.TerminalAt = &t
		   _ = f.store.UpdateLease(ctx,
			   lease)
		   return lease, ErrLeaseExpired
	  }
	   if lease.Status == LeaseIssued {
		   t := now
		  lease.Status = LeaseBound
		   lease.BoundAt = &t
		   lease.BindTicket = ticket
		  if err := f.store.UpdateLease(ctx, lease); err != nil {
			  return lease, err
		  }
	  }
	  return lease, nil
   }

  func (f *LeaseFSM) Consume(ctx context.Context, lease ScoreLease, now time.Time) (ScoreLease, error) {
	   if !lease.Status.CanConsume() {
		  if lease.Status == LeaseConsumed {
			   return lease, nil
		   }
		   return lease, ErrLeaseNotOpen
	  }
	   if lease.ExpiredAt(now) {
		  t := now
		   lease.Status = LeaseExpired
		  lease.TerminalAt = &t
		   _ = f.store.UpdateLease(ctx, lease)
		  return lease, ErrLeaseExpired
	   }
	   t := now
	  lease.Status = LeaseConsumed
	  lease.ConsumedAt = &t
	   lease.TerminalAt = &t
	   if err := f.store.UpdateLease(ctx, lease); err != nil {
		   return lease, err
	   }
	   return lease, nil
  }

  func (f *LeaseFSM) Revoke(ctx context.Context, lease ScoreLease, now time.Time, note string) (ScoreLease, error) {
	   if lease.Status.Terminal() {
		   return lease, nil
	  }
	   t := now
	  lease.Status = LeaseRevoked
	  lease.TerminalAt = &t
	  if note != "" {
		  lease.Note = note
	  }
	  if err := f.store.UpdateLease(ctx, lease); err != nil {
		  return lease, err
	  }
	   return lease, nil
  }

  func (f *LeaseFSM) Expire(ctx context.Context, lease ScoreLease, now time.Time) (ScoreLease, error) {
	   if lease.Status.Terminal() {
		   return lease, nil
	  }
	  if !lease.ExpiredAt(now) {
		  return lease, nil
	   }
	   t := now
	   lease.Status = LeaseExpired
	   lease.TerminalAt = &t
	  if err := f.store.UpdateLease(ctx, lease); err != nil {
		   return lease, err
	  }
	   return lease, nil
   }

   func amountsEqual(a, b string) bool {
	  left, err := normalizeAmount(a)
	   if err != nil {
		   return a == b
	   }
	  right, err := normalizeAmount(b)
	   if err != nil {
		  return a == b
	   }
	   return left == right
   }

  func matchSubmitFields(lease ScoreLease, req SubmitReq) error {
	  if req.UserID != "" && req.UserID != lease.UserID {
		   return ErrLeaseUser
	  }
	  if req.Amount != "" && !amountsEqual(req.Amount, lease.Amount) {
		   return ErrLeaseAmount
	   }
	  if req.Dest != "" && req.Dest != lease.Dest {
		  return ErrLeaseDest
	  }
	   if req.Asset != "" && req.Asset != lease.Asset {
		   return ErrLeaseAsset
	   }
	  if req.Chain != "" && req.Chain != lease.Chain {
		   return ErrLeaseChain
	  }
	   return nil
  }
