package feebumpgate

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Engine 占用收口引擎。
type Engine struct {
	store Store
	signer Signer
	metric *Metrics
	cfg GateConfig
	clock func() time.Time
}

// Option 构造项。

type Option func(*Engine)

// WithMetrics 挂指标。
   func WithMetrics(m *Metrics) Option {
	  return func(e *Engine) { e.metric = m }
   }

// WithConfig 闸配置。
  func WithConfig(c GateConfig) Option {
	   return func(e *Engine) {
		  c.Fill()
		   e.cfg = c
	  }
  }

// WithClock 时钟。

   func WithClock(fn func() time.Time) Option {
	  return func(e *Engine) {
		   if fn != nil {
			   e.clock = fn
		  }
	  }
}

// WithSigner 签章适配。
   func WithSigner(s Signer) Option {
	  return func(e *Engine) { e.signer = s }
   }

// NewEngine 构造。store 空则内存仓。
  func NewEngine(store Store, opts ...Option) *Engine {
	   if store == nil {
		   store = NewMemStore()
	   }
	  e := &Engine{
		   store: store,
		  clock: func() time.Time { return time.Now().UTC() },
		  metric: NewMetrics(),
	  }
	   e.cfg.Fill()
	   for _, opt := range opts {
		   opt(e)
	   }
	  return e
   }

// Store 只读仓。
   func (e *Engine) Store() Store {
	   if e == nil {
		   return nil
	  }
	   return e.store
   }

  func (e *Engine) now() time.Time {
	   if e == nil || e.clock == nil {
		  return time.Now().UTC()
	   }
	   return e.clock()
   }

