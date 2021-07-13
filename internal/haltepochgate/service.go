// 功能：Halt epoch 滑窗闸服务：EngageWithEpoch / ClearWithEpoch / LaneAssert。
// 不改写 custodyhalt 核心 FSM、haltclear、r10 误清、r13 禁提。

// 作者：Hale
// 日期：2021.07.12

package haltepochgate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Service 熔断世代与滑窗封口。

type Service struct {
	store Store
	clock func() time.Time
	gate GateReader
	lane LaneReader
	hist HistoryReader
	audit AuditReader
	quorum QuorumReader
	mu     sync.Mutex
	scopes map[string]*sync.Mutex
}

  func New() *Service {
	   return NewWithStore(NewMemStore())
  }

  func NewWithStore(store Store) *Service {
	  return &Service{
		  store: store,
		   clock: func() time.Time { return time.Now().UTC() },
		   gate:  NopGate{},
		   lane:  NopLane{},
		  hist:  NopHistory{},
		   audit: NopAudit{},
		  quorum: NewLocalQuorum(DefaultQuorumThreshold),
		   scopes: map[string]*sync.Mutex{},
	   }
  }

   func (s *Service) WithClock(c func() time.Time) *Service {
	   if c != nil {
		  s.clock = c
	   }
	  return s
   }

   func (s *Service) WithHooks(g GateReader, l LaneReader, h HistoryReader, a AuditReader, q QuorumReader) *Service {
	   if g != nil {
		   s.gate = g
	  }
	  if l != nil {
		  s.lane = l
	  }
	  if h != nil {
		   s.hist = h
	   }
	   if a != nil {
		  s.audit = a
	  }
	   if q != nil {
		   s.quorum = q
	   }
	   return s
  }

  func (s *Service) now() time.Time { return s.clock().UTC() }

  func (s *Service) scopeLock(key string) *sync.Mutex {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if s.scopes == nil {
		  s.scopes = map[string]*sync.Mutex{}
	  }
	  lk, ok := s.scopes[key]
	  if !ok {
		  lk = &sync.Mutex{}
		  s.scopes[key] = lk
	   }
	  return lk
  }

  func (s *Service) loadOrInit(ctx context.Context, scope ScopeKey) (*HaltEpoch, error) {
	   cur, err := s.store.GetEpoch(ctx, scope)
	   if err == nil {
		  return cur, nil
	   }
	   if !errors.Is(err, ErrEpochNotFound) {
		   return nil, err
	  }
	  now := s.now()
	  init := HaltEpoch{
		   EpochID: "he-" + uuid.NewString(),
		   Scope:      scope,
		   Generation: 0,
		  State:      EpochIdle,
		  LaneOpen:   true,
		   CreatedAt:  now,
		  UpdatedAt: now,
	   }
	   if err := s.store.PutEpoch(ctx, init); err != nil {
		  return nil, err
	   }
	   return &init, nil
   }

   func (s *Service) writeAudit(ctx context.Context, kind, actor, detail string, gen int64, scope ScopeKey) {
	  _ = s.store.AppendAudit(ctx, AuditEvent{
		  EventID: "ae-" + uuid.NewString(),
		   Kind:    kind,
		   Scope:   scope.String(),
		   Actor: actor,
		   Detail:     detail,
		   At:         s.now(),
		   Generation: gen,
	   })
  }

// CurrentEpoch 对外只读 halt_epoch。

  func (s *Service) CurrentEpoch(ctx context.Context, scope, chain, asset string) (*HaltEpoch, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrStoreUnavailable
	  }
	   ep, err := s.store.GetEpoch(ctx, NormalizeScope(scope, chain, asset))
	  if err != nil {
		   return nil, err
	  }
	  return ep, nil
  }

