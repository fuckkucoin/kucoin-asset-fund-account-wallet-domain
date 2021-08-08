// 功能：迟贷记持久互斥服务。
// OccupyWithGen / CheckCreditDurable / AutoCredit 抬升 late_credit_gen；
// 写路径强制围栏；同 output_key 占用与贷记互斥，冲突开 LateCreditRaceProof
// 且贷记 fail-closed。
// 不重写 错链回收编排状态名与补偿图。

// 作者：Cason
// 日期：2021.08.07

package latecreditgate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/reclaim"
)

// Service 迟贷记世代闸。
type Service struct {
	store Store
	policy FencePolicy
	clock  func() time.Time
	cases reclaim.ReclaimCaseReader
	occupancy reclaim.ReclaimOccupancyReader
	timeline  reclaim.TimelineAppender
}

// New 使用内存仓创建服务。
   func New() *Service {
	  return NewWithStore(NewMemStore())
  }

// NewWithStore 注入存储。

   func NewWithStore(store Store) *Service {
	  return &Service{
		  store:  store,
		   policy: DefaultFencePolicy(),
		  clock:  func() time.Time { return time.Now().UTC() },
		   cases: reclaim.NopCaseReader{},
		  occupancy: reclaim.NopOccupancyReader{},
		  timeline:  reclaim.NopTimelineAppender{},
	   }
  }

// WithClock 时钟。

   func (s *Service) WithClock(c func() time.Time) *Service {
	   if c != nil {
		  s.clock = c
	   }
	   return s
  }

  // WithFencePolicy 覆盖围栏策略。

   func (s *Service) WithFencePolicy(p FencePolicy) *Service {
	  s.policy = p
	   return s
  }

// WithReaders 挂钩只读 ReclaimCase / Occupancy / Timeline。
   func (s *Service) WithReaders(c reclaim.ReclaimCaseReader, o reclaim.ReclaimOccupancyReader, t reclaim.TimelineAppender) *Service {
	   if c != nil {
		  s.cases = c
	   }
	   if o != nil {
		  s.occupancy = o
	   }
	   if t != nil {
		  s.timeline = t
	   }
	  return s
   }

   func (s *Service) now() time.Time { return s.clock() }

// ─────────────────────────── OccupyWithGen ──────────────────────────────────
// OccupyWithGen 抬升并持久化 occupied 世代。必须携带有效围栏。
  func (s *Service) OccupyWithGen(ctx context.Context, req OccupyRequest) (*LateCreditGeneration, error) {
	  if err := validateOutput(req.OutputKey, req.ActorID); err != nil {
		  return nil, err
	  }
	  if err := MustFenceWrite(req.Fence); err != nil {
		   return nil, err
	   }
	  now := s.now()
	   existing,
		  err := s.store.GetGen(ctx, req.OutputKey)
	   if err != nil && !errors.Is(err, ErrGenNotFound) {
		   return nil, err
	  }
	   if existing != nil {
		  if existing.State == GenOccupied && existing.HolderActor == req.ActorID {
			   return existing, nil
		   }
		   if existing.State == GenOccupied || existing.IsTerminal() {
			   if err := s.openRace(ctx, existing, req.ActorID, "occupy", RaceDoubleOccupy, "duplicate occupy"); err != nil {
				  return nil, err
			  }
			   return nil, ErrOccupyConflict
		  }
		   next := *existing
		  next.Generation++
		  next.State = GenOccupied
		   next.HolderKind = "occupy"
		  next.HolderActor = req.ActorID
		   next.OccupancyID = req.OccupancyID
		   next.CaseID = req.CaseID
		   next.Asset = req.Asset
		  next.Amount = req.Amount
		  next.FenceEpoch = req.Fence.Epoch
		   next.FenceToken = req.Fence.Token
		  next.UpdatedAt = now
		  if err := s.store.CASUpdateGen(ctx, existing.GenID, existing.Generation, existing.State, next); err != nil {
			   return nil, err
		   }
		   s.appendTL(req.OutputKey, req.CaseID, "occupy", req.ActorID, fmt.Sprintf("gen=%d", next.Generation), now)
		   return &next, nil
	  }
	   g := LateCreditGeneration{
		  GenID: uuid.NewString(),
		   OutputKey:  req.OutputKey,
		  Generation: 1,
		  State: GenOccupied,
		   HolderKind: "occupy",
		  HolderActor: req.ActorID,
		  OccupancyID: req.OccupancyID,
		   CaseID:      req.CaseID,
		   Asset:       req.Asset,
		   Amount: req.Amount,
		  FenceEpoch: req.Fence.Epoch,
		   FenceToken: req.Fence.Token,
		  CreatedAt:  now,
		  UpdatedAt: now,
	  }
	   if err := s.store.InsertGen(ctx, g); err != nil {
		  if errors.Is(err, ErrOccupyConflict) {
			  got, gerr := s.store.GetGen(ctx, req.OutputKey)
			  if gerr != nil {
				  return nil, ErrOccupyConflict
			  }
			  if got.HolderActor == req.ActorID && got.State == GenOccupied {
				  return got, nil
			   }
			  _ = s.openRace(ctx, got, req.ActorID, "occupy", RaceDoubleOccupy, "insert race")
			   return nil, ErrOccupyConflict
		   }
		   return nil, err
	   }
	  s.appendTL(req.OutputKey, req.CaseID, "occupy", req.ActorID, "gen=1", now)
	  return &g, nil
   }

