package dispclaw

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Service 争议资金追偿服务（内存后端；单元与流程复用）。
// 部署中应由带 SQL 持久化的适配器包装本接口。
type Service struct {
	mu    sync.Mutex
	plans map[string]*FundPlan // plan_id → FundPlan
	legs map[string]*RecoveryLeg // leg_id → RecoveryLeg
	gates map[string]*CloseGate // gate_id → CloseGate
	proofs      map[string]*CloseProof // proof_id → CloseProof
	byCase      map[string]string      // case_id → plan_id
	legsByPlan  map[string][]string    // plan_id → []leg_id（有序）
	gateByCase  map[string]string      // case_id → gate_id
	proofByCase map[string]string      // case_id → proof_id
	byIdem      map[string]string      // idempotency_key → plan_id
	clock func() time.Time
}

// New 构造内存后端服务。

  func New() *Service {
	  return &Service{
		  plans: make(map[string]*FundPlan),
		   legs:  make(map[string]*RecoveryLeg),
		   gates: make(map[string]*CloseGate),
		  proofs:      make(map[string]*CloseProof),
		  byCase:      make(map[string]string),
		   legsByPlan:  make(map[string][]string),
		   gateByCase:  make(map[string]string),
		  proofByCase: make(map[string]string),
		   byIdem:      make(map[string]string),
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	  return s.clock()
   }

// SetClock 注入可控时钟（用）。
   func (s *Service) SetClock(fn func() time.Time) {
	   if s != nil {
		   s.clock = fn
	  }
  }

  // ─────────────────────────── 计划派生 ───────────────────────────
  // GeneratePlan 由 Case 信息派生 FundPlan 及追偿腿骨架。
  // 不变式：
   // - 一个 Case 只能有一条 FundPlan（唯一键保证）。
  // - 计划源必须是 dispute Case ID（不接受运营批次号作为来源）。
   // - 腿骨架由 planType 决定：claw_back 生成 freeze+claw_debit；
  // refund 生成 freeze+refund_credit；restore 生成 restore_unfreeze。

  func (s *Service) GeneratePlan(req GeneratePlanReq, caseUserID, caseChannel, caseAsset, caseAmount, openedBy string) (FundPlan, []RecoveryLeg, error) {
	  if s == nil {
		  return FundPlan{}, nil, ErrUnavailable
	   }
	   caseID := strings.TrimSpace(req.CaseID)
	   if caseID == "" {
		   return FundPlan{}, nil, ErrCase
	   }
	  pt, err := ParsePlanType(string(req.PlanType))
	  if err != nil {
		   return FundPlan{}, nil, fmt.Errorf("%w: %v", ErrState, err)
	   }
	  actor, err := validate.Actor(req.Actor)
	  if err != nil {
		  return FundPlan{}, nil, ErrActor
	  }
	  idem := strings.TrimSpace(req.IdempotencyKey)
	  if idem == "" {
		   return FundPlan{}, nil, ErrIdem
	  }
	   asset, err := validate.Asset(caseAsset)
	   if err != nil {
		   return FundPlan{}, nil, ErrAsset
	  }
	  amt, err := decimal.NewFromString(strings.TrimSpace(caseAmount))
	   if err != nil || amt.LessThanOrEqual(decimal.Zero) {
		  return FundPlan{},
			  nil, ErrAmount
	   }
	   uid := strings.TrimSpace(caseUserID)
	  if uid == "" {
		   return FundPlan{}, nil, ErrCase
	  }
	  opener := strings.TrimSpace(openedBy)
	   if opener == "" {
		   opener = actor
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  // 幂等：相同 idempotency_key 返回已有计划。
	  if pid, ok := s.byIdem[idem]; ok {
		   plan := s.plans[pid]
		   legs := s.legsOfLocked(pid)
		  return plan.Copy(),
			   copyLegs(legs), nil
	   }
	   // 唯一性：一个 Case 只允许一条计划。
	   if _, ok := s.byCase[caseID]; ok {
		   return FundPlan{}, nil, ErrPlanExists
	   }
	   now := s.now()
	  planID := uuid.NewString()
	   railBreakID := strings.TrimSpace(req.RailBreakID) // 只读引用，不写 fiatbreak 检测器
	   plan := &FundPlan{
		   ID:      planID,
		   CaseID:  caseID,
		  Channel: strings.TrimSpace(caseChannel),
		  UserID: uid,
		   Asset:  asset,
		  Amount: amt.String(),
		   PlanType: pt,
		   Status:   PlanStatusOpen,
		  RailBreakID: railBreakID,
		  OpenedBy:    opener,
		   Version:     1,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   s.plans[planID] = plan
	   s.byCase[caseID] = planID
	   s.byIdem[idem] = planID
	   // 生成腿骨架。
	  bones := buildLegBones(planID, caseID, uid, asset, amt.String(), pt)
	   createdLegs := make([]*RecoveryLeg, 0, len(bones))
	  for _, b := range bones {
		  leg := b
		   h := ComputeLegHash(LegHashInput{
			   PlanID:  leg.PlanID,
			   Seq:     leg.Seq,
			   LegType: leg.LegType,
			   UserID:  leg.UserID,
			  Asset: leg.Asset,
			  Amount: leg.Amount,
			  Status: leg.Status,
		  })
		  leg.LegHash = h
		   leg.CreatedAt = now
		  leg.UpdatedAt = now
		   lid := uuid.NewString()
		  leg.ID = lid
		   s.legs[lid] = &leg
		  s.legsByPlan[planID] = append(s.legsByPlan[planID],
			  lid)
		   createdLegs = append(createdLegs, &leg)
	  }
	  // 建立初始收口闸（blocking）。
	  pendingIDs := make([]string, 0, len(createdLegs))
	   for _, l := range createdLegs {
		   if l.Status == LegStatusPending {
			   pendingIDs = append(pendingIDs, l.ID)
		   }
	  }
	  gateID := uuid.NewString()
	   gate := &CloseGate{
		   ID:     gateID,
		   PlanID: planID,
		  CaseID:     caseID,
		  GateStatus: GateStatusBlocking,
		  BlockingReason: fmt.Sprintf("plan %s created with %d pending leg(s)", planID, len(pendingIDs)),
		   PendingLegIDs: strings.Join(pendingIDs, ","),
		  Version:       1,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	  s.gates[gateID] = gate
	  s.gateByCase[caseID] = gateID
	   resultLegs := make([]RecoveryLeg, 0, len(createdLegs))
	   for _, l := range createdLegs {
		  resultLegs = append(resultLegs, l.Copy())
	   }
	  return plan.Copy(), resultLegs, nil
  }

// ─────────────────────────── 腿执行（四眼） ───────────────────────────
// ExecuteLeg 执行单条追偿腿（需四眼：执行人不得与开单人相同）。
// 不变式：
// - 腿必须在 pending 状态。
// - 执行人（actor）不得与 plan.OpenedBy 相同。
// - 腿推进到 posted 后，更新 LegHash（状态已含在哈希中）。
   func (s *Service) ExecuteLeg(req ExecuteLegReq) (RecoveryLeg, error) {
	   if s == nil {
		   return RecoveryLeg{},
			  ErrUnavailable
	  }
	   actor, err := validate.Actor(req.Actor)
	   if err != nil {
		  return RecoveryLeg{}, ErrActor
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  plan,
		   ok := s.plans[strings.TrimSpace(req.PlanID)]
	   if !ok {
		   return RecoveryLeg{}, ErrPlan
	   }
	  if plan.Status.Terminal() {
		   return RecoveryLeg{}, ErrState
	  }
	   // 四眼校验：执行人不得为开单人。
	  if strings.EqualFold(actor, plan.OpenedBy) {
		  return RecoveryLeg{}, ErrFourEye
	   }
	   leg, ok := s.legs[strings.TrimSpace(req.LegID)]
	   if !ok {
		   return RecoveryLeg{}, ErrLeg
	  }
	   if leg.PlanID != plan.ID {
		   return RecoveryLeg{}, ErrLeg
	  }
	  if leg.Status != LegStatusPending {
		   return leg.Copy(), ErrLegNotPending
	  }
	   now := s.now()
	  leg.Status = LegStatusPosted
	   leg.PostedBy = actor
	  leg.Note = strings.TrimSpace(req.Note)
	   t := now
	   leg.PostedAt = &t
	   // 腿状态已变，重新计算哈希。
	   leg.LegHash = ComputeLegHash(LegHashInput{
		  PlanID: leg.PlanID,
		   Seq:     leg.Seq,
		   LegType: leg.LegType,
		  UserID:  leg.UserID,
		   Asset:  leg.Asset,
		  Amount: leg.Amount,
		  Status: leg.Status,
	  })
	  leg.UpdatedAt = now
	   // 若计划状态仍为 open，切换到 executing。
	   if plan.Status == PlanStatusOpen {
		  plan.Status = PlanStatusExecuting
		  plan.UpdatedAt = now
	   }
	   return leg.Copy(), nil
  }

// MarkLegFailed 将腿标记为失败（worker 探测到异常时调用）。
   func (s *Service) MarkLegFailed(legID, reason string, nextRetry *time.Time) (RecoveryLeg, error) {
	  if s == nil {
		  return RecoveryLeg{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  leg, ok := s.legs[strings.TrimSpace(legID)]
	  if !ok {
		   return RecoveryLeg{}, ErrLeg
	   }
	  if leg.Status.Terminal() {
		   return leg.Copy(), nil
	   }
	   now := s.now()
	  leg.Status = LegStatusFailed
	   t := now
	  leg.FailedAt = &t
	   leg.FailReason = strings.TrimSpace(reason)
	   leg.RetryCount++
	   leg.NextRetryAt = nextRetry
	   leg.LegHash = ComputeLegHash(LegHashInput{
		  PlanID: leg.PlanID,
		   Seq: leg.Seq,
		   LegType: leg.LegType,
		   UserID: leg.UserID,
		   Asset:  leg.Asset,
		  Amount: leg.Amount,
		  Status: leg.Status,
	  })
	   leg.UpdatedAt = now
	   return leg.Copy(), nil
  }

// SkipLeg 将腿标记为跳过（条件不满足时调用）。
   func (s *Service) SkipLeg(legID, reason string) (RecoveryLeg, error) {
	  if s == nil {
		  return RecoveryLeg{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   leg, ok := s.legs[strings.TrimSpace(legID)]
	   if !ok {
		  return RecoveryLeg{}, ErrLeg
	   }
	   if leg.Status.Terminal() {
		   return leg.Copy(), nil
	  }
	   now := s.now()
	   leg.Status = LegStatusSkipped
	  leg.Note = strings.TrimSpace(reason)
	  leg.LegHash = ComputeLegHash(LegHashInput{
		   PlanID: leg.PlanID,
		  Seq: leg.Seq,
		  LegType: leg.LegType,
		  UserID:  leg.UserID,
		   Asset:   leg.Asset,
		   Amount: leg.Amount,
		   Status: leg.Status,
	  })
	  leg.UpdatedAt = now
	   return leg.Copy(), nil
  }

// ─────────────────────────── 收口闸 ───────────────────────────
// CheckCloseGate 检查收口闸状态，刷新并决定是否放行 Case 结案。
// 不变式：
// - 若存在任何 pending/posting 腿，闸状态保持 blocking。
// - 所有腿为终态（posted/failed/skipped）时，闸自动切换到 passed。
// - passed 后调用方可以调用 Case.ResolveFourEye。
  func (s *Service) CheckCloseGate(req CloseGateCheckReq) (CloseGate, error) {
	   if s == nil {
		  return CloseGate{}, ErrUnavailable
	   }
	   caseID := strings.TrimSpace(req.CaseID)
	   actor, err := validate.Actor(req.Actor)
	  if err != nil {
		  return CloseGate{}, ErrActor
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   planID, ok := s.byCase[caseID]
	   if !ok {
		   return CloseGate{}, ErrPlan
	   }
	   gateID, ok := s.gateByCase[caseID]
	  if !ok {
		   return CloseGate{}, ErrGate
	   }
	   gate := s.gates[gateID]
	   if gate.GateStatus == GateStatusPassed || gate.GateStatus == GateStatusBypassed {
		  return gate.Copy(), nil
	  }
	   // 重新扫描所有腿的状态。
	   pendingIDs := make([]string, 0)
	   for _, lid := range s.legsByPlan[planID] {
		  l := s.legs[lid]
		   if !l.Status.Terminal() {
			  pendingIDs = append(pendingIDs, l.ID)
		   }
	  }
	  now := s.now()
	   gate.CheckedBy = actor
	   gate.UpdatedAt = now
	   if len(pendingIDs) == 0 {
		   gate.GateStatus = GateStatusPassed
		   gate.BlockingReason = ""
		   gate.PendingLegIDs = ""
		  t := now
		   gate.PassedAt = &t
	   } else {
		  gate.GateStatus = GateStatusBlocking
		  gate.BlockingReason = fmt.Sprintf("%d pending leg(s) remain", len(pendingIDs))
		  gate.PendingLegIDs = strings.Join(pendingIDs, ",")
	  }
	   gate.Version++
	   return gate.Copy(), nil
  }

// BypassCloseGate 管理员特批豁免收口闸（需提供理由；将留审计记录）。
   func (s *Service) BypassCloseGate(req BypassGateReq) (CloseGate, error) {
	  if s == nil {
		   return CloseGate{}, ErrUnavailable
	   }
	  caseID := strings.TrimSpace(req.CaseID)
	   actor, err := validate.Actor(req.Actor)
	   if err != nil {
		  return CloseGate{}, ErrActor
	  }
	  reason := strings.TrimSpace(req.BypassReason)
	  if reason == "" {
		   return CloseGate{}, ErrBypassReason
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   gateID,
		  ok := s.gateByCase[caseID]
	  if !ok {
		   return CloseGate{}, ErrGate
	  }
	   gate := s.gates[gateID]
	  if gate.GateStatus == GateStatusPassed {
		   return gate.Copy(), nil
	   }
	   now := s.now()
	  gate.GateStatus = GateStatusBypassed
	   gate.BypassReason = reason
	  gate.BypassedBy = actor
	   gate.UpdatedAt = now
	   gate.Version++
	  return gate.Copy(), nil
  }

// ─────────────────────────── 收口证明 ───────────────────────────
// GenerateCloseProof 在收口闸 passed/bypassed 后生成收口证明。
// 不变式：
// - 必须在收口闸放行后才能调用。
// - ProofHash = SHA256(各腿 leg_hash 有序拼接)。
// - 一个 Case 只能有一个 CloseProof（幂等）。
  func (s *Service) GenerateCloseProof(caseID, actor, decision string) (CloseProof, error) {
	   if s == nil {
		   return CloseProof{}, ErrUnavailable
	  }
	   caseID = strings.TrimSpace(caseID)
	   act, err := validate.Actor(actor)
	   if err != nil {
		  return CloseProof{}, ErrActor
	   }
	   decision = strings.ToLower(strings.TrimSpace(decision))
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  // 幂等：已有证明直接返回。
	   if pid, ok := s.proofByCase[caseID]; ok {
		  return s.proofs[pid].Copy(), nil
	  }
	   planID, ok := s.byCase[caseID]
	  if !ok {
		   return CloseProof{}, ErrPlan
	   }
	   gateID, ok := s.gateByCase[caseID]
	   if !ok {
		  return CloseProof{}, ErrGate
	  }
	   gate := s.gates[gateID]
	  if gate.GateStatus == GateStatusBlocking {
		  return CloseProof{}, ErrGateBlocking
	   }
	   plan := s.plans[planID]
	  legs := s.legsOfLocked(planID)
	  // 收集腿哈希列表（按 seq 排序）。
	   hashes := make([]string, 0, len(legs))
	   totalAmt := decimal.Zero
	  for _, l := range legs {
		  hashes = append(hashes, l.LegHash)
		  amt, _ := decimal.NewFromString(l.Amount)
		   totalAmt = totalAmt.Add(amt)
	  }
	   proofHash := ComputeProofHash(hashes)
	   hashesJSON,
		  err := MarshalLegHashes(hashes)
	   if err != nil {
		   return CloseProof{}, fmt.Errorf("dispclaw: marshal leg hashes: %w", err)
	  }
	  now := s.now()
	   proofID := uuid.NewString()
	  proof := &CloseProof{
		  ID: proofID,
		  PlanID: planID,
		   CaseID: caseID,
		  GateID:    gateID,
		  ProofHash: proofHash,
		   LegHashes: hashesJSON,
		   LegCount: len(legs),
		  TotalAmount: totalAmt.String(),
		   Asset:       plan.Asset,
		  DecidedBy:   act,
		   Decision:    decision,
		   CreatedAt: now,
	   }
	  s.proofs[proofID] = proof
	   s.proofByCase[caseID] = proofID
	   // 关闭计划。
	   plan.Status = PlanStatusClosed
	  plan.ClosedBy = act
	  plan.UpdatedAt = now
	   return proof.Copy(), nil
   }

// PatrolProof 巡检收口证明哈希，检测是否被篡改。
// 返回 ErrProofTamper 时已在 proof.TamperDetected 置位。
  func (s *Service) PatrolProof(caseID string) (CloseProof, error) {
	  if s == nil {
		  return CloseProof{}, ErrUnavailable
	   }
	  caseID = strings.TrimSpace(caseID)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   proofID, ok := s.proofByCase[caseID]
	  if !ok {
		  return CloseProof{}, ErrProof
	   }
	  proof := s.proofs[proofID]
	  planID, ok := s.byCase[caseID]
	   if !ok {
		  return proof.Copy(), ErrPlan
	  }
	  legs := s.legsOfLocked(planID)
	   legValues := make([]RecoveryLeg, len(legs))
	  for i, l := range legs {
		  legValues[i] = *l
	  }
	   now := s.now()
	   t := now
	  proof.TamperCheckedAt = &t
	  if err := VerifyProof(*proof, legValues); err != nil {
		   proof.TamperDetected = true
		  proof.TamperDetail = fmt.Sprintf("proof_hash=%s recomputed mismatch at %s", proof.ProofHash, now.Format(time.RFC3339))
		   return proof.Copy(), ErrProofTamper
	  }
	   proof.TamperDetected = false
	  proof.TamperDetail = ""
	  return proof.Copy(), nil
  }

// ─────────────────────────── 中止计划 ───────────────────────────
// AbortPlan 中止资金计划（审批拒绝或超时升级时调用）。
  func (s *Service) AbortPlan(req AbortPlanReq) (FundPlan, error) {
	   if s == nil {
		  return FundPlan{}, ErrUnavailable
	  }
	  actor, err := validate.Actor(req.Actor)
	   if err != nil {
		  return FundPlan{}, ErrActor
	   }
	   reason := strings.TrimSpace(req.AbortReason)
	   if reason == "" {
		  return FundPlan{}, ErrAbortReason
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   plan,
		  ok := s.plans[strings.TrimSpace(req.PlanID)]
	   if !ok {
		  return FundPlan{}, ErrPlan
	   }
	   if plan.Status.Terminal() {
		  return plan.Copy(), ErrState
	   }
	   now := s.now()
	  plan.Status = PlanStatusAborted
	  plan.ClosedBy = actor
	  plan.AbortReason = reason
	   plan.UpdatedAt = now
	   // 未完成的腿一并跳过。
	  for _, lid := range s.legsByPlan[plan.ID] {
		  l := s.legs[lid]
		   if !l.Status.Terminal() {
			   l.Status = LegStatusSkipped
			  l.Note = "plan aborted: " + reason
			   l.LegHash = ComputeLegHash(LegHashInput{
				  PlanID: l.PlanID,
				   Seq: l.Seq,
				  LegType: l.LegType,
				  UserID: l.UserID,
				   Asset:  l.Asset,
				   Amount: l.Amount,
				   Status: l.Status,
			  })
			   l.UpdatedAt = now
		   }
	  }
	   return plan.Copy(), nil
   }

// ─────────────────────────── 查询 ───────────────────────────
// GetPlanByCase 按 Case ID 查询 FundPlan。
  func (s *Service) GetPlanByCase(caseID string) (FundPlan, error) {
	   if s == nil {
		   return FundPlan{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   pid, ok := s.byCase[strings.TrimSpace(caseID)]
	   if !ok {
		   return FundPlan{}, ErrPlan
	   }
	   return s.plans[pid].Copy(), nil
   }

// GetPlan 按 plan_id 查询 FundPlan。

   func (s *Service) GetPlan(planID string) (FundPlan, error) {
	  if s == nil {
		  return FundPlan{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  p, ok := s.plans[strings.TrimSpace(planID)]
	   if !ok {
		   return FundPlan{}, ErrPlan
	   }
	  return p.Copy(), nil
   }

// ListLegs 查询 FundPlan 下所有追偿腿（按 seq 有序）。
   func (s *Service) ListLegs(planID string) []RecoveryLeg {
	   if s == nil {
		   return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   return copyLegs(s.legsOfLocked(strings.TrimSpace(planID)))
  }

// GetGateByCaseID 按 case_id 查询 CloseGate。
  func (s *Service) GetGateByCaseID(caseID string) (CloseGate, error) {
	  if s == nil {
		   return CloseGate{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  gid, ok := s.gateByCase[strings.TrimSpace(caseID)]
	   if !ok {
		   return CloseGate{}, ErrGate
	  }
	   return s.gates[gid].Copy(), nil
  }

// GetProofByCaseID 按 case_id 查询 CloseProof。

  func (s *Service) GetProofByCaseID(caseID string) (CloseProof, error) {
	  if s == nil {
		   return CloseProof{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  pid, ok := s.proofByCase[strings.TrimSpace(caseID)]
	   if !ok {
		   return CloseProof{}, ErrProof
	   }
	   return s.proofs[pid].Copy(), nil
   }

// ListAllPlans 枚举所有计划（worker 用）。

   func (s *Service) ListAllPlans() []FundPlan {
	   if s == nil {
		   return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]FundPlan, 0, len(s.plans))
	   for _, p := range s.plans {
		  out = append(out, p.Copy())
	  }
	   return out
  }

// ListAllProofs 枚举所有证明（巡检 worker 用）。

   func (s *Service) ListAllProofs() []CloseProof {
	   if s == nil {
		   return nil
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]CloseProof, 0, len(s.proofs))
	   for _, p := range s.proofs {
		  out = append(out, p.Copy())
	   }
	   return out
  }

// ─────────────────────────── 内部辅助 ───────────────────────────
// legsOfLocked 返回计划下所有腿（调用前必须持锁）。按 seq 排序。

  func (s *Service) legsOfLocked(planID string) []*RecoveryLeg {
	   ids := s.legsByPlan[planID]
	   out := make([]*RecoveryLeg, 0, len(ids))
	  for _, id := range ids {
		   if l, ok := s.legs[id]; ok {
			   out = append(out, l)
		   }
	   }
	   // 按 seq 排序（ids 本身按插入顺序，插入即按 seq 递增，此处为安全起见显式排序）。
	   for i := 1; i < len(out); i++ {
		  for j := i; j > 0 && out[j].Seq < out[j-1].Seq; j-- {
			  out[j], out[j-1] = out[j-1], out[j]
		  }
	  }
	   return out
   }

  func copyLegs(legs []*RecoveryLeg) []RecoveryLeg {
	  out := make([]RecoveryLeg, len(legs))
	   for i, l := range legs {
		  out[i] = l.Copy()
	   }
	  return out
  }

// buildLegBones 按计划类型生成腿骨架（仅设置固定字段，ID 由调用方赋值）。

  func buildLegBones(planID, caseID, userID, asset, amount string, pt PlanType) []RecoveryLeg {
	   idemBase := planID
	  switch pt {
	   case PlanTypeClawBack:
		   // 追偿路径：先冻结，再借记追偿。
		  return []RecoveryLeg{
			   {
				   PlanID:  planID,
				   CaseID:  caseID,
				  Seq:     1,
				  LegType: LegTypeFreeze,
				  UserID:  userID,
				  Asset:   asset,
				  Amount: amount,
				   Status:         LegStatusPending,
				  IdempotencyKey: idemBase + "|1|freeze",
			   },
			  {
				   PlanID: planID,
				   CaseID: caseID,
				  Seq:            2,
				  LegType:        LegTypeClawDebit,
				   UserID:         userID,
				  Asset:          asset,
				   Amount:         amount,
				   Status:         LegStatusPending,
				  IdempotencyKey: idemBase + "|2|claw_debit",
			  },
		   }
	   case PlanTypeRefund:
		   // 退还路径：先冻结，再贷记退还。
		  return []RecoveryLeg{
			   {
				   PlanID: planID,
				  CaseID: caseID,
				   Seq:    1,
				  LegType: LegTypeFreeze,
				  UserID:         userID,
				  Asset:          asset,
				  Amount:         amount,
				   Status:         LegStatusPending,
				   IdempotencyKey: idemBase + "|1|freeze",
			   },
			   {
				   PlanID: planID,
				  CaseID: caseID,
				   Seq:     2,
				   LegType: LegTypeRefundCredit,
				   UserID:  userID,
				  Asset: asset,
				  Amount:         amount,
				  Status:         LegStatusPending,
				  IdempotencyKey: idemBase + "|2|refund_credit",
			   },
		   }
	   case PlanTypeRestore:
		  // 恢复路径：直接解冻。
		  return []RecoveryLeg{
			   {
				  PlanID:  planID,
				  CaseID:  caseID,
				   Seq:     1,
				  LegType: LegTypeRestoreUnfreeze,
				   UserID: userID,
				  Asset: asset,
				  Amount:         amount,
				  Status:         LegStatusPending,
				   IdempotencyKey: idemBase + "|1|restore_unfreeze",
			   },
		   }
	  default:
		  return nil
	   }
   }

// MarshalPlan 将 FundPlan 序列化为 JSON（持久化用）。
   func MarshalPlan(p FundPlan) ([]byte, error) {
	  return json.Marshal(p)
   }

// UnmarshalPlan 从 JSON 反序列化 FundPlan。

  func UnmarshalPlan(b []byte) (FundPlan, error) {
	   var p FundPlan
	  if err := json.Unmarshal(b, &p); err != nil {
		  return FundPlan{}, err
	   }
	  return p, nil
  }