// EngageWithEpoch 置停并抬升 halt_epoch。

  func (s *Service) EngageWithEpoch(ctx context.Context, req EngageRequest) (*HaltEpoch, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrStoreUnavailable
	  }
	  actor := strings.TrimSpace(req.Actor)
	  reason := strings.TrimSpace(req.Reason)
	   if actor == "" {
		  return nil, ErrActorRequired
	  }
	   if reason == "" {
		   return nil, ErrReasonRequired
	  }
	  scope := NormalizeScope(req.Scope, req.Chain, req.Asset)
	   lk := s.scopeLock(scope.String())
	  lk.Lock()
	  defer lk.Unlock()
	   cur, err := s.loadOrInit(ctx, scope)
	   if err != nil {
		   return nil, err
	  }
	  if req.IdemKey != "" && cur.IdemKey == req.IdemKey && (cur.State == EpochEngaged || cur.State == EpochReengaged) {
		  return cur, nil
	   }
	  if err := CompatibleMutex(cur.MutexHolder, cur.MutexKind, actor, MutexKindOf(true)); err != nil {
		  _ = s.openSlip(ctx, *cur, SlipEngageClearRace, "", 0, actor, err.Error(), "")
		   return nil, err
	   }
	   st, gen, err := NextOnEngage(cur)
	  if err != nil {
		  _ = s.openSlip(ctx, *cur, SlipEngageClearRace, "", cur.Generation, actor, err.Error(), "")
		  return nil, err
	  }
	   if err := CanAdvance(cur.Generation, gen, st == cur.State && gen == cur.Generation); err != nil {
		   return nil, err
	  }
	   now := s.now()
	  next := *cur
	   next.PrevGen = cur.Generation
	   next.Generation = gen
	  next.State = st
	  next.LaneOpen = false
	  next.MutexHolder = ""
	  next.MutexKind = ""
	  next.Actor = actor
	  next.Reason = reason
	  next.IdemKey = strings.TrimSpace(req.IdemKey)
	   next.EngagedAt = now
	   next.UpdatedAt = now
	   if err := s.store.PutEpoch(ctx, next); err != nil {
		  return nil, err
	   }
	  s.writeAudit(ctx, "engage", actor, reason, gen, scope)
	   return &next, nil
  }

// ClearWithEpoch 经 quorum 解除并抬升世代，打开清窗案。桥路径禁止。
  func (s *Service) ClearWithEpoch(ctx context.Context, req ClearRequest) (*HaltEpoch, *ClearWindowCase, error) {
	  if s == nil || s.store == nil {
		  return nil, nil, ErrStoreUnavailable
	  }
	  if req.FromBridge {
		  scope := NormalizeScope(req.Scope,
			   req.Chain, req.Asset)
		   cur, _ := s.store.GetEpoch(ctx, scope)
		   if cur == nil {
			   cur = &HaltEpoch{Scope: scope}
		  }
		   _ = s.openSlip(ctx, *cur, SlipBridgeClearAttempt, "", 0, req.Actor, ErrBridgeClear.Error(), "")
		  return nil, nil, ErrBridgeClear
	  }
	   actor := strings.TrimSpace(req.Actor)
	   }
	   if req.IdemKey != "" && cur.IdemKey == req.IdemKey && (cur.State == EpochClearing || cur.State == EpochCleared) {
		  var cs *ClearWindowCase
		  if cur.WindowCase != "" {
			   cs, _ = s.store.GetCase(ctx, cur.WindowCase)
		   }
		  return cur, cs, nil
	   }
	  if err := CompatibleMutex(cur.MutexHolder, cur.MutexKind, actor, MutexKindOf(false)); err != nil {
		  _ = s.openSlip(ctx, *cur, SlipEngageClearRace, "", cur.Generation, actor, err.Error(), "")
		  return nil, nil, err
	   }
	   st, gen, err := NextOnClear(cur)
	  if err != nil {
		  return nil, nil, err
	   }
	   if st == EpochClearing && cur.State != EpochClearing {
		   if err := CanAdvance(cur.Generation, gen, false); err != nil {
			  return nil, nil, err
		  }
	   }
	   bundle, err := s.requireQuorum(ctx, cur, actor, req.Force)
	  if err != nil {
		  return nil, nil, err
	   }
	  now := s.now()
	   cs := ClearWindowCase{
		  CaseID:  "cw-" + uuid.NewString(),
		  EpochID: cur.EpochID,
		  Scope:   scope,
		  ClearGen: gen,
		   PrevGen:  cur.Generation,
		  State:    WindowOpen,
		  Actor: actor,
		  Reason:   reason,
		  OpenedAt: now,
	  }
	   if err := s.store.PutCase(ctx, cs); err != nil {
		  return nil, nil, err
	  }
	  next := *cur
	   next.PrevGen = cur.Generation
	   next.Generation = gen
	  next.State = st
	  next.LaneOpen = false
	  next.MutexHolder = actor
	   next.MutexKind = MutexKindOf(false)
	  next.Actor = actor
	  next.Reason = reason
	  next.QuorumID = bundle.QuorumID
	  next.IdemKey = strings.TrimSpace(req.IdemKey)
	   next.WindowCase = cs.CaseID
	   next.ClearedAt = now
	   next.UpdatedAt = now
	  if err := s.store.PutEpoch(ctx, next); err != nil {
		  return nil, nil, err
	  }
	   revoked, err := s.revokeInflight(ctx, scope, gen, cs.CaseID, actor)
	  if err != nil {
		   return nil, nil, err
	   }
	  cs.RevokedIntents = revoked
	  cs.State = WindowSealing
	   _ = s.store.PutCase(ctx, cs)
	   sealed, err := NextOnSeal(&next)
	  if err != nil {
		   return nil, nil, err
	   }
	  next.State = sealed
	   next.MutexHolder = ""
	   next.MutexKind = ""
	   next.UpdatedAt = s.now()
	   if err := s.store.PutEpoch(ctx, next); err != nil {
		   return nil, nil, err
	  }
	  s.writeAudit(ctx, "clear", actor, reason, gen, scope)
	  return &next, &cs, nil
  }

