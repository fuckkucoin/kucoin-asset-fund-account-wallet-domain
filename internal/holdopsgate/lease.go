package holdopsgate

import (
	"context"
	"strconv"
	"time"
)

type LeaseFSM struct {
	store  Store
	secret string
	clock func() time.Time
}

   func newLeaseFSM(store Store, secret string, clock func() time.Time) *LeaseFSM {
	   return &LeaseFSM{store: store, secret: secret, clock: clock}
   }

   func (f *LeaseFSM) Issue(ctx context.Context, req IssueReq, gen HoldOpsGeneration) (HoldOpsLease, error) {
	  if f == nil || f.store == nil {
		  return HoldOpsLease{},
			  ErrUnavailable
	   }
	  if gen.FenceOn {
		  return HoldOpsLease{}, ErrFenceOn
	  }
	  if err := rejectIsolation(req.Note, req.Nonce, req.Reason, req.Ref); err != nil {
		   return HoldOpsLease{}, err
	   }
	  kind, err := normKind(req.Kind)
	   if err != nil {
		   return HoldOpsLease{}, err
	   }
	   if kind != OpsPlace && kind != OpsRelease && kind != OpsGapAck && kind != OpsGapClose {
		  return HoldOpsLease{}, ErrKindInvalid
	   }
	  user, err := normUser(req.UserID)
	   if err != nil {
		   return HoldOpsLease{}, err
	  }
	   asset, err := normAsset(req.Asset)
	  if err != nil {
		   return HoldOpsLease{}, err
	  }
	  amount, err := normAmount(req.Amount)
	  if err != nil {
		   return HoldOpsLease{}, err
	   }
	  actor, err := normActor(req.Actor)
	  if err != nil {
		   return HoldOpsLease{}, err
	  }
	  nonce, err := normNonce(req.Nonce)
	   if err != nil {
		  return HoldOpsLease{}, err
	   }
	   if prev, ok, e := f.store.FindLeaseByNonce(ctx, actor, nonce); e == nil && ok {
		   if prev.Kind == kind && prev.UserID == user && prev.TokenID == req.TokenID {
			  return prev, nil
		  }
		  return HoldOpsLease{}, ErrIdempotentNonce
	  }
	  tokenID := stringsTrim(req.TokenID)
	  gapID := stringsTrim(req.GapID)
	   if kind == OpsRelease && tokenID == "" {
		   return HoldOpsLease{}, ErrTokenInvalid
	  }
	   if (kind == OpsGapAck || kind == OpsGapClose) && gapID == "" {
		  return HoldOpsLease{}, ErrGapInvalid
	   }
	   if kind == OpsPlace && tokenID == "" {
		   tokenID = "tok_" + signFields(f.secret, "shadow-token", user, asset, nonce)[:20]
	  }
	   openTok, _ := f.store.ListTokenOpen(ctx, tokenID)
	  if tokenID != "" && len(openTok) >= MaxOpenLeasesPerToken {
		  return HoldOpsLease{}, ErrLeaseQuota
	   }
	   openAct, _ := f.store.ListActorOpen(ctx, actor)
	  if len(openAct) >= MaxOpenLeasesPerActor {
		  return HoldOpsLease{},
			  ErrLeaseQuota
	   }
	   now := f.clock()
	  ttl := DefaultLeaseTTL
	  if req.TTLSec > 0 && req.TTLSec <= 3600 {
		   ttl = time.Duration(req.TTLSec) * time.Second
	  }
	   opsID := newOpsID(f.secret, actor, nonce, string(kind))
	  lease := HoldOpsLease{
		  ID: newLeaseID(f.secret, opsID, tokenID, gapID),
		  OpsID: opsID,
		   Kind:  kind,
		  UserID: user,
		   Asset:  asset,
		  Amount: amount,
		   TokenID: tokenID,
		  GapID:   gapID,
		  Ref:     stringsTrim(req.Ref),
		  Reason:  stringsTrim(req.Reason),
		   Actor:   actor,
		  Nonce: nonce,
		   OpsGen: gen.OpsGen,
		  Status: LeaseOpen,
		  IsolationMark: IsolationMark,
		  Note:          stringsTrim(req.Note),
		  IssuedAt:      now,
		  ExpiresAt:     now.Add(ttl),
	   }
	   if err := f.store.PutLease(ctx, lease); err != nil {
		  return HoldOpsLease{}, err
	  }
	   return lease, nil
   }

   func (f *LeaseFSM) Bind(ctx context.Context, lease HoldOpsLease, ticket string) (HoldOpsLease, error) {
	   if lease.ExpiredAt(f.clock()) {
		  lease.Status = LeaseExpired
		  lease.TerminalAt = f.clock()
		   _ = f.store.UpdateLease(ctx, lease)
		   return lease, ErrLeaseExpired
	   }
	  if lease.Status.Terminal() {
		   if lease.Status == LeaseExpired {
			   return lease, ErrLeaseExpired
		   }
		   if lease.Status == LeaseRevoked {
			  return lease, ErrLeaseRevoked
		   }
		   return lease, ErrLeaseConsumed
	   }
	   if !lease.Status.CanBind() && lease.Status != LeaseBound {
		   return lease, ErrLeaseNotOpen
	   }
	  if lease.Status == LeaseOpen {
		  to, ok := CanTransit(lease.Status, "bind")
		   if !ok {
			   return lease, ErrLeaseNotOpen
		   }
		  lease.Status = to
		   lease.BoundAt = f.clock()
		  lease.BindTicket = ticket
		  if err := f.store.UpdateLease(ctx, lease); err != nil {
			  return lease, err
		  }
	   }
	   return lease, nil
  }

  func (f *LeaseFSM) Consume(ctx context.Context, lease HoldOpsLease) (HoldOpsLease, error) {
	  if !lease.Status.CanConsume() {
		  return lease, ErrLeaseTerminal
	   }
	   to, ok := CanTransit(lease.Status, "consume")
	  if !ok {
		   return lease, ErrLeaseTerminal
	  }
	   now := f.clock()
	  lease.Status = to
	   lease.ConsumedAt = now
	  lease.TerminalAt = now
	   if err := f.store.UpdateLease(ctx, lease); err != nil {
		   return lease, err
	   }
	  return lease, nil
   }

   func (f *LeaseFSM) ExpireOne(ctx context.Context, lease HoldOpsLease) (HoldOpsLease, error) {
	   if lease.Status.Terminal() {
		   return lease, ErrLeaseTerminal
	   }
	   to, ok := CanTransit(lease.Status, "expire")
	   if !ok {
		   return lease, ErrLeaseTerminal
	  }
	   now := f.clock()
	  lease.Status = to
	  lease.TerminalAt = now
	   lease.Note = firstNonEmpty(lease.Note, "ttl-expire")
	  if err := f.store.UpdateLease(ctx, lease); err != nil {
		   return lease, err
	  }
	   return lease, nil
  }

  func (f *LeaseFSM) RevokeOne(ctx context.Context, lease HoldOpsLease, note string) (HoldOpsLease, error) {
	   if lease.Status.Terminal() {
		   return lease, ErrLeaseTerminal
	   }
	  to, ok := CanTransit(lease.Status, "revoke")
	   if !ok {
		  return lease, ErrLeaseTerminal
	  }
	   now := f.clock()
	   lease.Status = to
	  lease.TerminalAt = now
	  lease.Note = firstNonEmpty(note, lease.Note)
	   if err := f.store.UpdateLease(ctx, lease); err != nil {
		   return lease, err
	   }
	  return lease, nil
  }

  func stringsTrim(s string) string {
	   return trimSpace(s)
  }

   func trimSpace(s string) string {
	   for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		   s = s[1:]
	   }
	  for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		   s = s[:len(s)-1]
	   }
	  return s
   }

  func genString(n int64) string { return strconv.FormatInt(n, 10) }
