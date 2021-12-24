package wdcomp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/wdrollback"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Orchestrator 出金失败补偿的进程内编排器。
type Orchestrator struct {
	mu sync.Mutex
	svc *wdrollback.Service
	fail       map[string]*WdFailure
	byIdem     map[string]string
	plans      map[string]*CompensatePlan
	planByFail map[string]string
	legs map[string][]*CompensateLeg
	incidents []Incident
	inbox     []FailureFact
	policies  *PolicyBook
	clock func() time.Time
	seq   uint64
	sql       *mirror
	mirrorErr string
	shardSrc  ShardFailureSource
}

// New 带默认策略表和空回滚账。

   func New() *Orchestrator {
	  return &Orchestrator{
		   svc: wdrollback.New(),
		  fail: map[string]*WdFailure{},
		  byIdem: map[string]string{},
		   plans:      map[string]*CompensatePlan{},
		   planByFail: map[string]string{},
		  legs: map[string][]*CompensateLeg{},
		   policies: DefaultPolicies(),
	  }
   }

  func (o *Orchestrator) now() time.Time {
	   if o != nil && o.clock != nil {
		  return o.clock().UTC()
	  }
	   return time.Now().UTC()
  }

// SetClock 注入时钟。空函数恢复为当前时间。
   func (o *Orchestrator) SetClock(fn func() time.Time) {
	   if o == nil {
		  return
	  }
	  o.mu.Lock()
	   o.clock = fn
	  o.mu.Unlock()
   }

  func (o *Orchestrator) next(prefix string) string {
	   o.seq++
	  return fmt.Sprintf("%s-%d", prefix, o.seq)
   }

// Enqueue 把事实放进摄取队列，由工人 Tick 消化。
   func (o *Orchestrator) Enqueue(fact FailureFact) {
	  if o == nil {
		  return
	  }
	   o.mu.Lock()
	  o.inbox = append(o.inbox, fact)
	   o.mu.Unlock()
  }