// SignFeeBumpWithGen 签发世代并持有占用；二次抬费先退役上一 held。

   func (e *Engine) SignFeeBumpWithGen(ctx context.Context, req SignWithGenReq) (SignWithGenResp, error) {
	  if e == nil || e.store == nil {
		  return SignWithGenResp{}, ErrUnavailable
	  }
	  if err := validateSign(req); err != nil {
		  return SignWithGenResp{}, err
	  }
	   if req.IdempotencyKey != "" {
		   if old, err := e.store.GetGenByIdem(ctx, req.IdempotencyKey); err == nil {
			   occ, _ := e.store.GetOccByGen(ctx, old.GenID)
			  return SignWithGenResp{Gen: old, Occ: occ}, nil
		   } else if err != nil && !errors.Is(err, ErrGenNotFound) {
			   return SignWithGenResp{}, err
		   }
	  }
	   if err := e.guardOpenOrphan(ctx, req.TransferID); err != nil {
		  e.metric.IncFailClosed()
		   return SignWithGenResp{}, err
	  }
	   latest, latestErr := e.store.LatestGen(ctx, req.TransferID)
	  hasPrev := latestErr == nil
	  if hasPrev {
		   if e.cfg.RequirePrevHash || latest.NewBroadcastHash != "" {
			   if EmptyHash(req.PrevBroadcastHash) {
				  e.metric.IncFailClosed()
				  return SignWithGenResp{}, ErrPrevHashRequired
			  }
			   if !SameHash(req.PrevBroadcastHash, latest.NewBroadcastHash) {
				   p := e.recordProof(ctx, req.TransferID, latest.GenID, "", "", ProofPrevHashMismatch,
					   req.PrevBroadcastHash, latest.NewBroadcastHash, 0, latest.Generation,
					   "prev_broadcast_hash does not match latest gen", req.Actor)
				   e.metric.IncFailClosed()
				  return SignWithGenResp{Proof: &p}, ErrPrevHash
			  }
		   }
		   if !FeeRaised(latest.NewFee, req.NewFee) {
			   _ = e.recordProof(ctx, req.TransferID, latest.GenID, "", "", ProofFeeNotRaised,
				   req.NewFee, latest.NewFee, 0, latest.Generation, "new_fee not raised", req.Actor)
			  e.metric.IncFailClosed()
			  return SignWithGenResp{},
				   ErrFeeNotRaised
		   }
	  } else if latestErr != nil && !errors.Is(latestErr, ErrGenNotFound) {
		  return SignWithGenResp{}, latestErr
	  }
	   generation,
		  err := e.store.NextGeneration(ctx, req.TransferID)
	  if err != nil {
		  return SignWithGenResp{}, err
	  }
	   canon := strings.TrimSpace(req.Canon)
	  if canon == "" {
		   canon = BuildCanon(req.TransferID, req.Chain, req.To, req.Amount, req.NewFee, req.DestinationTag)
	   }
	   sig,
		   err := e.sign(canon, req.SigHex)
	  if err != nil {
		   return SignWithGenResp{}, err
	  }
	   now := e.now()
	  fp := OccupancyFingerprint(req.TransferID, req.PrevBroadcastHash, req.NewFee, req.DestinationTag, generation)
	  newHash := BroadcastHash(canon, hex.EncodeToString(sig), fp)
	   gid := uuid.NewString()
	  oid := uuid.NewString()
	   prevFee := ""
	   if hasPrev {
		  prevFee = latest.NewFee
	   }
	  gen := FeeBumpGen{
		  GenID: gid, TransferID: req.TransferID, Generation: generation,
		  PrevBroadcastHash: req.PrevBroadcastHash,
		   NewBroadcastHash:  newHash,
		   NewFee:            req.NewFee, PrevFee: prevFee, DestinationTag: req.DestinationTag,
		  OccupancyFP: fp, Chain: req.Chain, To: req.To, Amount: req.Amount,
		   Asset: req.Asset, CanonDigest: CanonDigest(canon), SigDigest: SigDigest(sig),
		  State: GenSigned, OccID: oid, IdempotencyKey: req.IdempotencyKey,
		  Actor: req.Actor, SignedAt: now, CreatedAt: now, UpdatedAt: now,
	   }
	   occ := BroadcastOccupancy{
		   OccID: oid, GenID: gid, TransferID: req.TransferID, Generation: generation,
		   BroadcastHash: newHash, OccupancyFP: fp, Asset: req.Asset, Amount: req.Amount,
		  Fee: req.NewFee, State: OccHeld, HeldAt: now, ExpireAt: now.Add(e.cfg.OccTTL),
		  Actor: req.Actor, CreatedAt: now, UpdatedAt: now,
	   }
	   if err := e.store.SaveGen(ctx, gen); err != nil {
		   return SignWithGenResp{}, err
	   }
	  if err := e.store.SaveOcc(ctx, occ); err != nil {
		   _ = e.blockGen(ctx, gen, "occupancy persist failed")
		  p := e.openOrphan(ctx, req.TransferID, gid, "", oid, ProofRetireFail, newHash, "", generation, generation, 0,
			  "signed but occupancy persist failed", req.Actor)
		   e.metric.IncFailClosed()
		   return SignWithGenResp{Gen: gen, Proof: &p}, ErrRetireFailed
	  }
	  if hasPrev {
		  if err := e.retirePrevious(ctx, latest, req.Actor); err != nil {
			   p := e.openOrphan(ctx, req.TransferID, gid, latest.GenID, latest.OccID, ProofRetireFail,
				   latest.NewBroadcastHash, newHash, latest.Generation, generation, 2,
				  "signed but previous occupancy retire failed", req.Actor)
			   held, _ := e.store.CountHeld(ctx, req.TransferID)
			   if held > 1 {
				  p = e.openOrphan(ctx, req.TransferID, gid, latest.GenID, oid, ProofDualOccupancy,
					  latest.NewBroadcastHash, newHash, latest.Generation, generation, held,
					  "dual hot occupancy after retire fail", req.Actor)
			  }
			   _ = e.blockGen(ctx, gen, "retire previous failed")
			   e.metric.IncOrphan()
			  e.metric.IncFailClosed()
			   return SignWithGenResp{Gen: gen, Occ: occ, Proof: &p}, ErrRetireFailed
		  }
	  }
	  held, err := e.store.CountHeld(ctx, req.TransferID)
	   if err != nil {
		  return SignWithGenResp{}, err
	   }
	  if held != 1 {
		   p := e.openOrphan(ctx, req.TransferID, gid, "", oid, ProofDualOccupancy,
			   "", newHash, 0, generation, held, "held occupancy count not exactly one", req.Actor)
		   _ = e.blockGen(ctx, gen, "dual occupancy")
		   e.metric.IncOrphan()
		  e.metric.IncFailClosed()
		   return SignWithGenResp{Gen: gen, Occ: occ, Proof: &p}, ErrDualOccupancy
	  }
	   _ = e.store.AppendAudit(ctx, GenAudit{
		  AuditID: uuid.NewString(), GenID: gid, TransferID: req.TransferID,
		   Action: "sign_with_gen", FromState: "", ToState: string(GenSigned),
		   Actor: req.Actor, Detail: "issued feebump_gen", At: now,
	  })
	   e.metric.IncSigned()
	   return SignWithGenResp{Gen: gen, Occ: occ}, nil
   }

