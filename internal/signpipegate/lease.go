package signpipegate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// LeaseFSM 推进在途租约。

type LeaseFSM struct {
	store Store
	gen   *GenerationEngine
	clock func() time.Time
}

   func newLeaseFSM(store Store, gen *GenerationEngine, clock func() time.Time) *LeaseFSM {
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &LeaseFSM{store: store, gen: gen, clock: clock}
}

   func (f *LeaseFSM) now() time.Time {
	  if f.clock == nil {
		  return time.Now().UTC()
	  }
	   return f.clock()
   }

// Arm 幂等或 CAS 抬世代后落 armed。
   func (f *LeaseFSM) Arm(ctx context.Context, req ArmReq) (*InflightLease, error) {
	  if f == nil || f.store == nil {
		   return nil, ErrUnavailable
	   }
	  if err := validateArm(req); err != nil {
		  return nil, err
	  }
	  if exist, err := f.store.GetArtifactByMovement(ctx, req.MovementID); err != nil {
		  return nil, err
	  } else if exist != nil {
		  return nil, ErrDoubleArtifact
	  }
	  if hit, err := f.store.GetLeaseByNonce(ctx, req.MovementID, req.Nonce); err != nil {
		  return nil, err
	   } else if hit != nil && !hit.Terminal() && !hit.ExpiredAt(f.now()) {
		   cp := hit.Copy()
		   return &cp, nil
	   }
	   if active, err := f.store.FindActiveLease(ctx, req.MovementID); err != nil {
		   return nil, err
	  } else if active != nil {
		  return nil, ErrInflightOccupied
	  }
	   if blocks, err := f.store.ListBlockingCloseouts(ctx, req.MovementID); err != nil {
		  return nil, err
	  } else if len(blocks) > 0 {
		   return nil, ErrCloseoutBlocked
	  }
	   if faults, err := f.store.ListBlockingFaults(ctx, req.MovementID); err != nil {
		  return nil, err
	   } else if len(faults) > 0 {
		  // 未收口故障挡新重试，防止故障窗口再签。
		  for _, fc := range faults {
			   if fc.Kind == FaultDualBroadcast || fc.Kind == FaultRetryExhausted {
				  return nil, ErrFaultBlocked
			  }
		   }
	  }
	   cur, err := f.gen.Current(ctx, req.MovementID, req.JobID)
	   if err != nil {
		   return nil, err
	   }
	   next, err := f.gen.Bump(ctx, req.MovementID, req.JobID, cur)
	   if err != nil {
		  return nil, err
	  }
	   ttl := req.TTL
	  if ttl <= 0 {
		  ttl = DefaultLeaseTTL
	   }
	   now := f.now()
	   canon := Canonical(req.MovementID, req.Chain, req.To, req.Amount)
	   id := uuid.NewString()
	  l := &InflightLease{
		   ID:              id,
		  InflightLeaseID: id,
		  MovementID:      NormalizeMovement(req.MovementID),
		   JobID:           strings.TrimSpace(req.JobID),
		  Chain: NormalizeChain(req.Chain),
		   To:    NormalizeDest(req.To),
		   Amount:     strings.TrimSpace(req.Amount),
		   Canonical:  canon,
		   Nonce:      NormalizeNonce(req.Nonce),
		   Generation: next,
		  Status:     LeaseArmed,
		  Actor:      NormalizeActor(req.Actor),
		  FenceEpoch: req.FenceEpoch,
		   FenceToken:    strings.TrimSpace(req.FenceToken),
		  IsolationMark: IsolationMark,
		   ArmedAt: now,
		  ExpiresAt: now.Add(ttl),
	  }
	   if err := isolationOK(l.IsolationMark); err != nil {
		  return nil, err
	  }
	  if err := f.store.SaveLease(ctx, l); err != nil {
		  return nil, err
	   }
	  cp := l.Copy()
	  return &cp, nil
   }