// Ingest 同步分类并落失败单。已存在的同指纹键直接返回，不动余额。

  func (o *Orchestrator) Ingest(ctx context.Context, fact FailureFact) (IngestResult, error) {
	   if err := ctx.Err(); err != nil {
		  return IngestResult{}, err
	  }
	   if o == nil {
		   return IngestResult{}, ErrUnavailable
	   }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	  return o.ingest(ctx, fact)
  }

   func (o *Orchestrator) ingest(ctx context.Context, fact FailureFact) (IngestResult, error) {
	  f, amts, err := normalize(fact)
	  if err != nil {
		   return IngestResult{}, err
	  }
	  dec, err := classify(f, amts, o.policies)
	   if err != nil {
		  return IngestResult{}, err
	  }
	   fp := fingerprintOf(f)
	   if id, ok := o.byIdem[f.IdempotencyKey]; ok {
		   old := o.fail[id]
		  if old == nil {
			   return IngestResult{}, ErrNotFound
		   }
		   if old.Fingerprint != fp {
			  return IngestResult{}, ErrIdemConflict
		   }
		   noteReplay(dec.Code)
		  return IngestResult{Failure: *old, Replayed: true}, nil
	   }
	   if err := o.seed(f); err != nil {
		   return IngestResult{}, err
	   }
	   prot, err := parseOpt(dec.Protected)
	  if err != nil {
		  return IngestResult{}, err
	   }
	  if err := o.svc.Protect(f.TransferID, f.UserID, f.Asset, prot); err != nil {
		   return IngestResult{}, err
	  }
	  now := o.now()
	   if f.OccurredAt.IsZero() {
		  f.OccurredAt = now
	   }
	   row := &WdFailure{
		   ID: o.next("wdf"), IdempotencyKey: f.IdempotencyKey, Fingerprint: fp,
		   TransferID: f.TransferID, UserID: f.UserID, Asset: f.Asset, Chain: f.Chain,
		   Source: f.Source, Code: dec.Code, ReasonCode: dec.ReasonCode, Reason: strings.TrimSpace(f.Reason),
		  UserText: dec.UserText, TxHash: f.TxHash, FilledTxNote: f.FilledTxNote,
		   LiveHash: dec.Live, ChainBlocked: dec.Blocked, BroadcastStatus: f.BroadcastStatus,
		   FreezeState: f.FreezeState, ChainState: f.ChainState, ShardPlanID: f.ShardPlanID,
		   ShardID: f.ShardID, ShardSeq: f.ShardSeq, Amount: f.Amount, FilledAmount: f.FilledAmount,
		  FailedAmount: f.FailedAmount, InFlightAmount: f.InFlightAmount, FeeReserved: f.FeeReserved,
		   Occupancy: f.Occupancy, ProtectedAmount: dec.Protected, Compensable: dec.Compensable,
		  AvailableSnapshot: f.AvailableSnapshot, FrozenSnapshot: f.FrozenSnapshot, FeeSnapshot: f.FeeSnapshot,
		  Status: FailOpen,
		   MayAuto: dec.MayAuto, OccurredAt: f.OccurredAt.UTC(), IngestedAt: now, UpdatedAt: now,
	  }
	  o.fail[row.ID] = row
	  o.byIdem[row.IdempotencyKey] = row.ID
	  o.mirrorFailure(ctx, row)
	   noteIngest(dec.Code, false)
	   return IngestResult{Failure: *row}, nil
  }

   func (o *Orchestrator) seed(f FailureFact) error {
	   if strings.TrimSpace(f.AvailableSnapshot) == "" && strings.TrimSpace(f.FrozenSnapshot) == "" && strings.TrimSpace(f.FeeSnapshot) == "" {
		  if _, err := o.svc.Position(f.UserID, f.Asset); err == nil {
			  return nil
		  }
		  return ErrPosition
	   }
	  feeBook := f.FeeSnapshot
	   if strings.TrimSpace(feeBook) == "" {
		   feeBook = f.FeeReserved
	   }
	  occBook := f.OccupancySnapshot
	   if strings.TrimSpace(occBook) == "" {
		  occBook = f.Occupancy
	   }
	  return o.svc.SeedIfAbsent(f.UserID, f.Asset, f.AvailableSnapshot, f.FrozenSnapshot, feeBook, occBook)
  }