// AssertOccupancy 复验最新 held 与哈希。

   func (e *Engine) AssertOccupancy(ctx context.Context, req AssertOccReq) (BroadcastOccupancy, error) {
	  if e == nil || e.store == nil {
		   return BroadcastOccupancy{}, ErrUnavailable
	   }
	  if strings.TrimSpace(req.TransferID) == "" {
		   return BroadcastOccupancy{}, ErrTransfer
	   }
	   if err := e.guardOpenOrphan(ctx, req.TransferID); err != nil {
		  e.metric.IncFailClosed()
		   return BroadcastOccupancy{}, err
	   }
	  latest, err := e.store.LatestGen(ctx, req.TransferID)
	  if err != nil {
		   if errors.Is(err, ErrGenNotFound) {
			   _ = e.recordProof(ctx, req.TransferID, req.GenID, "", "", ProofNoGen,
				  req.BroadcastHash, "", 0, 0, "no feebump_gen", req.Actor)
			  e.metric.IncFailClosed()
			   return BroadcastOccupancy{}, ErrNoGen
		  }
		   return BroadcastOccupancy{}, err
	   }
	  if req.GenID != "" && req.GenID != latest.GenID {
		  _ = e.recordProof(ctx,
			  req.TransferID, req.GenID, latest.GenID, latest.OccID, ProofStaleGen,
			  req.BroadcastHash, latest.NewBroadcastHash, 0, latest.Generation, "quoted gen is not latest", req.Actor)
		  e.metric.IncFailClosed()
		  return BroadcastOccupancy{}, ErrGenStale
	  }
	  occ,
		  err := e.store.GetOccByGen(ctx, latest.GenID)
	   if err != nil {
		   return BroadcastOccupancy{}, err
	   }
	  if !occ.State.Held() {
		   e.metric.IncFailClosed()
		   return BroadcastOccupancy{}, ErrOccNotHeld
	  }
	   if req.BroadcastHash != "" && !SameHash(req.BroadcastHash, occ.BroadcastHash) {
		  _ = e.recordProof(ctx, req.TransferID, latest.GenID, "", occ.OccID, ProofPrevHashMismatch,
			  req.BroadcastHash, occ.BroadcastHash, latest.Generation, latest.Generation,
			  "assert occupancy hash mismatch", req.Actor)
		   e.metric.IncFailClosed()
		   return BroadcastOccupancy{}, ErrPrevHash
	  }
	   e.metric.IncAssert()
	   return occ, nil
   }