// ─────────────────────────── CheckCreditDurable ─────────────────────────────
// CheckCreditDurable 贷记前互斥检查。occupied 则 fail-closed 并开证明。

   func (s *Service) CheckCreditDurable(ctx context.Context, req CreditRequest) (*LateCreditGeneration, error) {
	  if err := validateOutput(req.OutputKey, req.ActorID); err != nil {
		  return nil, err
	  }
	  if err := MustFenceWrite(req.Fence); err != nil {
		  return nil, err
	  }
	   kind := req.Kind
	  if kind == "" {
		   kind = "credit"
	   }
	   now := s.now()
	   existing, err := s.store.GetGen(ctx, req.OutputKey)
	  if err != nil && !errors.Is(err, ErrGenNotFound) {
		  return nil, err
	  }
	   if existing != nil {
		  if existing.State == GenCredited && existing.HolderActor == req.ActorID {
			  return existing, nil
		   }
		   if existing.State == GenCredited {
			  return nil, ErrAlreadyCredited
		   }
		   if existing.State == GenReclaimed {
			  return nil, ErrAlreadyReclaimed
		   }
		   if existing.State == GenOccupied {
			  rk := RaceOccupyVsCredit
			  if kind == "auto_credit" {
				  rk = RaceOccupyVsAutoCredit
			  }
			  if err := s.openRace(ctx, existing, req.ActorID, kind, rk, "credit vs occupy mutex"); err != nil {
				   return nil, err
			   }
			   return nil, ErrCreditBlocked
		  }
		  opens, _ := s.store.ListOpenRaceEventsByKey(ctx, req.OutputKey)
		   if len(opens) > 0 {
			   return nil, ErrRaceStillOpen
		   }
		   next := *existing
		   next.Generation++
		  next.State = GenCredited
		  next.HolderKind = kind
		   next.HolderActor = req.ActorID
		  next.Asset = req.Asset
		   next.Amount = req.Amount
		  next.FenceEpoch = req.Fence.Epoch
		  next.FenceToken = req.Fence.Token
		  next.UpdatedAt = now
		  if err := s.store.CASUpdateGen(ctx, existing.GenID, existing.Generation, existing.State, next); err != nil {
			   return nil, err
		  }
		  s.appendTL(req.OutputKey, existing.CaseID, kind, req.ActorID, fmt.Sprintf("gen=%d", next.Generation), now)
		  return &next, nil
	  }
	  g := LateCreditGeneration{
		   GenID: uuid.NewString(),
		   OutputKey:  req.OutputKey,
		   Generation: 1,
		   State:      GenCredited,
		  HolderKind: kind,
		  HolderActor: req.ActorID,
		  Asset: req.Asset,
		   Amount:     req.Amount,
		  FenceEpoch: req.Fence.Epoch,
		   FenceToken: req.Fence.Token,
		   CreatedAt:  now,
		  UpdatedAt:  now,
	   }
	  if err := s.store.InsertGen(ctx, g); err != nil {
		  if errors.Is(err, ErrOccupyConflict) {
			  got, gerr := s.store.GetGen(ctx, req.OutputKey)
			  if gerr == nil && got.State == GenOccupied {
				   rk := RaceOccupyVsCredit
				   if kind == "auto_credit" {
					   rk = RaceOccupyVsAutoCredit
				   }
				  _ = s.openRace(ctx, got, req.ActorID, kind, rk, "credit insert lost mutex")
				   return nil, ErrCreditBlocked
			   }
			  return nil, ErrOccupyConflict
		  }
		   return nil, err
	   }
	  s.appendTL(req.OutputKey, "", kind, req.ActorID, "gen=1 credited", now)
	  return &g, nil
  }

