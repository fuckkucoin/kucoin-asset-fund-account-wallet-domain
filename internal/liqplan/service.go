package liqplan

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LTVReader 只读消费 marginhook 的 LTV 计算结果。
// 不写 marginhook 内部状态；由 marginhook.Service 在 LTV 扫描时调用 TriggerPlan。

type LTVReader interface {
	CurrentLTV(borrowID string) (ltvBPS,
		capBPS int, err error)
}

// MarkReader 只读消费 oracle/fxmark 的标价世代。
// 不写行情主路径；仅观察世代序号与价格。

type MarkReader interface {
	LatestMark(asset string) (price string, generation int64, observedAt time.Time, err error)
}

// BorrowReader 只读消费 marginhook 借款与抵押信息。
type BorrowReader interface {
	BorrowInfo(borrowID string) (userID, borrowAsset, borrowAmount, collateralID, collateralAsset, collateralAmount string, err error)
}

// Service 是 liqplan 的核心服务。
type Service struct {
	plans *PlanStore
	legs   *LegStore
	marks  *MarkGateStore
	proofs *ProofStore
	occupy   *OccupyStore
	fenceTab *persist.Table
	clock func() time.Time
	ltvReader    LTVReader
	markReader   MarkReader
	borrowReader BorrowReader
	markMaxAgeSeconds int
}

   func New() *Service {
	  return &Service{
		  plans: NewPlanStore(),
		   legs:  NewLegStore(),
		  marks:    NewMarkGateStore(),
		  proofs:   NewProofStore(),
		  occupy:   NewOccupyStore(),
		   fenceTab: persist.NewTable(),
		  clock:    func() time.Time { return time.Now().UTC() },
		   markMaxAgeSeconds: 60,
	  }
   }

   func (s *Service) WithLTVReader(r LTVReader) *Service {
	  s.ltvReader = r
	   return s
  }

  func (s *Service) WithMarkReader(r MarkReader) *Service {
	  s.markReader = r
	  return s
  }

   func (s *Service) WithBorrowReader(r BorrowReader) *Service {
	   s.borrowReader = r
	   return s
   }

  func (s *Service) WithMarkMaxAge(seconds int) *Service {
	  if seconds > 0 {
		  s.markMaxAgeSeconds = seconds
	   }
	   return s
  }

   func (s *Service) WithClock(fn func() time.Time) *Service {
	  if fn != nil {
		  s.clock = fn
	  }
	  return s
   }

   // ─────────────────────────────────────────────
  // TriggerPlan：LTV 穿仓触发强平计划
   // ─────────────────────────────────────────────
   // TriggerReq 是触发强平计划的请求。
  type TriggerReq struct {
	   BorrowID       string
	   IdempotencyKey string
	   // 若 BorrowReader 未注入，可直接填快照（用）
	   UserID      string
	   BorrowAsset string
	  BorrowAmount    string
	  CollateralID    string
	  CollateralAsset string
	  CollateralAmount string
	  TriggerLTVBPS    int
	   CapBPS int
   }

  // TriggerPlan：LTV 破线 → 生成 LiqPlan(triggered) + 颁发占用令牌。
  // 若同一 BorrowID 已有活跃计划，返回 ErrAlreadyExists（幂等）。
   // 若标价世代过期/中毒，返回 ErrMarkStale（fail-closed）。
  func (s *Service) TriggerPlan(req TriggerReq) (LiqPlan, error) {
	   if s == nil {
		  return LiqPlan{},
			   ErrUnavailable
	   }
	  if req.IdempotencyKey != "" {
		  if prev, ok := s.plans.ByIdem(req.IdempotencyKey); ok {
			   return prev, nil
		  }
	   }
	   if existing, ok := s.plans.ByBorrow(req.BorrowID); ok {
		  return existing, ErrAlreadyExists
	   }
	  now := s.clock()
	   userID := req.UserID
	  borrowAsset := req.BorrowAsset
	  borrowAmount := req.BorrowAmount
	   collateralID := req.CollateralID
	   collateralAsset := req.CollateralAsset
	  collateralAmount := req.CollateralAmount
	   triggerLTV := req.TriggerLTVBPS
	  capBPS := req.CapBPS
	  if s.borrowReader != nil {
		  uid,
			  ba, bamt, cid, ca, camt, err := s.borrowReader.BorrowInfo(req.BorrowID)
		  if err != nil {
			  return LiqPlan{}, fmt.Errorf("borrow info: %w", err)
		  }
		  userID = uid
		  borrowAsset = ba
		   borrowAmount = bamt
		  collateralID = cid
		  collateralAsset = ca
		   collateralAmount = camt
	  }
	  if s.ltvReader != nil {
		   ltvBPS, cap, err := s.ltvReader.CurrentLTV(req.BorrowID)
		   if err != nil {
			  return LiqPlan{}, fmt.Errorf("ltv read: %w", err)
		   }
		   if ltvBPS <= cap {
			  return LiqPlan{}, ErrLTVInsideCap
		  }
		   triggerLTV = ltvBPS
		  capBPS = cap
	   }
	  var markGateID string
	  var markGen int64
	   if s.markReader != nil {
		  price, gen, obsAt, err := s.markReader.LatestMark(collateralAsset)
		  if err != nil {
			   return LiqPlan{}, fmt.Errorf("%w: mark read: %v", ErrMarkStale, err)
		   }
		  gate := MarkGate{
			   ID:        uuid.NewString(),
			   Asset:     collateralAsset,
			  MarkPrice: price,
			   Generation:    gen,
			  ObservedAt:    obsAt,
			  MaxAgeSeconds: s.markMaxAgeSeconds,
			  CreatedAt: now,
		   }
		  if gate.IsStale(now) {
			  return LiqPlan{}, ErrMarkStale
		  }
		   s.marks.Put(gate)
		  markGateID = gate.ID
		   markGen = gen
	  } else {
		  if g, ok := s.marks.Latest(collateralAsset); ok {
			   if g.IsStale(now) {
				  return LiqPlan{}, ErrMarkStale
			  }
			  markGateID = g.ID
			   markGen = g.Generation
		   }
	  }
	  planID := uuid.NewString()
	   token := persist.NewToken()
	   plan := LiqPlan{
		  ID: planID,
		  UserID:       userID,
		  BorrowID:     req.BorrowID,
		  CollateralID: collateralID,
		  TriggerLTVBPS: triggerLTV,
		   CapBPS: capBPS,
		   CollateralAsset: collateralAsset,
		  CollateralAmount: collateralAmount,
		   BorrowAsset:      borrowAsset,
		  BorrowAmount: borrowAmount,
		  OccupyToken:  token,
		   MarkGateID:     markGateID,
		  MarkGeneration: markGen,
		  State:          PlanTriggered,
		   IdempotencyKey: req.IdempotencyKey,
		  CreatedAt:      now,
		  UpdatedAt:      now,
	  }
	   ot := OccupyToken{Token: token, BorrowID: req.BorrowID, PlanID: planID, IssuedAt: now}
	  if err := s.occupy.Acquire(ot); err != nil {
		  return LiqPlan{}, err
	  }
	   s.plans.Put(plan)
	   return plan, nil
   }