// BroadcastReplace 仅最新 signed gen 可进入替换广播。

   func (e *Engine) BroadcastReplace(ctx context.Context, req BroadcastReq) (BroadcastResp, error) {
	   if e == nil || e.store == nil {
		   return BroadcastResp{},
			   ErrUnavailable
	  }
	  if strings.TrimSpace(req.TransferID) == "" {
		  return BroadcastResp{}, ErrTransfer
	   }
	   if strings.TrimSpace(req.GenID) == "" {
		   _ = e.recordProof(ctx, req.TransferID, "", "", "", ProofNoGen, req.BroadcastHash, "", 0, 0, "broadcast without gen", req.Actor)
		  e.metric.IncFailClosed()
		  return BroadcastResp{},
			   ErrNoGen
	  }
	  if err := e.guardOpenOrphan(ctx, req.TransferID); err != nil {
		  e.metric.IncFailClosed()
		   return BroadcastResp{}, err
	  }
	  gen, err := e.store.GetGen(ctx, req.GenID)
	  if err != nil {
		  return BroadcastResp{}, err
	  }
	  latest, err := e.store.LatestGen(ctx, req.TransferID)
	  if err != nil {
		  return BroadcastResp{}, err
	  }
	  if latest.GenID != gen.GenID {
		  p := e.recordProof(ctx, req.TransferID, gen.GenID, latest.GenID, gen.OccID, ProofOldBroadcast,
			   req.BroadcastHash, latest.NewBroadcastHash, gen.Generation, latest.Generation,
			  "old gen broadcast refused", req.Actor)
		   e.metric.IncFailClosed()
		   return BroadcastResp{Gen: gen, Proof: &p}, ErrOldBroadcast
	  }
	   if gen.State.Terminal() {
		   if gen.State == GenSuperseded {
			  return BroadcastResp{Gen: gen}, ErrGenSuperseded
		   }
		  if gen.State == GenBlocked {
			  return BroadcastResp{Gen: gen}, ErrGenBlocked
		   }
		   return BroadcastResp{Gen: gen}, ErrGenFinal
	   }
	   next, err := NextOnBroadcast(gen.State)
	   if err != nil {
		   e.metric.IncFailClosed()
		  return BroadcastResp{Gen: gen}, err
	   }
	   if req.BroadcastHash != "" && !SameHash(req.BroadcastHash, gen.NewBroadcastHash) {
		  p := e.recordProof(ctx, req.TransferID, gen.GenID, "", gen.OccID, ProofPrevHashMismatch,
			   req.BroadcastHash, gen.NewBroadcastHash, gen.Generation, latest.Generation,
			   "broadcast hash mismatch", req.Actor)
		  e.metric.IncFailClosed()
		  return BroadcastResp{Gen: gen,
			   Proof: &p}, ErrPrevHash
	  }
	  occ, err := e.store.GetOccByGen(ctx, gen.GenID)
	   if err != nil {
		  return BroadcastResp{}, err
	   }
	  if !occ.State.Held() {
		  e.metric.IncFailClosed()
		  return BroadcastResp{Gen: gen,
			  Occ: occ}, ErrOccNotHeld
	  }
	   now := e.now()
	  gen.State = next
	  gen.UpdatedAt = now
	   t := now
	  gen.BroadcastAt = &t
	   if err := e.store.CASGen(ctx, gen.GenID, GenSigned, GenBroadcast, gen); err != nil {
		  return BroadcastResp{}, err
	  }
	  _ = e.recordProof(ctx, req.TransferID, gen.GenID, "", occ.OccID, ProofBroadcastOK,
		  gen.NewBroadcastHash, gen.NewBroadcastHash, gen.Generation, gen.Generation, "broadcast accepted", req.Actor)
	   _ = e.store.AppendAudit(ctx, GenAudit{
		   AuditID: uuid.NewString(),
		   GenID:   gen.GenID, TransferID: gen.TransferID,
		   Action: "broadcast", FromState: string(GenSigned), ToState: string(GenBroadcast),
		   Actor: req.Actor, Detail: "replace broadcast accepted", At: now,
	   })
	  e.metric.IncBroadcast()
	  return BroadcastResp{Gen: gen,
		   Occ: occ}, nil
   }