// SealResume 封窗完成后恢复 idle 放行（仍要求新意图绑定当前 epoch）。

   func (s *Service) SealResume(ctx context.Context, scope, chain, asset, actor string) (*HaltEpoch, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrStoreUnavailable
	  }
	  actor = strings.TrimSpace(actor)
	  if actor == "" {
		  return nil, ErrActorRequired
	   }
	  sk := NormalizeScope(scope, chain, asset)
	  lk := s.scopeLock(sk.String())
	  lk.Lock()
	  defer lk.Unlock()
	   cur, err := s.store.GetEpoch(ctx, sk)
	  if err != nil {
		  return nil, err
	  }
	   st, err := NextOnResume(cur)
	   if err != nil {
		  return nil, err
	   }
	   if cur.WindowCase != "" {
		  cs, err := s.store.GetCase(ctx, cur.WindowCase)
		   if err == nil {
			   cs.State = WindowSealed
			  cs.SealedAt = s.now()
			  _ = s.store.PutCase(ctx, *cs)
		  }
	  }
	   next := *cur
	   next.State = st
	  next.LaneOpen = true
	  next.MutexHolder = ""
	   next.MutexKind = ""
	  next.Actor = actor
	   next.UpdatedAt = s.now()
	   if err := s.store.PutEpoch(ctx, next); err != nil {
		  return nil, err
	   }
	  s.writeAudit(ctx, "seal_resume", actor, "window sealed", next.Generation, sk)
	  return &next, nil
   }