// ─────────────────────────────────────────────
// AddSeizeLeg：追加扣押腿
// ─────────────────────────────────────────────
// AddLegReq 追加扣押腿请求。

type AddLegReq struct {
	PlanID         string
	Asset          string
	TargetAmount   string
	HoldID         string
	IdempotencyKey string
}

// AddSeizeLeg 在计划中追加一条扣押腿，并推进计划至 seizing。
   func (s *Service) AddSeizeLeg(req AddLegReq) (SeizeLeg, error) {
	  if s == nil {
		   return SeizeLeg{}, ErrUnavailable
	  }
	   if req.IdempotencyKey != "" {
		   if prev, ok := s.legs.ByIdem(req.IdempotencyKey); ok {
			   return prev, nil
		  }
	  }
	   plan, ok := s.plans.Get(req.PlanID)
	   if !ok {
		   return SeizeLeg{}, ErrNotFound
	  }
	  if plan.State != PlanTriggered && plan.State != PlanSeizing {
		   return SeizeLeg{},
			  ErrStateIllegal
	  }
	  now := s.clock()
	  leg := SeizeLeg{
		  ID: uuid.NewString(),
		   PlanID: req.PlanID,
		   UserID:         plan.UserID,
		   Asset:          req.Asset,
		  TargetAmount:   req.TargetAmount,
		   PartialAmount:  "0",
		  HoldID:         req.HoldID,
		  BorrowID:       plan.BorrowID,
		  State:          LegPending,
		   IdempotencyKey: req.IdempotencyKey,
		  MaxRetry:       3,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	   s.legs.Put(leg)
	  if plan.State == PlanTriggered {
		  next,
			  err := AdvancePlan(plan.State, EvSeizeStart)
		   if err != nil {
			   return leg, err
		   }
		  plan.State = next
		  plan.UpdatedAt = now
		   s.plans.Put(plan)
	  }
	   return leg, nil
  }

// ─────────────────────────────────────────────
// StartLeg / CompleteLeg / FailLeg
// ─────────────────────────────────────────────
// StartLeg 将腿从 pending 推进至 executing。
  func (s *Service) StartLeg(legID string) (SeizeLeg, error) {
	  leg, ok := s.legs.Get(legID)
	   if !ok {
		  return SeizeLeg{}, ErrLegNotFound
	   }
	  next, err := AdvanceLeg(leg.State, EvLegStart)
	  if err != nil {
		  return leg, err
	   }
	  leg.State = next
	   leg.UpdatedAt = s.clock()
	   s.legs.Put(leg)
	   return leg, nil
  }

// CompleteLeg 将腿标记为 done，并检查是否可推进计划至 settling。
   func (s *Service) CompleteLeg(legID, seizedAmount string) (SeizeLeg, LiqPlan, error) {
	   leg, ok := s.legs.Get(legID)
	   if !ok {
		   return SeizeLeg{}, LiqPlan{}, ErrLegNotFound
	  }
	   next, err := AdvanceLeg(leg.State, EvLegSucceed)
	  if err != nil {
		   return leg, LiqPlan{}, err
	   }
	  leg.State = next
	  leg.PartialAmount = seizedAmount
	  leg.UpdatedAt = s.clock()
	   s.legs.Put(leg)
	   plan, _ := s.plans.Get(leg.PlanID)
	   plan = s.maybeAdvancePlanAfterLeg(plan)
	  return leg, plan, nil
  }

// FailLeg 将腿标记为 failed，记录失败原因与已扣押金额（可能为 0 或部分）。
   func (s *Service) FailLeg(legID, partialAmount, reason string) (SeizeLeg, LiqPlan, error) {
	   leg, ok := s.legs.Get(legID)
	  if !ok {
		  return SeizeLeg{}, LiqPlan{}, ErrLegNotFound
	   }
	  next, err := AdvanceLeg(leg.State, EvLegFail)
	   if err != nil {
		  return leg, LiqPlan{}, err
	   }
	   now := s.clock()
	   leg.State = next
	  leg.PartialAmount = partialAmount
	  leg.FailReason = reason
	   leg.FailAt = now
	   leg.RetryCount++
	  leg.UpdatedAt = now
	   s.legs.Put(leg)
	  plan, _ := s.plans.Get(leg.PlanID)
	   if leg.HasResidue() {
		   if nextPlan, err2 := AdvancePlan(plan.State, EvResidueFound); err2 == nil {
			   plan.State = nextPlan
			  plan.UpdatedAt = now
			   s.plans.Put(plan)
		  }
	   }
	   return leg, plan, nil
  }

// ─────────────────────────────────────────────
// ResumeLeg / RollbackLeg / SuspendLeg
// ─────────────────────────────────────────────
// ResumeLeg 将 failed/suspended 腿续跑至 executing。
  func (s *Service) ResumeLeg(legID string) (SeizeLeg, error) {
	  leg, ok := s.legs.Get(legID)
	  if !ok {
		  return SeizeLeg{}, ErrLegNotFound
	  }
	  next, err := AdvanceLeg(leg.State, EvLegResume)
	  if err != nil {
		  return leg, err
	   }
	  leg.State = next
	   leg.UpdatedAt = s.clock()
	   s.legs.Put(leg)
	   return leg, nil
  }

// RollbackLeg 将 failed 腿回滚至 rolled，表示残留已清。
   func (s *Service) RollbackLeg(legID string) (SeizeLeg, LiqPlan, error) {
	  leg, ok := s.legs.Get(legID)
	  if !ok {
		  return SeizeLeg{}, LiqPlan{}, ErrLegNotFound
	  }
	   next, err := AdvanceLeg(leg.State, EvLegRollback)
	   if err != nil {
		  return leg, LiqPlan{}, err
	  }
	  now := s.clock()
	  leg.State = next
	   leg.RolledAt = now
	   leg.UpdatedAt = now
	   s.legs.Put(leg)
	   plan, _ := s.plans.Get(leg.PlanID)
	   if !s.legs.HasResidue(plan.ID) {
		  if nextPlan, err2 := AdvancePlan(plan.State, EvResidueCleared); err2 == nil {
			   plan.State = nextPlan
			   plan.UpdatedAt = now
			  s.plans.Put(plan)
		   }
	  }
	   return leg, plan, nil
   }

// SuspendLeg 将 failed 腿挂账，等人工干预。

  func (s *Service) SuspendLeg(legID, note string) (SeizeLeg, error) {
	   leg, ok := s.legs.Get(legID)
	   if !ok {
		  return SeizeLeg{}, ErrLegNotFound
	  }
	   next, err := AdvanceLeg(leg.State, EvLegSuspend)
	   if err != nil {
		  return leg, err
	  }
	   now := s.clock()
	  leg.State = next
	   leg.SuspendNote = note
	   leg.SuspendedAt = now
	   leg.UpdatedAt = now
	  s.legs.Put(leg)
	   return leg, nil
  }

// ─────────────────────────────────────────────
// SettleBorrow：清偿借项
// ─────────────────────────────────────────────
// SettleBorrow 执行借项清偿，将计划推进至 closed（或 residued）。
// 接受 seizing 或 settling 状态：若当前为 seizing，先推进至 settling，再推进至 closed。
   func (s *Service) SettleBorrow(planID, settledAmount string) (LiqPlan, error) {
	  plan, ok := s.plans.Get(planID)
	   if !ok {
		   return LiqPlan{}, ErrNotFound
	   }
	   if plan.State != PlanSeizing && plan.State != PlanSettling {
		   return plan, ErrStateIllegal
	   }
	  hasResidue := s.legs.HasResidue(planID)
	  now := s.clock()
	   // 若仍在 seizing，先推进至 settling
	  if plan.State == PlanSeizing {
		  next, err := AdvancePlan(plan.State, EvSeizeDone)
		  if err != nil {
			   return plan, err
		  }
		  plan.State = next
		   plan.UpdatedAt = now
		  s.plans.Put(plan)
	   }
	  if hasResidue {
		   if nr, err2 := AdvancePlan(plan.State, EvResidueFound); err2 == nil {
			   plan.State = nr
			   plan.UpdatedAt = now
			   s.plans.Put(plan)
		   }
		   return plan, nil
	   }
	   nr,
		  err := AdvancePlan(plan.State, EvSettleDone)
	  if err != nil {
		  return plan, err
	  }
	  plan.State = nr
	   plan.UpdatedAt = now
	   s.plans.Put(plan)
	  s.occupy.Release(plan.BorrowID)
	  return plan, nil
  }

// ─────────────────────────────────────────────
// AbortPlan：双人中止（仅 triggered 态）
// ─────────────────────────────────────────────
// AbortReq 双人授权中止请求。Actor1/Actor2 不得相同。
type AbortReq struct {
	PlanID string
	Actor1 string
	Actor2 string
	Reason string
}

// AbortPlan 双人授权中止强平计划（仅允许 triggered 态）。
   func (s *Service) AbortPlan(req AbortReq) (LiqPlan, error) {
	   if s == nil {
		   return LiqPlan{}, ErrUnavailable
	   }
	  if req.Actor1 == "" || req.Actor2 == "" || req.Actor1 == req.Actor2 {
		   return LiqPlan{},
			  ErrAbortUnauthorized
	  }
	   plan, ok := s.plans.Get(req.PlanID)
	   if !ok {
		   return LiqPlan{}, ErrNotFound
	  }
	   if plan.State != PlanTriggered {
		   return plan, ErrAbortForbidden
	  }
	  next, err := AdvancePlan(plan.State, EvAbort)
	   if err != nil {
		   return plan, err
	  }
	  now := s.clock()
	  plan.State = next
	   plan.AbortedBy = req.Actor1 + "+" + req.Actor2
	  plan.AbortedAt = now
	   plan.AbortReason = req.Reason
	  plan.UpdatedAt = now
	  s.plans.Put(plan)
	  s.occupy.Release(plan.BorrowID)
	   return plan, nil
   }

// ─────────────────────────────────────────────
// BuildProof / ValidateProof
// ─────────────────────────────────────────────
// BuildProof 生成强平终态证明（closed 或 residued 状态下调用）。

  func (s *Service) BuildProof(planID string) (LiqProof, error) {
	  if s == nil {
		  return LiqProof{}, ErrUnavailable
	  }
	   if prev, ok := s.proofs.ByPlan(planID); ok {
		   return prev, nil
	   }
	  plan, ok := s.plans.Get(planID)
	  if !ok {
		  return LiqProof{}, ErrNotFound
	  }
	   if plan.State != PlanClosed && plan.State != PlanResidued {
		  return LiqProof{}, fmt.Errorf("%w: plan not in closed/residued state: %s", ErrStateIllegal, plan.State)
	  }
	  legs := s.legs.ByPlan(planID)
	   var seizedLegs []SeizeLegSummary
	  var residueLegs []ResidueLegSummary
	  totalSeized := decimal.Zero
	   totalResidue := decimal.Zero
	  for _, l := range legs {
		   seizedAmt := decimal.Zero
		  if l.PartialAmount != "" && l.PartialAmount != "0" {
			   if d, err := decimal.NewFromString(l.PartialAmount); err == nil {
				  seizedAmt = d
			  }
		  }
		  seizedLegs = append(seizedLegs, SeizeLegSummary{
			  LegID: l.ID,
			  Asset:        l.Asset,
			   TargetAmount: l.TargetAmount,
			   SeizedAmount: l.PartialAmount,
			  State: l.State,
		   })
		  if l.State == LegDone {
			  totalSeized = totalSeized.Add(seizedAmt)
		   }
		   if l.HasResidue() {
			  residueLegs = append(residueLegs, ResidueLegSummary{
				  LegID: l.ID,
				  Asset:         l.Asset,
				  PartialAmount: l.PartialAmount,
				   State:         l.State,
			  })
			  totalResidue = totalResidue.Add(seizedAmt)
		   }
	  }
	   proof := LiqProof{
		   ID:              uuid.NewString(),
		  PlanID:          planID,
		   OccupyToken:     plan.OccupyToken,
		  CollateralAsset: plan.CollateralAsset,
		  CollateralAmount: plan.CollateralAmount,
		  SeizedLegs: seizedLegs,
		   TotalSeized: totalSeized.String(),
		   BorrowAsset:  plan.BorrowAsset,
		  BorrowAmount: plan.BorrowAmount,
		  SettledAmount: totalSeized.String(),
		   ResidueLegs:  residueLegs,
		  TotalResidue: totalResidue.String(),
		  Status:       ProofDraft,
		  CreatedAt:    s.clock(),
	  }
	   digest, err := proofDigest(proof)
	   if err != nil {
		   return LiqProof{}, fmt.Errorf("proof digest: %w", err)
	   }
	   proof.Digest = digest
	   s.proofs.Put(proof)
	   plan.ProofID = proof.ID
	  plan.UpdatedAt = s.clock()
	  s.plans.Put(plan)
	   return proof, nil
  }

// ValidateProof 双人校验强平证明（draft → validated）。

   func (s *Service) ValidateProof(proofID, actor1, actor2 string) (LiqProof, error) {
	  if actor1 == "" || actor2 == "" || actor1 == actor2 {
		  return LiqProof{}, ErrAbortUnauthorized
	   }
	   proof, ok := s.proofs.Get(proofID)
	  if !ok {
		   return LiqProof{}, ErrProofNotFound
	   }
	   expected, err := proofDigest(proof)
	  if err != nil {
		  return proof, fmt.Errorf("re-digest: %w", err)
	  }
	  if expected != proof.Digest {
		  return proof, ErrProofInvalid
	  }
	  proof.Status = ProofValidated
	   proof.ValidatedBy = actor1 + "+" + actor2
	  proof.ValidatedAt = s.clock()
	  s.proofs.Put(proof)
	  return proof, nil
   }

// ─────────────────────────────────────────────
// MarkGate 管理
// ─────────────────────────────────────────────
// RegisterMark 注册一个标价世代快照（供或外部调用）。

  func (s *Service) RegisterMark(g MarkGate) {
	  if g.ID == "" {
		   g.ID = uuid.NewString()
	   }
	  if g.CreatedAt.IsZero() {
		  g.CreatedAt = s.clock()
	   }
	   if g.MaxAgeSeconds == 0 {
		  g.MaxAgeSeconds = s.markMaxAgeSeconds
	   }
	   s.marks.Put(g)
   }

// CheckMark 检查给定 markGateID 是否仍有效。

   func (s *Service) CheckMark(markGateID string) error {
	   g, ok := s.marks.Get(markGateID)
	  if !ok {
		  return ErrMarkNotFound
	   }
	  if g.IsStale(s.clock()) {
		   return ErrMarkStale
	  }
	  return nil
  }

// PoisonMark 将某标价世代标记为中毒（禁止用于新触发）。
  func (s *Service) PoisonMark(markGateID, note string) error {
	   g, ok := s.marks.Get(markGateID)
	   if !ok {
		  return ErrMarkNotFound
	  }
	  g.Poisoned = true
	   g.PoisonNote = note
	  s.marks.Put(g)
	   return nil
   }

// ─────────────────────────────────────────────
// 查询接口
// ─────────────────────────────────────────────

  func (s *Service) GetPlan(planID string) (LiqPlan, error) {
	  p, ok := s.plans.Get(planID)
	   if !ok {
		  return LiqPlan{}, ErrNotFound
	   }
	   return p, nil
   }

  func (s *Service) GetPlanByBorrow(borrowID string) (LiqPlan, error) {
	  p, ok := s.plans.ByBorrow(borrowID)
	   if !ok {
		   return LiqPlan{}, ErrNotFound
	   }
	  return p, nil
  }

  func (s *Service) ListPlans(state string) []LiqPlan {
	   if state == "" {
		  return s.plans.List()
	  }
	  return s.plans.ListByState(PlanState(state))
  }

  func (s *Service) ListByUser(userID string) []LiqPlan {
	  return s.plans.ListByUser(userID)
  }

  func (s *Service) ListLegs(planID string) []SeizeLeg {
	  return s.legs.ByPlan(planID)
   }

   func (s *Service) GetProof(proofID string) (LiqProof, error) {
	  p, ok := s.proofs.Get(proofID)
	  if !ok {
		  return LiqProof{}, ErrProofNotFound
	   }
	  return p, nil
   }

  func (s *Service) GetProofByPlan(planID string) (LiqProof, error) {
	   p, ok := s.proofs.ByPlan(planID)
	   if !ok {
		   return LiqProof{}, ErrProofNotFound
	  }
	  return p, nil
  }

// ─────────────────────────────────────────────
// 内部辅助
// ─────────────────────────────────────────────

   func (s *Service) maybeAdvancePlanAfterLeg(plan LiqPlan) LiqPlan {
	  if plan.State != PlanSeizing {
		   return plan
	   }
	   legs := s.legs.ByPlan(plan.ID)
	   if len(legs) == 0 {
		   return plan
	  }
	  for _, l := range legs {
		  if l.State != LegDone {
			  return plan
		  }
	   }
	  now := s.clock()
	   if next, err := AdvancePlan(plan.State, EvSeizeDone); err == nil {
		  plan.State = next
		  plan.UpdatedAt = now
		   s.plans.Put(plan)
	  }
	   return plan
   }

// proofDigest 计算 LiqProof 的 SHA256 摘要（排除 Digest/Status/ValidatedBy/ValidatedAt 字段）。

  func proofDigest(p LiqProof) (string, error) {
	   tmp := p
	   tmp.Digest = ""
	   tmp.Status = ""
	  tmp.ValidatedBy = ""
	  tmp.ValidatedAt = time.Time{}
	  data, err := json.Marshal(tmp)
	   if err != nil {
		   return "", err
	   }
	   h := sha256.Sum256(data)
	   return fmt.Sprintf("%x", h), nil
   }