// Finalize 广播确认后终态。
  func (e *Engine) Finalize(ctx context.Context, genID, actor string) (FeeBumpGen, error) {
	  if e == nil || e.store == nil {
		  return FeeBumpGen{},
			   ErrUnavailable
	  }
	  gen, err := e.store.GetGen(ctx, genID)
	  if err != nil {
		   return FeeBumpGen{}, err
	  }
	   if gen.State == GenFinal {
		   return gen, nil
	   }
	  if gen.State != GenBroadcast {
		   return gen, ErrInvalidTransition
	  }
	  now := e.now()
	   gen.State = GenFinal
	  gen.UpdatedAt = now
	   t := now
	   gen.FinalAt = &t
	  if err := e.store.CASGen(ctx, gen.GenID, GenBroadcast, GenFinal, gen); err != nil {
		   return FeeBumpGen{}, err
	  }
	   if occ, err := e.store.GetOccByGen(ctx, gen.GenID); err == nil && occ.State.Held() {
		  occ.State = OccRetired
		  occ.UpdatedAt = now
		   occ.RetiredAt = &t
		  _ = e.store.CASOcc(ctx, occ.OccID, OccHeld, OccRetired, occ)
	  }
	  _ = e.store.AppendAudit(ctx, GenAudit{
		   AuditID: uuid.NewString(),
		   GenID: gen.GenID, TransferID: gen.TransferID,
		  Action: "finalize", FromState: string(GenBroadcast), ToState: string(GenFinal),
		   Actor: actor, Detail: "confirmed on chain", At: now,
	  })
	   e.metric.IncFinal()
	   return gen, nil
   }