// LaneAssert 充/提/清扫入口绑定当前 epoch。Clear 窗内拒并落 SlipProof。
   func (s *Service) LaneAssert(ctx context.Context, req LaneAssertRequest) (*FundIntent, *LaneConclusion, error) {
	   if s == nil || s.store == nil {
		   return nil,
			   nil, ErrStoreUnavailable
	  }
	  if !req.Lane.Valid() {
		  return nil, nil, ErrLaneUnknown
	   }
	  scope := NormalizeScope(req.Scope, req.Chain, req.Asset)
	  if req.Asset != "" && scope.Asset == "" {
		   scope.Asset = strings.ToUpper(strings.TrimSpace(req.Asset))
	   }
	   if err := s.readGate(ctx); err != nil {
		  ep, _ := s.store.GetEpoch(ctx, scope)
		   if ep == nil {
			   ep = &HaltEpoch{Scope: scope}
		  }
		   p := s.openSlip(ctx, *ep, SlipReadGateFail, req.Lane, req.CarriedGen, "", err.Error(), req.IntentID)
		   conc := LaneConclusion{Scope: scope.String(), Lane: string(req.Lane), Blocked: true, FailClosed: true, Reason: err.Error()}
		  if p != nil {
			  conc.ProofID = p.ProofID
		   }
		  return nil, &conc, err
	  }
	   cur, err := s.loadOrInit(ctx, scope)
	  if err != nil {
		  return nil, nil, err
	  }
	   conc := cur.LaneConclusion()
	   conc.Lane = string(req.Lane)
	   if req.IdemKey != "" {
		  if exist, e2 := s.store.GetIntentByIdem(ctx, req.IdemKey); e2 == nil && exist != nil {
			   c := conc
			   return exist, &c, nil
		   }
	   }
	  if req.CarriedGen <= 0 {
		   p := s.openSlip(ctx, *cur, SlipMissingEpoch, req.Lane, 0, "", ErrEpochMissing.Error(), req.IntentID)
		   conc.Blocked = true
		  conc.LaneOpen = false
		   conc.Reason = ErrEpochMissing.Error()
		   if p != nil {
			  conc.ProofID = p.ProofID
		   }
		  return nil, &conc, ErrEpochMissing
	   }
	   if err := cur.BindOK(req.CarriedGen); err != nil {
		   kind := SlipKindForLane(req.Lane)
		  if errors.Is(err, ErrEpochStale) {
			  kind = SlipLaneMismatch
		   }
		  p := s.openSlip(ctx, *cur, kind, req.Lane, req.CarriedGen, "", err.Error(), req.IntentID)
		  conc.Blocked = true
		  conc.LaneOpen = false
		  conc.Reason = err.Error()
		  if p != nil {
			   conc.ProofID = p.ProofID
		  }
		   return nil, &conc, err
	   }
	  if cur.State.WindowSealed() || cur.State.Halted() || !cur.LaneOpen {
		  kind := SlipKindForLane(req.Lane)
		   p := s.openSlip(ctx, *cur, kind, req.Lane, req.CarriedGen, "", ErrLaneSealed.Error(), req.IntentID)
		   conc.Blocked = true
		   conc.LaneOpen = false
		  conc.Reason = ErrLaneSealed.Error()
		   if p != nil {
			   conc.ProofID = p.ProofID
		  }
		   return nil, &conc, ErrLaneSealed
	   }
	  now := s.now()
	  in := FundIntent{
		   IntentID: "fi-" + uuid.NewString(),
		   Scope:      scope,
		  Lane:       req.Lane,
		   Generation: cur.Generation,
		   State:      IntentAllowed,
		  Account:    strings.TrimSpace(req.Account),
		  Asset: strings.ToUpper(strings.TrimSpace(req.Asset)),
		  Amount:    strings.TrimSpace(req.Amount),
		  IdemKey:   strings.TrimSpace(req.IdemKey),
		   AllowTick: strings.TrimSpace(req.AllowTick),
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if req.IntentID != "" {
		   in.IntentID = req.IntentID
	  }
	   if err := s.store.PutIntent(ctx, in); err != nil {
		   return nil, &conc, err
	   }
	  return &in, &conc, nil
  }

// AssertBroadcast 广播前复验：世代已升则拒并落 SlipProof。
  func (s *Service) AssertBroadcast(ctx context.Context, req BroadcastRequest) (*FundIntent, error) {
	   in, ep, err := s.loadIntentEpoch(ctx, req.IntentID)
	   if err != nil {
		   return nil, err
	  }
	   if in.State == IntentRevoked || in.State == IntentRejected {
		  return in, ErrIntentRevoked
	  }
	  if req.CarriedGen <= 0 {
		  p := s.openSlip(ctx,
			   *ep, SlipMissingEpoch, in.Lane, 0, req.Actor, ErrEpochMissing.Error(), in.IntentID)
		  in.State = IntentRejected
		   if p != nil {
			   in.ProofID = p.ProofID
		   }
		  in.UpdatedAt = s.now()
		  _ = s.store.PutIntent(ctx, *in)
		  return in, ErrEpochMissing
	  }
	   if in.StaleVersus(ep.Generation) || req.CarriedGen != ep.Generation {
		  p := s.openSlip(ctx,
			  *ep, SlipStaleEpochBroadcast, in.Lane, req.CarriedGen, req.Actor, ErrBroadcastDenied.Error(), in.IntentID)
		  in.State = IntentRevoked
		   if p != nil {
			   in.ProofID = p.ProofID
		   }
		  in.UpdatedAt = s.now()
		  _ = s.store.PutIntent(ctx, *in)
		  return in, ErrBroadcastDenied
	   }
	   if ep.State.WindowSealed() || ep.State.Halted() {
		   p := s.openSlip(ctx,
			   *ep, SlipStaleEpochBroadcast, in.Lane, req.CarriedGen, req.Actor, ErrLaneSealed.Error(), in.IntentID)
		  in.State = IntentRevoked
		  if p != nil {
			  in.ProofID = p.ProofID
		   }
		   in.UpdatedAt = s.now()
		  _ = s.store.PutIntent(ctx, *in)
		   return in, ErrBroadcastDenied
	   }
	  if err := TransitionIntent(in.State, IntentBroadcast); err != nil {
		  return in, err
	   }
	   in.State = IntentBroadcast
	  in.UpdatedAt = s.now()
	   if err := s.store.PutIntent(ctx, *in); err != nil {
		   return nil, err
	   }
	  return in, nil
   }

// AssertCredit 贷记前复验。
  func (s *Service) AssertCredit(ctx context.Context, req CreditRequest) (*FundIntent, error) {
	   in, ep, err := s.loadIntentEpoch(ctx, req.IntentID)
	   if err != nil {
		   return nil, err
	   }
	  if in.State == IntentRevoked || in.State == IntentRejected {
		  return in, ErrCreditDenied
	   }
	   if req.CarriedGen != ep.Generation || in.StaleVersus(ep.Generation) {
		   p := s.openSlip(ctx, *ep, SlipStaleEpochCredit, in.Lane, req.CarriedGen, req.Actor, ErrCreditDenied.Error(), in.IntentID)
		   in.State = IntentRevoked
		   if p != nil {
			   in.ProofID = p.ProofID
		   }
		  in.UpdatedAt = s.now()
		  _ = s.store.PutIntent(ctx, *in)
		  return in, ErrCreditDenied
	  }
	  if ep.State.WindowSealed() || ep.State.Halted() {
		  p := s.openSlip(ctx, *ep, SlipStaleEpochCredit, in.Lane, req.CarriedGen, req.Actor, ErrLaneSealed.Error(), in.IntentID)
		   in.State = IntentRevoked
		   if p != nil {
			   in.ProofID = p.ProofID
		   }
		  in.UpdatedAt = s.now()
		   _ = s.store.PutIntent(ctx, *in)
		   return in, ErrCreditDenied
	  }
	   if err := TransitionIntent(in.State, IntentCredited); err != nil {
		  return in, err
	   }
	   in.State = IntentCredited
	  in.UpdatedAt = s.now()
	   if err := s.store.PutIntent(ctx, *in); err != nil {
		   return nil, err
	  }
	  return in, nil
   }

   func (s *Service) loadIntentEpoch(ctx context.Context, id string) (*FundIntent, *HaltEpoch, error) {
	  if strings.TrimSpace(id) == "" {
		   return nil, nil, ErrIntentNotFound
	  }
	  in, err := s.store.GetIntent(ctx, id)
	  if err != nil {
		  return nil, nil, err
	  }
	  ep, err := s.store.GetEpoch(ctx, in.Scope)
	  if err != nil {
		   return nil, nil, err
	   }
	   return in, ep, nil
   }

   func (s *Service) readGate(ctx context.Context) error {
	  if s.gate == nil {
		  return ErrReadGateFailed
	   }
	   if _, err := s.gate.Snapshot(ctx); err != nil {
		   return errors.Join(ErrReadGateFailed, err)
	   }
	  return nil
   }

   func (s *Service) LaneOf(ctx context.Context, lane FundLane, scope, chain, asset string) LaneConclusion {
	   sk := NormalizeScope(scope, chain, asset)
	   ep, err := s.store.GetEpoch(ctx, sk)
	  if err != nil {
		  return LaneConclusion{Scope: sk.String(), Lane: string(lane), Blocked: true, FailClosed: errors.Is(err, ErrStoreUnavailable) || !errors.Is(err, ErrEpochNotFound), Reason: err.Error()}
	  }
	   c := ep.LaneConclusion()
	   c.Lane = string(lane)
	   if s.lane != nil {
		   if err := s.lane.CheckLane(ctx, MapLane(lane), chain, asset); err != nil {
			  c.Blocked = true
			   c.LaneOpen = false
			   c.Reason = err.Error()
		   }
	  }
	  return c
   }

// SeedAllowed /巡检：在当前世代写入已放行意图（Engage 前窗口）。
  func (s *Service) SeedAllowed(ctx context.Context, in FundIntent) (*FundIntent, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrStoreUnavailable
	   }
	  if in.IntentID == "" {
		   in.IntentID = "fi-" + uuid.NewString()
	  }
	   if in.State == "" {
		  in.State = IntentAllowed
	   }
	  now := s.now()
	  if in.CreatedAt.IsZero() {
		  in.CreatedAt = now
	   }
	  in.UpdatedAt = now
	  if err := s.store.PutIntent(ctx, in); err != nil {
		  return nil, err
	   }
	   return &in, nil
   }

  func (s *Service) GetProof(ctx context.Context, id string) (*SlipProof, error) {
	  return s.store.GetProof(ctx, id)
   }

  func (s *Service) ListProofs(ctx context.Context, state SlipState, limit int) ([]SlipProof, error) {
	  return s.store.ListProofs(ctx, state, limit)
  }

  func (s *Service) ListEpochs(ctx context.Context, limit int) ([]HaltEpoch, error) {
	  return s.store.ListEpochs(ctx, limit)
  }

   func (s *Service) GetCase(ctx context.Context, id string) (*ClearWindowCase, error) {
	   return s.store.GetCase(ctx, id)
   }

   func (s *Service) RecentAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	   return s.store.ListAudit(ctx, limit)
   }

   func (s *Service) HaltHistory(limit int) []string {
	   if s.hist == nil {
		  return nil
	   }
	   ev := s.hist.Recent(limit)
	  out := make([]string, 0, len(ev))
	   for _, e := range ev {
		   out = append(out, fmt.Sprintf("%s:%s:%s", e.Action, e.Actor, e.Scope))
	   }
	   return out
  }