// AutoCredit 自动贷记写路径：强制围栏 + 互斥。

  func (s *Service) AutoCredit(ctx context.Context, req CreditRequest) (*LateCreditGeneration, error) {
	  req.Kind = "auto_credit"
	  return s.CheckCreditDurable(ctx, req)
  }

// ─────────────────────────── Forced write paths ─────────────────────────────
// GuardWrite 写路径（Reserve/Reclaim/Resolve/AutoCredit）强制围栏，省略则不推进。
   func (s *Service) GuardWrite(ctx context.Context, req WriteRequest) (*LateCreditGeneration, error) {
	  if err := ValidateWritePath(req.Path); err != nil {
		  return nil, err
	  }
	   if err := validateOutput(req.OutputKey, req.ActorID); err != nil {
		  return nil, err
	   }
	  if err := MustFenceWrite(req.Fence); err != nil {
		  return nil, err
	   }
	  now := s.now()
	  existing,
		   err := s.store.GetGen(ctx, req.OutputKey)
	  if err != nil && !errors.Is(err, ErrGenNotFound) {
		   return nil, err
	   }
	   nextState := req.NextState
	   if nextState == "" {
		  switch req.Path {
		  case PathReserve, PathOccupy:
			  nextState = GenOccupied
		  case PathReclaim, PathResolve:
			   nextState = GenReclaimed
		   case PathAutoCredit, PathCredit:
			   nextState = GenCredited
		   default:
			   nextState = GenOccupied
		   }
	   }
	  if existing == nil {
		  g := LateCreditGeneration{
			   GenID: uuid.NewString(),
			  OutputKey:  req.OutputKey,
			   Generation: 1,
			  State:      nextState,
			   HolderKind: string(req.Path),
			  HolderActor: req.ActorID,
			  FenceEpoch:  req.Fence.Epoch,
			  FenceToken:  req.Fence.Token,
			   CreatedAt: now,
			   UpdatedAt: now,
		   }
		  if err := s.store.InsertGen(ctx, g); err != nil {
			   return nil, err
		  }
		   s.appendTL(req.OutputKey, "", string(req.Path), req.ActorID, "guard write insert", now)
		  return &g, nil
	  }
	  if existing.IsTerminal() && existing.State != nextState {
		   return nil, ErrStateInvalid
	  }
	  next := *existing
	  next.Generation++
	   next.State = nextState
	   next.HolderKind = string(req.Path)
	   next.HolderActor = req.ActorID
	  next.FenceEpoch = req.Fence.Epoch
	  next.FenceToken = req.Fence.Token
	  next.UpdatedAt = now
	   if err := s.store.CASUpdateGen(ctx, existing.GenID, existing.Generation, existing.State, next); err != nil {
		  return nil, err
	  }
	   s.appendTL(req.OutputKey, existing.CaseID, string(req.Path), req.ActorID, fmt.Sprintf("gen=%d", next.Generation), now)
	  return &next, nil
   }

// Reserve / Reclaim / Resolve 薄封装，保证写路径不得省略 fence。
  func (s *Service) Reserve(ctx context.Context, key OutputKey, actor string, fence FenceToken) (*LateCreditGeneration, error) {
	  return s.GuardWrite(ctx, WriteRequest{Path: PathReserve, OutputKey: key, ActorID: actor, Fence: fence, NextState: GenOccupied})
   }

   func (s *Service) Reclaim(ctx context.Context, key OutputKey, actor string, fence FenceToken) (*LateCreditGeneration, error) {
	   return s.GuardWrite(ctx,
		   WriteRequest{Path: PathReclaim, OutputKey: key, ActorID: actor, Fence: fence, NextState: GenReclaimed})
   }

   func (s *Service) Resolve(ctx context.Context, key OutputKey, actor string, fence FenceToken) (*LateCreditGeneration, error) {
	  return s.GuardWrite(ctx, WriteRequest{Path: PathResolve, OutputKey: key, ActorID: actor, Fence: fence, NextState: GenReclaimed})
  }

// ─────────────────────────── Race / Proof ───────────────────────────────────

   func (s *Service) openRace(ctx context.Context, g *LateCreditGeneration, otherActor, otherKind string, kind RaceKind, detail string) error {
	   now := s.now()
	   ev := RaceEvent{
		   EventID: uuid.NewString(),
		   OutputKey: g.OutputKey,
		  GenID:      g.GenID,
		   Generation: g.Generation,
		  Kind:       kind,
		  State: RaceOpen,
		   ActorA: g.HolderActor,
		  ActorB:    otherActor,
		  Detail:    detail,
		   CreatedAt: now,
	  }
	   if err := s.store.InsertRaceEvent(ctx, ev); err != nil {
		   return err
	   }
	   p := LateCreditRaceProof{
		   ProofID: uuid.NewString(),
		   OutputKey: g.OutputKey,
		   GenID: g.GenID,
		   Generation: g.Generation,
		  EventID:    ev.EventID,
		  Kind: kind,
		   OccupyActor: g.HolderActor,
		  CreditActor: otherActor,
		  FailClosed:  true,
		  Summary:     fmt.Sprintf("%s holder=%s vs %s actor=%s gen=%d", kind, g.HolderActor, otherKind, otherActor, g.Generation),
		   OpenedAt:    now,
	  }
	   if err := s.store.InsertProof(ctx, p); err != nil {
		  return err
	   }
	  s.appendTL(g.OutputKey, g.CaseID, "race_open", otherActor, p.Summary, now)
	  return nil
  }