// ForceRetire 运维强制退役 held。
   func (e *Engine) ForceRetire(ctx context.Context, req RetireReq) (BroadcastOccupancy, error) {
	   if e == nil || e.store == nil {
		  return BroadcastOccupancy{}, ErrUnavailable
	  }
	  if strings.TrimSpace(req.Actor) == "" {
		  return BroadcastOccupancy{}, ErrActor
	  }
	  occ, err := e.lookupOcc(ctx, req)
	  if err != nil {
		  return BroadcastOccupancy{}, err
	  }
	  if occ.State == OccRetired {
		   return occ, nil
	  }
	   now := e.now()
	  from := occ.State
	   if from != OccHeld && from != OccOrphaned {
		   return occ, ErrOccNotHeld
	  }
	   // 孤儿占用只允许 force。
	  if from == OccOrphaned && !req.Force {
		   return occ, ErrOccOrphaned
	   }
	   occ.UpdatedAt = now
	  t := now
	  occ.RetiredAt = &t
	   if from == OccHeld {
		   if err := e.store.CASOcc(ctx, occ.OccID, OccHeld, OccRetired, occ); err != nil {
			  return BroadcastOccupancy{}, err
		   }
	   } else {
		   // 孤儿行保持 orphaned，另记证明；force 时允许关孤儿证明。
		  _ = e.closeOpenOrphans(ctx, occ.TransferID, req.Actor)
	   }
	  if gen, err := e.store.GetGen(ctx, occ.GenID); err == nil && gen.State.CanSupersede() {
		   prev := gen.State
		  gen.State = GenSuperseded
		  gen.UpdatedAt = now
		  gen.SupersededAt = &t
		  _ = e.store.CASGen(ctx, gen.GenID, prev, GenSuperseded, gen)
	   }
	   _ = e.recordProof(ctx, occ.TransferID, occ.GenID, "", occ.OccID, ProofForceRetire,
		   occ.BroadcastHash, "", occ.Generation, occ.Generation, req.Why, req.Actor)
	  e.metric.IncRetire()
	   occ.State = OccRetired
	   return occ, nil
  }

   func (e *Engine) lookupOcc(ctx context.Context, req RetireReq) (BroadcastOccupancy, error) {
	   if req.GenID != "" {
		   return e.store.GetOccByGen(ctx, req.GenID)
	   }
	   held, err := e.store.ListHeldByTransfer(ctx, req.TransferID)
	  if err != nil {
		  return BroadcastOccupancy{}, err
	  }
	   if len(held) == 0 {
		   return BroadcastOccupancy{},
			  ErrOccNotFound
	   }
	  return held[0], nil
  }

   func (e *Engine) retirePrevious(ctx context.Context, prev FeeBumpGen, actor string) error {
	  if prev.OccID == "" {
		   return nil
	   }
	  occ, err := e.store.GetOcc(ctx, prev.OccID)
	   if err != nil {
		   if errors.Is(err, ErrOccNotFound) {
			   return ErrRetireFailed
		   }
		   return err
	  }
	   if occ.State == OccRetired {
		  return e.supersedeGen(ctx, prev, actor)
	  }
	   if occ.State != OccHeld {
		  return ErrRetireFailed
	  }
	   now := e.now()
	   occ.UpdatedAt = now
	   t := now
	   occ.RetiredAt = &t
	   if err := e.store.CASOcc(ctx, occ.OccID, OccHeld, OccRetired, occ); err != nil {
		   return ErrRetireFailed
	  }
	   if err := e.supersedeGen(ctx, prev, actor); err != nil {
		  return err
	   }
	  _ = e.recordProof(ctx, prev.TransferID, prev.GenID, "", occ.OccID, ProofRetireOK,
		   prev.NewBroadcastHash, "", prev.Generation, prev.Generation, "previous occupancy retired", actor)
	  e.metric.IncRetire()
	   return nil
  }

  func (e *Engine) supersedeGen(ctx context.Context, prev FeeBumpGen, actor string) error {
	   if !prev.State.CanSupersede() {
		   return nil
	  }
	   now := e.now()
	   from := prev.State
	  prev.State = GenSuperseded
	  prev.UpdatedAt = now
	   t := now
	   prev.SupersededAt = &t
	  if err := e.store.CASGen(ctx, prev.GenID, from, GenSuperseded, prev); err != nil {
		   return ErrRetireFailed
	  }
	   _ = e.store.AppendAudit(ctx, GenAudit{
		  AuditID: uuid.NewString(), GenID: prev.GenID, TransferID: prev.TransferID,
		   Action: "supersede", FromState: string(from), ToState: string(GenSuperseded),
		  Actor: actor, Detail: "retired by newer feebump_gen", At: now,
	  })
	   return nil
  }

  func (e *Engine) blockGen(ctx context.Context, gen FeeBumpGen, why string) error {
	  if !AllowGenTransition(gen.State, GenBlocked) && gen.State != GenSigned && gen.State != GenBroadcast {
		   return nil
	   }
	  from := gen.State
	  now := e.now()
	  gen.State = GenBlocked
	   gen.UpdatedAt = now
	   t := now
	  gen.BlockedAt = &t
	  if err := e.store.CASGen(ctx, gen.GenID, from, GenBlocked, gen); err != nil {
		   return err
	  }
	  _ = e.store.AppendAudit(ctx, GenAudit{
		  AuditID: uuid.NewString(), GenID: gen.GenID, TransferID: gen.TransferID,
		   Action: "block", FromState: string(from), ToState: string(GenBlocked),
		  Actor: "system", Detail: why, At: now,
	   })
	   return nil
  }

   func (e *Engine) sign(canon, sigHex string) ([]byte, error) {
	   if strings.TrimSpace(sigHex) != "" {
		   b, err := hex.DecodeString(strings.TrimSpace(sigHex))
		  if err != nil || len(b) == 0 {
			   return nil, ErrSigner
		  }
		   return b, nil
	   }
	  if e.signer != nil {
		  return e.signer.SignCanonical(canon)
	   }
	  // 无适配器时用规范串摘要作材料占位，与 ops 预演；必须注入 Signer。
	  sum := []byte(CanonDigest(canon))
	  return sum, nil
   }

   func validateSign(req SignWithGenReq) error {
	   if strings.TrimSpace(req.TransferID) == "" {
		   return ErrTransfer
	   }
	   if strings.TrimSpace(req.Chain) == "" {
		  return ErrChain
	   }
	   if strings.TrimSpace(req.To) == "" {
		  return ErrDest
	  }
	  if strings.TrimSpace(req.Amount) == "" {
		  return ErrAmount
	   }
	  if strings.TrimSpace(req.NewFee) == "" {
		  return ErrFee
	   }
	  if _, ok := FeeUnits(req.NewFee); !ok {
		  return ErrFee
	   }
	  if strings.TrimSpace(req.Actor) == "" {
		   return ErrActor
	  }
	  if strings.TrimSpace(req.Asset) == "" {
		  return ErrAsset
	  }
	   return nil
   }