// Preview 生成或返回计划，不执行资金腿。
  func (o *Orchestrator) Preview(ctx context.Context, failureID string) (PlanView, error) {
	   if err := ctx.Err(); err != nil {
		  return PlanView{}, err
	   }
	   if o == nil {
		  return PlanView{}, ErrUnavailable
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   return o.build(ctx, strings.TrimSpace(failureID), o.now())
   }

// Execute 执行计划。热态返回 ErrNeedsHuman，已提交的计划原样返回。
   func (o *Orchestrator) Execute(ctx context.Context, planID string) (PlanView, error) {
	   if err := ctx.Err(); err != nil {
		   return PlanView{}, err
	   }
	   if o == nil {
		  return PlanView{}, ErrUnavailable
	   }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	  return o.execute(ctx, strings.TrimSpace(planID), o.now())
  }

   func (o *Orchestrator) build(ctx context.Context, failureID string, now time.Time) (PlanView, error) {
	  f := o.fail[failureID]
	  if f == nil {
		  return PlanView{}, ErrNotFound
	  }
	   if id := o.planByFail[failureID]; id != "" {
		   return o.view(id), nil
	   }
	  pol, ok := o.policies.Get(f.Code)
	   if !ok {
		  return PlanView{}, ErrPolicy
	  }
	  dec := decisionFromFailure(f, pol)
	   p := &CompensatePlan{
		  ID: o.next("wdp"), FailureID: f.ID, TransferID: f.TransferID, UserID: f.UserID, Asset: f.Asset,
		  State: PlanOpen, AutoOK: dec.MayAuto, ForceHuman: dec.ForceHuman, LiveHash: dec.Live,
		   ChainBlocked: dec.Blocked, PolicyCode: string(f.Code), PolicyVersion: pol.Version,
		  IdempotencyKey: f.IdempotencyKey, TimeoutAt: now.Add(time.Duration(dec.Timeout) * time.Second),
		  CreatedAt: now, UpdatedAt: now,
	   }
	  o.plans[p.ID] = p
	  o.planByFail[f.ID] = p.ID
	  f.PlanID = p.ID
	  if err := o.transit(p, PlanPlanning, now); err != nil {
		  return PlanView{}, err
	  }
	  o.materialize(p, f, dec, pol.Legs, now)
	  if !dec.MayAuto {
		   if err := o.enterHuman(ctx, p, f, "auto_plan", now); err != nil {
			   return PlanView{}, err
		   }
		  o.mirrorPlan(ctx, p)
		   o.mirrorFailure(ctx, f)
		   notePlan(string(p.State))
		   return o.view(p.ID), nil
	  }
	   f.Status = FailPlanned
	   f.UpdatedAt = now
	   o.mirrorPlan(ctx, p)
	   o.mirrorLegs(ctx, p.ID)
	   o.mirrorFailure(ctx, f)
	   notePlan(string(p.State))
	   return o.view(p.ID), nil
  }

   func decisionFromFailure(f *WdFailure, pol AutoPolicy) Decision {
	  d := Decision{
		   Code: f.Code, ReasonCode: f.ReasonCode, UserText: f.UserText,
		  MayAuto: f.MayAuto, ForceHuman: !f.MayAuto, Live: f.LiveHash, Blocked: f.ChainBlocked,
		   Compensable: f.Compensable, Filled: f.FilledAmount, InFlight: f.InFlightAmount,
		  Protected: f.ProtectedAmount,
		   Fee: f.FeeReserved, OccRelease: occReleaseOf(f),
		   PolicyVersion: pol.Version, Timeout: pol.TimeoutSec, Dual: pol.DualConfirm || f.LiveHash || f.ChainBlocked,
	  }
	   if d.Timeout <= 0 {
		  d.Timeout = 900
	   }
	  return d
  }

   func occReleaseOf(f *WdFailure) string {
	   comp, err1 := parseOpt(f.Compensable)
	   occ, err2 := parseOpt(f.Occupancy)
	  if err1 != nil || err2 != nil || !comp.IsPositive() || !occ.IsPositive() {
		  return "0"
	  }
	   return minAmt(occ, comp).String()
   }

  func (o *Orchestrator) materialize(p *CompensatePlan, f *WdFailure, dec Decision, template []string, now time.Time) {
	   specs := assembleLegs(dec, template)
	   legs := make([]*CompensateLeg, 0, len(specs))
	  for i, sp := range specs {
		  legs = append(legs, &CompensateLeg{
			   ID: o.next("leg"), PlanID: p.ID, Seq: i + 1, Kind: sp.Kind, Status: LegPending,
			   Amount: sp.Amount,
			   Asset: f.Asset, UserID: f.UserID, TransferID: f.TransferID,
			  IdempotencyKey: fmt.Sprintf("%s:%d:%s", p.ID, i+1, sp.Kind),
			   ReasonCode: f.ReasonCode, CreatedAt: now, UpdatedAt: now,
		  })
	  }
	  o.legs[p.ID] = legs
   }

  func (o *Orchestrator) execute(ctx context.Context, planID string, now time.Time) (PlanView, error) {
	  p := o.plans[planID]
	   if p == nil {
		  return PlanView{}, ErrNotFound
	   }
	   f := o.fail[p.FailureID]
	  if f == nil {
		   return PlanView{}, ErrNotFound
	  }
	   if p.State == PlanCommitted {
		  return o.view(planID), nil
	   }
	   if p.State == PlanAborted {
		   return o.view(planID), ErrState
	  }
	  hot := p.LiveHash || p.ChainBlocked || f.LiveHash || f.ChainBlocked
	   if hot {
		   if p.State == PlanOpen || p.State == PlanPlanning || p.State == PlanExecuting {
			   if err := o.enterHuman(ctx, p, f, "live_hash", now); err != nil {
				  return o.view(planID), err
			  }
		  }
		  return o.view(planID), ErrNeedsHuman
	  }
	  if p.State == PlanNeedsHuman {
		   return o.view(planID), ErrNeedsHuman
	  }
	   if p.State == PlanPlanning {
		  if !p.AutoOK {
			  if err := o.enterHuman(ctx, p, f, "policy", now); err != nil {
				  return o.view(planID), err
			   }
			   return o.view(planID), ErrNeedsHuman
		   }
		   if err := o.transit(p, PlanExecuting, now); err != nil {
			   return o.view(planID), err
		   }
		  f.Status = FailCompensating
		  f.UpdatedAt = now
	   }
	   if p.State != PlanExecuting {
		  return o.view(planID), ErrState
	   }
	   for _, leg := range o.legs[planID] {
		   if leg.Status == LegDone || leg.Status == LegSkipped || leg.Status == LegBlocked || leg.Status == LegRejected {
			  continue
		  }
		  if err := o.runLeg(ctx, p, f, leg, now); err != nil {
			  leg.Attempts++
			  leg.ErrText = err.Error()
			  leg.Status = LegFailed
			  leg.UpdatedAt = now
			  o.mirrorLeg(ctx, leg)
			  if errorsIsStop(err) || leg.Attempts >= 3 {
				  _ = o.enterHuman(ctx, p, f, "leg_stop", now)
			  }
			   return o.view(planID), err
		  }
	  }
	  if !legsReady(o.legs[planID]) {
		   return o.view(planID), ErrState
	   }
	  if err := o.transit(p, PlanCommitted, now); err != nil {
		   return o.view(planID), err
	   }
	   p.CommittedAt = now
	   f.Status = FailClosed
	  f.UpdatedAt = now
	  o.mirrorPlan(ctx, p)
	   o.mirrorFailure(ctx, f)
	   notePlan(string(PlanCommitted))
	   return o.view(planID), nil
   }

  func errorsIsStop(err error) bool {
	  return err == ErrAutoUnfreezeDenied || err == ErrFilledProtected || err == wdrollback.ErrAutoUnfreezeDenied || err == wdrollback.ErrFilledProtected
   }

   func (o *Orchestrator) runLeg(ctx context.Context, p *CompensatePlan, f *WdFailure, leg *CompensateLeg, now time.Time) error {
	   leg.Status = LegRunning
	  leg.UpdatedAt = now
	  if leg.Kind == LegIncident {
		  o.openIncident(p, f, "plan", "", "", "补偿腿挂事故", now)
		   leg.Status = LegDone
		  leg.ResultRef = p.HumanTicketID
		   o.mirrorLeg(ctx, leg)
		  return nil
	   }
	  amt, err := parseOpt(leg.Amount)
	  if err != nil {
		   return err
	  }
	  if isMoneyKind(leg.Kind) && leg.Kind != LegShard && !amt.IsPositive() {
		  leg.Status = LegSkipped
		   o.mirrorLeg(ctx, leg)
		  return nil
	  }
	  req := wdrollback.Request{
		  PlanID: p.ID, LegID: leg.ID, FailureID: f.ID, TransferID: f.TransferID,
		  UserID: f.UserID, Asset: f.Asset, Amount: amt, Mode: wdrollback.ModeAuto,
		   LiveHash: f.LiveHash || p.LiveHash, ChainBlocked: f.ChainBlocked || p.ChainBlocked,
		  IdempotencyKey: leg.IdempotencyKey, ReasonCode: f.ReasonCode, Path: string(leg.Kind),
		   TxHash: f.TxHash, Now: now,
	  }
	   switch leg.Kind {
	  case LegOccupancy:
		  req.Kind = wdrollback.KindOccupancy
	   case LegUnfreeze:
		  req.Kind = wdrollback.KindFreeze
	  case LegFee:
		   req.Kind = wdrollback.KindFee
	   case LegShard:
		  req.Kind = wdrollback.KindShard
	   case LegSeal:
		  req.Kind = wdrollback.KindSeal
		  req.Amount = money.Zero()
	  case LegAudit:
		  req.Kind = wdrollback.KindDeny
		   req.Path = "audit_deny"
		   req.Amount = money.Zero()
	   default:
		   return ErrLeg
	  }
	   res, err := o.svc.Apply(req)
	   if err != nil {
		   if err == wdrollback.ErrAutoUnfreezeDenied {
			  noteDeny()
		   }
		   return err
	   }
	  if req.Kind == wdrollback.KindFreeze || req.Kind == wdrollback.KindFee || req.Kind == wdrollback.KindOccupancy {
		   noteRollback(req.Kind)
	  }
	  leg.Status = LegDone
	  leg.ResultRef = res.Entry.ID
	   leg.UpdatedAt = now
	   o.mirrorLeg(ctx, leg)
	   o.mirrorEntry(ctx, res.Entry)
	  return nil
   }

   func (o *Orchestrator) view(planID string) PlanView {
	   p := o.plans[planID]
	   if p == nil {
		  return PlanView{}
	  }
	  legs := make([]CompensateLeg, 0, len(o.legs[planID]))
	   for _, leg := range o.legs[planID] {
		  legs = append(legs, *leg)
	   }
	  var f WdFailure
	   if src := o.fail[p.FailureID]; src != nil {
		  f = *src
	   }
	  return PlanView{Plan: *p, Legs: legs, Failure: f}
  }

// Failure 按编号读取失败单。
   func (o *Orchestrator) Failure(id string) (WdFailure, error) {
	  if o == nil {
		  return WdFailure{}, ErrUnavailable
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   f := o.fail[strings.TrimSpace(id)]
	  if f == nil {
		   return WdFailure{}, ErrNotFound
	  }
	   return *f, nil
  }

// Plan 按编号读取计划。

  func (o *Orchestrator) Plan(id string) (PlanView, error) {
	  if o == nil {
		  return PlanView{}, ErrUnavailable
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  if o.plans[strings.TrimSpace(id)] == nil {
		  return PlanView{}, ErrNotFound
	  }
	   return o.view(id), nil
   }

// Failures 列表。
  func (o *Orchestrator) Failures(q ListFilter) []WdFailure {
	   if o == nil {
		   return nil
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	  return o.listFail(q)
  }

   func (o *Orchestrator) listFail(q ListFilter) []WdFailure {
	  ids := make([]string,
		  0, len(o.fail))
	  for id := range o.fail {
		  ids = append(ids, id)
	  }
	  sort.Strings(ids)
	  limit := q.Limit
	   if limit <= 0 {
		   limit = 50
	  }
	  if limit > 200 {
		   limit = 200
	   }
	   var out []WdFailure
	  for _, id := range ids {
		   f := o.fail[id]
		  if q.TransferID != "" && f.TransferID != q.TransferID {
			   continue
		   }
		  if q.UserID != "" && f.UserID != q.UserID {
			   continue
		  }
		   if q.Asset != "" && f.Asset != q.Asset {
			   continue
		  }
		   if q.Status != "" && string(f.Status) != q.Status {
			   continue
		   }
		   if q.Code != "" && string(f.Code) != q.Code {
			  continue
		  }
		  out = append(out, *f)
		  if len(out) >= limit {
			   break
		   }
	   }
	   return out
   }

// Plans 列表。

   func (o *Orchestrator) Plans(q ListFilter) []CompensatePlan {
	   if o == nil {
		  return nil
	  }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   ids := make([]string,
		  0, len(o.plans))
	   for id := range o.plans {
		  ids = append(ids, id)
	  }
	  sort.Strings(ids)
	  limit := q.Limit
	  if limit <= 0 {
		  limit = 50
	   }
	  if limit > 200 {
		   limit = 200
	  }
	   var out []CompensatePlan
	  for _, id := range ids {
		  p := o.plans[id]
		  if q.TransferID != "" && p.TransferID != q.TransferID {
			  continue
		   }
		   if q.UserID != "" && p.UserID != q.UserID {
			  continue
		  }
		   if q.Status != "" && string(p.State) != q.Status {
			   continue
		  }
		   out = append(out, *p)
		   if len(out) >= limit {
			  break
		   }
	  }
	   return out
  }

// Rollback 读取回滚分录。
  func (o *Orchestrator) Rollback(user, asset, transfer string) []wdrollback.Entry {
	   if o == nil || o.svc == nil {
		   return nil
	   }
	   return o.svc.Entries(user, asset, transfer)
   }

// Audits 读取自动解冻拒绝审计。
   func (o *Orchestrator) Audits() []wdrollback.Audit {
	  if o == nil || o.svc == nil {
		  return nil
	  }
	   return o.svc.Audits()
   }

// Incidents 读取事故挂钩。

   func (o *Orchestrator) Incidents() []Incident {
	  if o == nil {
		   return nil
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  return append([]Incident(nil), o.incidents...)
  }

// Position 读取用户资产位并附带守恒结论。
  func (o *Orchestrator) Position(user, asset string) (PositionView, error) {
	  if o == nil || o.svc == nil {
		   return PositionView{}, ErrUnavailable
	  }
	   pos, err := o.svc.Position(user, asset)
	   if err != nil {
		   return PositionView{}, err
	  }
	  eq, err := pos.Equity()
	   if err != nil {
		  return PositionView{}, err
	   }
	   conserved := o.svc.Assert(user, asset) == nil
	   return PositionView{
		   UserID: pos.UserID, Asset: pos.Asset,
		   Available: pos.Available.String(), Frozen: pos.Frozen.String(),
		   FeeReserved: pos.FeeReserved.String(),
		  Occupancy: pos.Occupancy.String(),
		  Equity:    eq.String(),
		  Conserved: conserved,
	   }, nil
   }

// AssertPosition 夹具用的守恒断言。

  func (o *Orchestrator) AssertPosition(user, asset string) error {
	   if o == nil || o.svc == nil {
		  return ErrUnavailable
	  }
	  return o.svc.Assert(user, asset)
  }

// Policies 当前策略表。
   func (o *Orchestrator) Policies() []AutoPolicy {
	   if o == nil {
		  return nil
	   }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	   return o.policies.List()
  }

// PutPolicy 更新一条策略。live hash 不允许改成自动解冻。
   func (o *Orchestrator) PutPolicy(ctx context.Context, in AutoPolicy) (AutoPolicy, error) {
	   if err := ctx.Err(); err != nil {
		  return AutoPolicy{}, err
	   }
	  if o == nil {
		  return AutoPolicy{}, ErrUnavailable
	  }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	   if in.UpdatedAt.IsZero() {
		  in.UpdatedAt = o.now()
	  }
	   row, err := o.policies.Put(in)
	  if err != nil {
		  return AutoPolicy{}, err
	  }
	  o.mirrorPolicy(ctx, row)
	   return row, nil
  }

// MirrorError 最近一次 SQL 镜像错误。内存账仍是资金真相。

   func (o *Orchestrator) MirrorError() string {
	   if o == nil {
		  return ""
	  }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	  return o.mirrorErr
   }