// DrainOpenRaces 导出未 Ack 竞态队列（重启后仍可用）。
   func (s *Service) DrainOpenRaces(ctx context.Context, limit int) ([]RaceEvent, error) {
	   if limit <= 0 {
		   limit = 200
	   }
	  return s.store.ListOpenRaceEvents(ctx, limit)
  }

// AckRace 确认处理竞态事件。
   func (s *Service) AckRace(ctx context.Context, req ResolveRaceRequest) (*RaceEvent, error) {
	  if strings.TrimSpace(req.EventID) == "" {
		  return nil, ErrInvalidRequest
	  }
	  now := s.now()
	   if err := s.store.AckRaceEvent(ctx, req.EventID, now); err != nil {
		  return nil, err
	  }
	  return s.store.GetRaceEvent(ctx, req.EventID)
  }

// ExportProofs 导出竞态证明。
   func (s *Service) ExportProofs(ctx context.Context, limit int) ([]LateCreditRaceProof, error) {
	  if limit <= 0 {
		  limit = 200
	   }
	   proofs, err := s.store.ListProofs(ctx, limit)
	   if err != nil {
		   return nil, err
	   }
	  now := s.now()
	   for i := range proofs {
		   _ = s.store.MarkProofExported(ctx, proofs[i].ProofID, now)
	  }
	   return proofs, nil
  }

// GetGeneration 只读查询世代（允许 legacy 省略围栏）。
  func (s *Service) GetGeneration(ctx context.Context, key OutputKey, fence FenceToken, legacyRead bool) (*LateCreditGeneration, error) {
	  if key.String() == "" {
		  return nil, ErrOutputKeyEmpty
	  }
	  if !legacyRead {
		  if err := CheckFence(fence, false, s.policy); err != nil {
			   return nil, err
		   }
	   }
	  return s.store.GetGen(ctx, key)
  }

// View 只读挂钩 ReclaimCase / Occupancy / Timeline。

   func (s *Service) View(ctx context.Context, key OutputKey) (*OutputView, error) {
	   v := &OutputView{OutputKey: key}
	  g, err := s.store.GetGen(ctx, key)
	   if err != nil && !errors.Is(err, ErrGenNotFound) {
		   return nil, err
	  }
	  v.Generation = g
	  if s.occupancy != nil {
		   if occ, oerr := s.occupancy.GetActiveOccupancy(key); oerr == nil {
			   v.Occupancy = occ
		  }
	   }
	   if s.cases != nil {
		   if c, cerr := s.cases.GetActiveCase(key); cerr == nil {
			  v.Case = c
		  }
	   }
	  v.OpenRaces, _ = s.store.ListOpenRaceEventsByKey(ctx, key)
	   v.Proofs, _ = s.store.ListProofsByKey(ctx, key)
	  return v, nil
  }

  func (s *Service) appendTL(key OutputKey, caseID, kind, actor, detail string, now time.Time) {
	  _ = s.timeline.AppendEvent(reclaim.ViewTimelineEvent{
		   EventID: uuid.NewString(),
		   CaseID: caseID,
		  OutputKey: key,
		   Kind:      kind,
		   Actor:     actor,
		   Detail: detail,
		   OccurredAt: now,
	  })
  }

   func validateOutput(key OutputKey, actor string) error {
	   if key.String() == "" {
		   return ErrOutputKeyEmpty
	  }
	   if strings.TrimSpace(actor) == "" {
		   return ErrActorEmpty
	  }
	   return nil
   }

// CurrentGeneration 返回世代号，不存在则为 0（只读）。

  func (s *Service) CurrentGeneration(ctx context.Context, key OutputKey) (int64, error) {
	  g,
		  err := s.store.GetGen(ctx, key)
	   if errors.Is(err, ErrGenNotFound) {
		   return 0, nil
	   }
	   if err != nil {
		   return 0, err
	   }
	  return g.Generation, nil
  }