// EnterInflight armed → inflight。
   func (f *LeaseFSM) EnterInflight(ctx context.Context, leaseID string) (*InflightLease, error) {
	   l, err := f.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return nil, err
	   }
	  if l.Terminal() {
		   return nil, ErrLeaseTerminal
	  }
	   if l.ExpiredAt(f.now()) {
		   return nil, f.expire(ctx, l)
	  }
	  if l.Status != LeaseArmed && l.Status != LeaseInflight {
		  return nil, ErrLeaseNotArmed
	   }
	  if err := f.gen.MustMatch(ctx, l); err != nil {
		  return nil, err
	   }
	   if l.Status == LeaseArmed {
		   l.Status = LeaseInflight
		  l.InflightAt = f.now()
		   if err := f.store.SaveLease(ctx, l); err != nil {
			  return nil, err
		   }
	   }
	   cp := l.Copy()
	   return &cp, nil
  }

  func (f *LeaseFSM) expire(ctx context.Context, l *InflightLease) error {
	  if l.Terminal() {
		  return ErrLeaseExpired
	  }
	  l.Status = LeaseExpired
	   l.TerminalAt = f.now()
	   l.Note = "ttl"
	   if err := f.store.SaveLease(ctx, l); err != nil {
		   return err
	  }
	   return ErrLeaseExpired
  }

// Seal inflight → sealed，绑定产物。

  func (f *LeaseFSM) Seal(ctx context.Context, l *InflightLease, sig string) (*InflightLease, error) {
	   if l == nil {
		  return nil, ErrLeaseNotFound
	   }
	   if l.Terminal() {
		  return nil, ErrLeaseTerminal
	   }
	  if l.ExpiredAt(f.now()) {
		  return nil, f.expire(ctx, l)
	  }
	   if l.Status != LeaseInflight && l.Status != LeaseArmed {
		   return nil, ErrLeaseNotInflight
	  }
	   if err := f.gen.MustMatch(ctx, l); err != nil {
		   return nil, err
	  }
	  if exist, err := f.store.GetArtifactByMovement(ctx, l.MovementID); err != nil {
		  return nil, err
	  } else if exist != nil {
		  return nil, ErrDoubleArtifact
	   }
	  now := f.now()
	  l.Status = LeaseSealed
	   l.SigHex = strings.TrimSpace(sig)
	   l.SealedAt = now
	   l.TerminalAt = now
	  if err := f.store.SaveLease(ctx, l); err != nil {
		   return nil, err
	  }
	   cp := l.Copy()
	   return &cp, nil
   }

// Abort 中止。
  func (f *LeaseFSM) Abort(ctx context.Context, leaseID, reason string) (*InflightLease, error) {
	   l, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return nil, err
	   }
	   if l.Status == LeaseSealed {
		  return nil, ErrSealed
	   }
	   if l.Terminal() {
		   cp := l.Copy()
		   return &cp, nil
	  }
	  l.Status = LeaseAborted
	   l.Note = strings.TrimSpace(reason)
	   l.TerminalAt = f.now()
	  if err := f.store.SaveLease(ctx, l); err != nil {
		  return nil, err
	  }
	  cp := l.Copy()
	  return &cp, nil
  }

// Supersede 旧租约被新世代顶替。

  func (f *LeaseFSM) Supersede(ctx context.Context, l *InflightLease) error {
	  if l == nil || l.Terminal() {
		  return nil
	  }
	  l.Status = LeaseSuperseded
	  l.Note = "superseded"
	   l.TerminalAt = f.now()
	   return f.store.SaveLease(ctx, l)
   }

// ExpireDue 扫描过期 armed/inflight。
  func (f *LeaseFSM) ExpireDue(ctx context.Context, limit int) (int, error) {
	  rows,
		   err := f.store.ListExpiredArmed(ctx, f.now(), limit)
	  if err != nil {
		  return 0, err
	  }
	   n := 0
	   for _, l := range rows {
		  if err := f.expire(ctx, l); err != nil && err != ErrLeaseExpired {
			  return n, err
		   }
		  n++
	   }
	   return n, nil
   }
