package p2pabandon

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Service 弃单收口核心服务（内存后端；环境可由 SQL 适配器替换接口）。

type Service struct {
	mu sync.Mutex
	// 主索引
	plans  map[string]*AbandonPlan // plan_id → AbandonPlan
	tokens map[string]*MutexToken  // token_id → MutexToken
	ages   map[string]*HoldAge     // age_id → HoldAge
	proofs map[string]*CloseProof // proof_id → CloseProof
	// 快速查询索引
	planByOrder  map[string]string   // order_id → plan_id
	tokensByPlan map[string][]string // plan_id → []token_id
	agByPlan     map[string]string   // plan_id → age_id
	proofByPlan map[string]string // plan_id → proof_id
	planByIdem  map[string]string // idempotency_key → plan_id
	sla   SLAConfig
	clock func() time.Time
}

// New 构造内存后端服务，使用默认 SLA 配置。
  func New() *Service {
	   return NewWithSLA(DefaultSLAConfig)
   }

// NewWithSLA 构造内存后端服务，使用指定 SLA 配置。

   func NewWithSLA(sla SLAConfig) *Service {
	   if sla.OrderTimeoutSecs <= 0 {
		   sla.OrderTimeoutSecs = DefaultSLAConfig.OrderTimeoutSecs
	  }
	  if sla.GraceSecs < 0 {
		   sla.GraceSecs = 0
	  }
	   if sla.SLAEscalateSecs <= 0 {
		   sla.SLAEscalateSecs = DefaultSLAConfig.SLAEscalateSecs
	   }
	  if sla.SLAWarningRatio <= 0 || sla.SLAWarningRatio >= 1 {
		   sla.SLAWarningRatio = DefaultSLAConfig.SLAWarningRatio
	  }
	   return &Service{
		  plans: make(map[string]*AbandonPlan),
		   tokens: make(map[string]*MutexToken),
		  ages: make(map[string]*HoldAge),
		   proofs:       make(map[string]*CloseProof),
		   planByOrder:  make(map[string]string),
		   tokensByPlan: make(map[string][]string),
		  agByPlan: make(map[string]string),
		   proofByPlan: make(map[string]string),
		   planByIdem:  make(map[string]string),
		  sla:   sla,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// SetClock 注入可控时钟（用）。

  func (s *Service) SetClock(fn func() time.Time) {
	   if s != nil && fn != nil {
		  s.clock = fn
	   }
  }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	  return s.clock()
  }

// ─────────────────────────── ArmPlan 创建弃单计划 ───────────────────────────
// ArmPlan 由超时扫描 worker 调用，为超时卖单创建弃单计划（armed 状态）。
// 不变式：
// - 同一 order_id 只能有一条 AbandonPlan（唯一约束）。
// - 幂等键（idempotency_key）相同时返回已有计划。
// - armed 状态下不执行任何资金操作；仅记录计划待推进。
   func (s *Service) ArmPlan(req ArmPlanReq) (AbandonPlan, error) {
	   if s == nil {
		  return AbandonPlan{}, ErrUnavailable
	   }
	  orderID := strings.TrimSpace(req.OrderID)
	  if orderID == "" {
		   return AbandonPlan{}, ErrOrder
	  }
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return AbandonPlan{}, err
	   }
	  asset, err := validate.Asset(req.Asset)
	  if err != nil {
		   return AbandonPlan{}, ErrAsset
	  }
	   amt, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	   if err != nil || !amt.IsPositive() {
		   return AbandonPlan{}, ErrAmount
	   }
	   holdID := strings.TrimSpace(req.HoldID)
	  if holdID == "" {
		   return AbandonPlan{}, ErrHold
	  }
	  idem := strings.TrimSpace(req.IdempotencyKey)
	   if idem == "" {
		  return AbandonPlan{}, ErrIdem
	   }
	  timeoutSecs := req.TimeoutSecs
	   if timeoutSecs <= 0 {
		   timeoutSecs = s.sla.OrderTimeoutSecs
	  }
	  graceSecs := req.GraceSecs
	  if graceSecs < 0 {
		   graceSecs = 0
	  }
	   slaEscalateSecs := req.SLAEscalateSecs
	   if slaEscalateSecs <= 0 {
		  slaEscalateSecs = s.sla.SLAEscalateSecs
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  // 幂等：相同 idempotency_key 返回已有计划。
	   if pid, ok := s.planByIdem[idem]; ok {
		   return s.plans[pid].Copy(), nil
	   }
	  // 唯一性：同一 order_id 只允许一条计划。
	   if _, ok := s.planByOrder[orderID]; ok {
		  return AbandonPlan{}, ErrPlanExists
	   }
	   now := s.now()
	  planID := uuid.NewString()
	  plan := &AbandonPlan{
		  ID:      planID,
		   OrderID: orderID,
		  UserID: uid,
		  Asset: asset,
		  Amount: amt.String(),
		   HoldID: holdID,
		   Status: PlanStatusArmed,
		  ArmedAt:     now,
		   TimeoutSecs: timeoutSecs,
		  GraceSecs:   graceSecs,
		   SLAEscalateSecs: slaEscalateSecs,
		   Version:         1,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  s.plans[planID] = plan
	  s.planByOrder[orderID] = planID
	  s.planByIdem[idem] = planID
	  return plan.Copy(), nil
  }

// ─────────────────────────── AdvanceAbandon 推进弃单 ───────────────────────────
// AdvanceAbandon 推进弃单计划 armed → abandoning，预留释放互斥令牌。
// 不变式：
// - 计划必须在 armed 状态。
// - 推进时预留 release 令牌（reserved 状态）；同时将 fill 令牌状态记录为 reserved（待 fill 路径可能抢占）。
// - 令牌预留成功后计划进入 abandoning；后续由 CommitRelease 或 CommitFill 决定终态。
   func (s *Service) AdvanceAbandon(req AdvanceAbandonReq) (AbandonPlan, MutexToken, error) {
	   if s == nil {
		  return AbandonPlan{}, MutexToken{}, ErrUnavailable
	   }
	  planID := strings.TrimSpace(req.PlanID)
	  actor := strings.TrimSpace(req.Actor)
	   if actor == "" {
		   actor = "system"
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   plan, ok := s.plans[planID]
	   if !ok {
		  return AbandonPlan{}, MutexToken{}, ErrPlan
	   }
	  if plan.Status.Terminal() {
		  return plan.Copy(), MutexToken{}, ErrPlanTerminal
	   }
	  if plan.Status != PlanStatusArmed {
		  return plan.Copy(), MutexToken{}, ErrPlanNotArmed
	  }
	  now := s.now()
	  // 预留释放令牌。
	  releaseToken := &MutexToken{
		  ID:     uuid.NewString(),
		   PlanID: planID,
		   OrderID:    plan.OrderID,
		  Kind:       TokenKindRelease,
		   Status:     TokenStatusReserved,
		  ReservedBy: actor,
		  ReservedAt: now,
		  Version:   1,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  s.tokens[releaseToken.ID] = releaseToken
	  s.tokensByPlan[planID] = append(s.tokensByPlan[planID], releaseToken.ID)
	   // 计划进入 abandoning。
	   plan.Status = PlanStatusAbandoning
	  t := now
	   plan.AbandoningAt = &t
	   plan.Version++
	   plan.UpdatedAt = now
	   return plan.Copy(), releaseToken.Copy(), nil
  }

// ─────────────────────────── CommitRelease 提交释放令牌 ───────────────────────────
// CommitRelease 弃单释放路径成功，提交释放令牌。
// 不变式：
// - 释放令牌必须在 reserved 状态。
// - 若此时 fill 令牌也已 committed，则两路冲突：计划进入 conflict，返回 ErrTokenConflict。
// - 否则：释放令牌 → committed；若存在 fill 令牌则将其 aborted；计划 → released；生成 CloseProof。

   func (s *Service) CommitRelease(req CommitReleaseReq) (AbandonPlan, CloseProof, error) {
	  if s == nil {
		  return AbandonPlan{}, CloseProof{}, ErrUnavailable
	  }
	  planID := strings.TrimSpace(req.PlanID)
	  tokenID := strings.TrimSpace(req.TokenID)
	   actor := strings.TrimSpace(req.Actor)
	   if actor == "" {
		  actor = "system"
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   plan, ok := s.plans[planID]
	  if !ok {
		   return AbandonPlan{},
			   CloseProof{}, ErrPlan
	   }
	   // 幂等：若计划已是 released 且有证明，直接返回。
	   if plan.Status == PlanStatusReleased {
		  proof := s.proofOfPlanLocked(planID)
		   if proof != nil {
			  return plan.Copy(),
				  proof.Copy(), nil
		   }
	  }
	  // 其他终态（conflict、filled）需先经过令牌校验再决定走冲突路径还是报错。
	  // 若已是 conflict，直接报终态。
	   if plan.Status == PlanStatusConflict {
		  return plan.Copy(), CloseProof{}, ErrPlanTerminal
	   }
	   token, ok := s.tokens[tokenID]
	   if !ok {
		  return AbandonPlan{}, CloseProof{}, ErrToken
	   }
	  if token.PlanID != planID {
		   return AbandonPlan{}, CloseProof{}, ErrToken
	  }
	  if token.Kind != TokenKindRelease {
		  return AbandonPlan{}, CloseProof{}, ErrToken
	   }
	  if token.Status == TokenStatusCommitted {
		  // 幂等：已 committed 则返回现有证明。
		   proof := s.proofOfPlanLocked(planID)
		   if proof != nil {
			  return plan.Copy(), proof.Copy(), nil
		  }
		  return plan.Copy(), CloseProof{}, ErrTokenTerminal
	  }
	  // Token 已 aborted（被对手方抢占）→ 释放路径已失去竞争。
	   if token.Status == TokenStatusAborted {
		   return plan.Copy(), CloseProof{}, ErrTokenAborted
	   }
	   now := s.now()
	  // 冲突检测（仅针对并发状态）：
	   // 当 plan 仍在 abandoning 状态且 fill 令牌已 committed 时，
	   // 说明两路资金操作均已推进，需 desk 仲裁。
	   // 注意：plan 已处于 filled 终态时，fill 路径已正常完成（非冲突），
	   // release 路径为竞争失败者，此情况已在上方的 terminal 检查中处理。
	  if s.hasFillCommittedLocked(planID) {
		  // 两路均尝试 commit：标记冲突。
		   token.Status = TokenStatusCommitted
		  token.ConflictDetected = true
		   token.CommittedBy = actor
		  token.LedgerRef = strings.TrimSpace(req.LedgerRef)
		  token.HoldRef = strings.TrimSpace(req.HoldRef)
		  tt := now
		  token.TerminalAt = &tt
		  token.Version++
		   token.UpdatedAt = now
		  plan.Status = PlanStatusConflict
		   plan.ConflictNote = fmt.Sprintf("release committed at %s after fill already committed; token=%s", now.Format(time.RFC3339), tokenID)
		  t := now
		  plan.TerminalAt = &t
		   plan.Version++
		  plan.UpdatedAt = now
		  return plan.Copy(), CloseProof{}, ErrTokenConflict
	   }
	  // 正常路径：释放令牌 committed。
	   token.Status = TokenStatusCommitted
	  token.CommittedBy = actor
	  token.LedgerRef = strings.TrimSpace(req.LedgerRef)
	  token.HoldRef = strings.TrimSpace(req.HoldRef)
	   tt := now
	   token.TerminalAt = &tt
	   token.Version++
	   token.UpdatedAt = now
	   // 将同计划下已有的 fill 令牌（若存在且未终态）设为 aborted。
	  s.abortCounterTokensLocked(planID, TokenKindFill, actor, "release committed first")
	  // 计划进入 released。
	   plan.Status = PlanStatusReleased
	   t := now
	  plan.TerminalAt = &t
	   plan.TerminalBy = actor
	   plan.Version++
	  plan.UpdatedAt = now
	  // 生成 CloseProof。
	   proof := s.buildProofLocked(plan, token, OutcomeReleased, actor)
	   s.proofs[proof.ID] = proof
	  s.proofByPlan[planID] = proof.ID
	   return plan.Copy(),
		  proof.Copy(), nil
  }

// ─────────────────────────── CommitFill 提交成交令牌 ───────────────────────────
// CommitFill P2P 成交贷记路径成功，提交成交令牌。
// 不变式：
// - 若计划尚未有 fill 令牌，先自动预留（支持 fill 在 armed/abandoning 阶段均可抢占）。
// - 若 release 令牌已 committed（弃单已释放），则两路冲突：计划进入 conflict，返回 ErrTokenConflict。
// - 否则：fill 令牌 → committed；若存在 release 令牌则将其 aborted；计划 → filled；生成 CloseProof。
  func (s *Service) CommitFill(req CommitFillReq) (AbandonPlan, CloseProof, error) {
	   if s == nil {
		   return AbandonPlan{}, CloseProof{}, ErrUnavailable
	  }
	   planID := strings.TrimSpace(req.PlanID)
	   actor := strings.TrimSpace(req.Actor)
	   if actor == "" {
		  actor = "fill_worker"
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  plan, ok := s.plans[planID]
	   if !ok {
		   return AbandonPlan{}, CloseProof{}, ErrPlan
	  }
	   // 幂等：若计划已是 filled 且有证明，直接返回。
	   if plan.Status == PlanStatusFilled {
		   proof := s.proofOfPlanLocked(planID)
		  if proof != nil {
			  return plan.Copy(), proof.Copy(), nil
		  }
	   }
	  // 其他终态直接报错（released=release 已正常完成；conflict=已需要 desk 仲裁）。
	  if plan.Status == PlanStatusReleased || plan.Status == PlanStatusConflict {
		  return plan.Copy(), CloseProof{}, ErrPlanTerminal
	   }
	   now := s.now()
	   // 冲突检测（仅针对并发状态）：
	   // 当 plan 仍在 abandoning 状态且 release 令牌已 committed 时，说明两路并发推进。
	  // 注意：plan 已处于 released 终态时，release 路径已正常完成（非冲突），
	   // fill 路径为竞争失败者，此情况已在上方 terminal 检查中处理。
	  if plan.Status == PlanStatusAbandoning && s.hasReleaseCommittedLocked(planID) {
		   // release 已 committed；fill 抢占失败，进入 conflict。
		   // 自动创建 fill 令牌并标记 conflict。
		   fillToken := &MutexToken{
			   ID: uuid.NewString(),
			  PlanID: planID,
			  OrderID: plan.OrderID,
			   Kind: TokenKindFill,
			   Status: TokenStatusCommitted,
			  ReservedBy:  actor,
			  CommittedBy: actor,
			  ConflictDetected: true,
			   LedgerRef: strings.TrimSpace(req.LedgerRef),
			  ReservedAt: now,
			  Version:   1,
			  CreatedAt: now,
			   UpdatedAt: now,
		  }
		   tt := now
		   fillToken.TerminalAt = &tt
		   s.tokens[fillToken.ID] = fillToken
		   s.tokensByPlan[planID] = append(s.tokensByPlan[planID], fillToken.ID)
		   plan.Status = PlanStatusConflict
		   plan.ConflictNote = fmt.Sprintf("fill committed at %s after release already committed; fill_token=%s", now.Format(time.RFC3339), fillToken.ID)
		   t := now
		   plan.TerminalAt = &t
		  plan.Version++
		   plan.UpdatedAt = now
		   return plan.Copy(),
			  CloseProof{}, ErrTokenConflict
	  }
	   // 正常路径：创建 fill 令牌（若未存在）或复用已预留的 fill 令牌。
	  fillToken := s.findOrCreateFillTokenLocked(plan,
		   actor, now)
	  if fillToken.Status == TokenStatusCommitted {
		   // 幂等：fill 已 committed。
		   proof := s.proofOfPlanLocked(planID)
		  if proof != nil {
			  return plan.Copy(), proof.Copy(), nil
		   }
	   }
	  if fillToken.Status == TokenStatusAborted {
		   return plan.Copy(), CloseProof{}, ErrTokenAborted
	  }
	   // Fill 令牌 committed。
	   fillToken.Status = TokenStatusCommitted
	   fillToken.CommittedBy = actor
	   fillToken.LedgerRef = strings.TrimSpace(req.LedgerRef)
	  tt := now
	   fillToken.TerminalAt = &tt
	   fillToken.Version++
	  fillToken.UpdatedAt = now
	  // 将同计划下已有的 release 令牌（若存在且未终态）设为 aborted。
	  s.abortCounterTokensLocked(planID, TokenKindRelease, actor, "fill committed first")
	   // 计划进入 filled。
	   plan.Status = PlanStatusFilled
	   t := now
	   plan.TerminalAt = &t
	  plan.TerminalBy = actor
	  plan.Version++
	   plan.UpdatedAt = now
	   // 生成 CloseProof。
	   proof := s.buildProofLocked(plan, fillToken, OutcomeFilled, actor)
	   s.proofs[proof.ID] = proof
	   s.proofByPlan[planID] = proof.ID
	  return plan.Copy(), proof.Copy(), nil
  }

// ─────────────────────────── ForceAbandon 强制弃单 ───────────────────────────
// ForceAbandon desk 强制弃单（双人确认），适用于超龄 hold 或卡住的 armed 计划。
// 不变式：
// - 双人确认（operator1 != operator2）。
// - 计划必须未处于终态。
// - 强制弃单后计划直接进入 released 终态，生成 CloseProof（outcome=released）。
  func (s *Service) ForceAbandon(req ForceAbandonReq) (AbandonPlan, CloseProof, error) {
	   if s == nil {
		   return AbandonPlan{}, CloseProof{}, ErrUnavailable
	  }
	   planID := strings.TrimSpace(req.PlanID)
	   op1 := strings.TrimSpace(req.Operator1)
	   op2 := strings.TrimSpace(req.Operator2)
	   reason := strings.TrimSpace(req.Reason)
	   if op1 == "" || op2 == "" {
		   return AbandonPlan{},
			   CloseProof{}, ErrActor
	  }
	  if strings.EqualFold(op1, op2) {
		  return AbandonPlan{}, CloseProof{}, ErrFourEye
	  }
	   if reason == "" {
		  return AbandonPlan{},
			  CloseProof{}, ErrReason
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  plan, ok := s.plans[planID]
	  if !ok {
		  return AbandonPlan{}, CloseProof{}, ErrPlan
	  }
	   if plan.Status.Terminal() {
		   if plan.Status == PlanStatusReleased {
			  proof := s.proofOfPlanLocked(planID)
			   if proof != nil {
				  return plan.Copy(), proof.Copy(), nil
			  }
		   }
		   return plan.Copy(), CloseProof{}, ErrPlanTerminal
	   }
	   now := s.now()
	  actor := op1 + "+" + op2 // 双人组合标识符
	  // 创建强制弃单令牌。
	   forceToken := &MutexToken{
		  ID:      uuid.NewString(),
		   PlanID:  planID,
		   OrderID: plan.OrderID,
		  Kind:    TokenKindRelease,
		   Status: TokenStatusCommitted,
		  ReservedBy:  actor,
		   CommittedBy: actor,
		  ReservedAt:  now,
		  Version: 1,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	   tt := now
	   forceToken.TerminalAt = &tt
	   s.tokens[forceToken.ID] = forceToken
	  s.tokensByPlan[planID] = append(s.tokensByPlan[planID], forceToken.ID)
	  // 中止所有未终态的对手方令牌。
	   s.abortCounterTokensLocked(planID, TokenKindFill, actor, "force abandon by desk: "+reason)
	  // 计划进入 released。
	  plan.Status = PlanStatusReleased
	   t := now
	  plan.TerminalAt = &t
	  plan.TerminalBy = actor
	   plan.Version++
	   plan.UpdatedAt = now
	   // 生成 CloseProof。
	   proof := s.buildProofLocked(plan, forceToken, OutcomeReleased, actor)
	  s.proofs[proof.ID] = proof
	   s.proofByPlan[planID] = proof.ID
	  // 标记超龄记录为已处置（若存在）。
	   if req.AgeRecordID != "" {
		   if age, ok2 := s.ages[strings.TrimSpace(req.AgeRecordID)]; ok2 && !age.Stage.Terminal() {
			   age.Stage = AgeStageResolved
			   t2 := now
			  age.ResolvedAt = &t2
			  age.ResolvedBy = actor
			  age.ResolveNote = "force abandoned by desk: " + reason
			   age.Version++
			  age.UpdatedAt = now
		  }
	  }
	  return plan.Copy(),
		  proof.Copy(), nil
  }

// ─────────────────────────── ValidateClose 收口校验 ───────────────────────────
// ValidateClose 收口校验：调用方在标 closed 前必须调用此方法。
// 不变式：
// - 计划必须存在。
// - 计划必须处于终态（released / filled / conflict）。
// - CloseProof 必须存在且哈希校验通过。
// - conflict 终态的计划不允许 CanClose=true（需人工处理后才能 close）。

   func (s *Service) ValidateClose(req ValidateCloseReq) (ValidateCloseResp, error) {
	  if s == nil {
		   return ValidateCloseResp{}, ErrUnavailable
	   }
	   planID := strings.TrimSpace(req.PlanID)
	  orderID := strings.TrimSpace(req.OrderID)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  plan, ok := s.plans[planID]
	   if !ok {
		  // 先尝试按 order_id 查找。
		  if orderID != "" {
			   if pid, ok2 := s.planByOrder[orderID]; ok2 {
				   plan = s.plans[pid]
				   planID = pid
				   ok = true
			   }
		   }
		  if !ok {
			  return ValidateCloseResp{PlanID: planID, OrderID: orderID, CanClose: false, Reason: "abandon plan not found"}, ErrPlan
		  }
	  }
	  if !plan.Status.Terminal() {
		  return ValidateCloseResp{
			   PlanID: plan.ID,
			  OrderID:  plan.OrderID,
			  CanClose: false,
			   Reason:   fmt.Sprintf("plan status=%s not terminal; cannot close", plan.Status),
		   }, nil
	   }
	  if plan.Status == PlanStatusConflict {
		  return ValidateCloseResp{
			  PlanID:  plan.ID,
			   OrderID: plan.OrderID,
			   CanClose: false,
			   Reason:   "plan in conflict state; requires desk resolution before close",
		   }, nil
	   }
	  proof := s.proofOfPlanLocked(planID)
	   if proof == nil {
		   return ValidateCloseResp{
			  PlanID: plan.ID,
			   OrderID:  plan.OrderID,
			   CanClose: false,
			  Reason:   "close proof missing; funds surface must not mark closed",
		   }, ErrNoProof
	   }
	  // 哈希校验。
	  if err := VerifyProofHash(*proof); err != nil {
		  return ValidateCloseResp{
			   PlanID:   plan.ID,
			   OrderID:  plan.OrderID,
			   CanClose: false,
			  Reason:   "close proof tamper detected: " + err.Error(),
		   }, ErrProofTamper
	   }
	   return ValidateCloseResp{
		  PlanID: plan.ID,
		  OrderID:  plan.OrderID,
		  CanClose: true,
		   Outcome:  proof.Outcome,
		  ProofID:  proof.ID,
	   }, nil
  }

// ─────────────────────────── 查询接口 ───────────────────────────
// GetPlan 按 plan_id 查询弃单计划。
  func (s *Service) GetPlan(planID string) (AbandonPlan, error) {
	   if s == nil {
		   return AbandonPlan{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  p, ok := s.plans[strings.TrimSpace(planID)]
	   if !ok {
		   return AbandonPlan{}, ErrPlan
	  }
	  return p.Copy(), nil
  }

// GetPlanByOrder 按 order_id 查询弃单计划。

   func (s *Service) GetPlanByOrder(orderID string) (AbandonPlan, error) {
	   if s == nil {
		  return AbandonPlan{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   pid, ok := s.planByOrder[strings.TrimSpace(orderID)]
	   if !ok {
		   return AbandonPlan{}, ErrPlan
	   }
	   return s.plans[pid].Copy(), nil
   }

// GetProofByPlan 按 plan_id 查询收口证明。
   func (s *Service) GetProofByPlan(planID string) (CloseProof, error) {
	  if s == nil {
		  return CloseProof{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   proof := s.proofOfPlanLocked(strings.TrimSpace(planID))
	  if proof == nil {
		   return CloseProof{}, ErrProof
	   }
	  return proof.Copy(), nil
  }

// GetToken 按 token_id 查询互斥令牌。

   func (s *Service) GetToken(tokenID string) (MutexToken, error) {
	  if s == nil {
		   return MutexToken{},
			  ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   t, ok := s.tokens[strings.TrimSpace(tokenID)]
	   if !ok {
		  return MutexToken{}, ErrToken
	   }
	  return t.Copy(), nil
   }

// ListTokensByPlan 查询计划下所有互斥令牌。
   func (s *Service) ListTokensByPlan(planID string) []MutexToken {
	   if s == nil {
		  return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  ids := s.tokensByPlan[strings.TrimSpace(planID)]
	  out := make([]MutexToken,
		   0, len(ids))
	   for _, id := range ids {
		  if t, ok := s.tokens[id]; ok {
			   out = append(out, t.Copy())
		  }
	  }
	   return out
   }

// ListArmedPlans 枚举所有 armed 状态的计划（超时扫描 worker 用）。
   func (s *Service) ListArmedPlans() []AbandonPlan {
	   return s.listPlansByStatus(PlanStatusArmed)
  }

// ListAbandoningPlans 枚举所有 abandoning 状态的计划（弃单推进 worker 用）。

  func (s *Service) ListAbandoningPlans() []AbandonPlan {
	   return s.listPlansByStatus(PlanStatusAbandoning)
  }

// ListAllPlans 枚举所有弃单计划（巡检 worker 用）。
  func (s *Service) ListAllPlans() []AbandonPlan {
	   if s == nil {
		  return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]AbandonPlan, 0, len(s.plans))
	   for _, p := range s.plans {
		   out = append(out, p.Copy())
	  }
	  return out
  }

// ListAllProofs 枚举所有收口证明（巡检 worker 用）。

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

// GetAge 按 age_id 查询超龄记录。
  func (s *Service) GetAge(ageID string) (HoldAge, error) {
	   if s == nil {
		  return HoldAge{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  a,
		   ok := s.ages[strings.TrimSpace(ageID)]
	   if !ok {
		  return HoldAge{}, ErrHoldAge
	  }
	   return a.Copy(), nil
  }

// GetAgeByPlan 按 plan_id 查询超龄记录。
  func (s *Service) GetAgeByPlan(planID string) (HoldAge, error) {
	  if s == nil {
		  return HoldAge{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  aid, ok := s.agByPlan[strings.TrimSpace(planID)]
	  if !ok {
		   return HoldAge{}, ErrHoldAge
	   }
	  a, ok := s.ages[aid]
	  if !ok {
		   return HoldAge{}, ErrHoldAge
	   }
	   return a.Copy(), nil
  }

// ─────────────────────────── 内部辅助 ───────────────────────────

   func (s *Service) listPlansByStatus(status PlanStatus) []AbandonPlan {
	   if s == nil {
		  return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]AbandonPlan, 0)
	  for _, p := range s.plans {
		   if p.Status == status {
			   out = append(out, p.Copy())
		   }
	  }
	   return out
   }

// proofOfPlanLocked 返回计划的 CloseProof（调用前必须持锁）。

  func (s *Service) proofOfPlanLocked(planID string) *CloseProof {
	   pid, ok := s.proofByPlan[planID]
	   if !ok {
		   return nil
	  }
	  p, ok := s.proofs[pid]
	  if !ok {
		  return nil
	   }
	   return p
  }

// hasFillCommittedLocked 检查计划下是否已有 fill 令牌 committed（持锁）。
   func (s *Service) hasFillCommittedLocked(planID string) bool {
	  for _, tid := range s.tokensByPlan[planID] {
		   t := s.tokens[tid]
		  if t.Kind == TokenKindFill && t.Status == TokenStatusCommitted {
			  return true
		   }
	   }
	  return false
   }

// hasReleaseCommittedLocked 检查计划下是否已有 release 令牌 committed（持锁）。
   func (s *Service) hasReleaseCommittedLocked(planID string) bool {
	  for _, tid := range s.tokensByPlan[planID] {
		   t := s.tokens[tid]
		  if t.Kind == TokenKindRelease && t.Status == TokenStatusCommitted {
			  return true
		   }
	   }
	  return false
  }

// abortCounterTokensLocked 将同计划下指定类型的未终态令牌全部设为 aborted（持锁）。
   func (s *Service) abortCounterTokensLocked(planID string, kind TokenKind, actor, reason string) {
	   now := s.now()
	   for _, tid := range s.tokensByPlan[planID] {
		  t := s.tokens[tid]
		  if t.Kind == kind && !t.Status.Terminal() {
			   t.Status = TokenStatusAborted
			   t.AbortedBy = actor
			  t.AbortReason = reason
			  tt := now
			  t.TerminalAt = &tt
			  t.Version++
			   t.UpdatedAt = now
		  }
	   }
   }

// findOrCreateFillTokenLocked 找到计划已有的 fill 令牌，若不存在则新建（持锁）。
   func (s *Service) findOrCreateFillTokenLocked(plan *AbandonPlan, actor string, now time.Time) *MutexToken {
	   for _, tid := range s.tokensByPlan[plan.ID] {
		   t := s.tokens[tid]
		  if t.Kind == TokenKindFill {
			   return t
		   }
	  }
	   // 新建 fill 令牌。
	   fillToken := &MutexToken{
		   ID:     uuid.NewString(),
		   PlanID: plan.ID,
		   OrderID: plan.OrderID,
		  Kind:    TokenKindFill,
		  Status:  TokenStatusReserved,
		   ReservedBy: actor,
		  ReservedAt: now,
		   Version:    1,
		  CreatedAt:  now,
		  UpdatedAt: now,
	  }
	  s.tokens[fillToken.ID] = fillToken
	   s.tokensByPlan[plan.ID] = append(s.tokensByPlan[plan.ID], fillToken.ID)
	  return fillToken
  }

// buildProofLocked 生成 CloseProof（持锁）。
  func (s *Service) buildProofLocked(plan *AbandonPlan, token *MutexToken, outcome CloseOutcome, actor string) *CloseProof {
	  now := s.now()
	  proofID := uuid.NewString()
	  phash := ComputeProofHash(ProofHashInput{
		   PlanID:  plan.ID,
		   OrderID: plan.OrderID,
		   Outcome:          outcome,
		  CommittedTokenID: token.ID,
		   Amount:           plan.Amount,
		   Asset:            plan.Asset,
	   })
	  return &CloseProof{
		   ID:     proofID,
		   PlanID: plan.ID,
		   OrderID: plan.OrderID,
		  UserID:  plan.UserID,
		  Asset:  plan.Asset,
		   Amount: plan.Amount,
		   Outcome: outcome,
		  CommittedTokenID: token.ID,
		  CommittedKind: token.Kind,
		   ProofHash: phash,
		   HoldRef:   token.HoldRef,
		   LedgerRef: token.LedgerRef,
		  DecidedBy: actor,
		   CreatedAt: now,
	  }
  }

// UpsertAge 创建或更新 HoldAge 记录（超龄巡检 worker 调用）。

  func (s *Service) UpsertAge(planID string, holdAgeSecs, slaSecs int64) (HoldAge, error) {
	  if s == nil {
		   return HoldAge{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  plan, ok := s.plans[strings.TrimSpace(planID)]
	  if !ok {
		  return HoldAge{}, ErrPlan
	   }
	   now := s.now()
	   // 若已存在，更新年龄与阶段。
	   if aid, ok2 := s.agByPlan[planID]; ok2 {
		   age := s.ages[aid]
		  if age.Stage.Terminal() {
			  return age.Copy(), nil
		   }
		   age.HoldAgeSecs = holdAgeSecs
		   age.PatrolCount++
		   t := now
		  age.LastPatrol = &t
		  // 重新评估阶段。
		   age.Stage = evaluateAgeStage(holdAgeSecs, slaSecs, DefaultSLAConfig.SLAWarningRatio)
		  age.Version++
		  age.UpdatedAt = now
		   return age.Copy(), nil
	  }
	   // 新建超龄记录。
	  ageID := uuid.NewString()
	  age := &HoldAge{
		   ID: ageID,
		  PlanID: planID,
		  OrderID: plan.OrderID,
		  UserID: plan.UserID,
		   Asset:  plan.Asset,
		   Amount: plan.Amount,
		  HoldID:      plan.HoldID,
		   Stage:       evaluateAgeStage(holdAgeSecs, slaSecs, DefaultSLAConfig.SLAWarningRatio),
		  HoldAgeSecs: holdAgeSecs,
		   SLASecs:     slaSecs,
		   PatrolCount: 1,
		  Version:     1,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	  t := now
	   age.LastPatrol = &t
	  s.ages[ageID] = age
	   s.agByPlan[planID] = ageID
	   return age.Copy(), nil
   }

// EscalateAge 超龄升级（超过 SLA 阈值时调用）。
  func (s *Service) EscalateAge(req EscalateAgeReq) (HoldAge, error) {
	  if s == nil {
		   return HoldAge{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   age, ok := s.ages[strings.TrimSpace(req.AgeID)]
	  if !ok {
		   return HoldAge{}, ErrHoldAge
	  }
	   if age.Stage.Terminal() {
		  return age.Copy(), ErrHoldAgeResolved
	  }
	  now := s.now()
	  age.Stage = AgeStageEscalated
	  t := now
	   age.EscalatedAt = &t
	   age.EscalatedTo = strings.TrimSpace(req.EscalateTo)
	  age.EscalateNote = strings.TrimSpace(req.EscalateNote)
	  age.Version++
	  age.UpdatedAt = now
	  return age.Copy(), nil
  }

// ResolveAge 标记超龄记录已处置。
   func (s *Service) ResolveAge(req ResolveAgeReq) (HoldAge, error) {
	  if s == nil {
		   return HoldAge{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   age,
		  ok := s.ages[strings.TrimSpace(req.AgeID)]
	   if !ok {
		  return HoldAge{}, ErrHoldAge
	  }
	  if age.Stage == AgeStageResolved {
		   return age.Copy(), nil
	   }
	   now := s.now()
	   age.Stage = AgeStageResolved
	   t := now
	   age.ResolvedAt = &t
	  age.ResolvedBy = strings.TrimSpace(req.ResolvedBy)
	  age.ResolveNote = strings.TrimSpace(req.ResolveNote)
	  age.Version++
	  age.UpdatedAt = now
	   return age.Copy(), nil
   }

// ListEscalatedAges 枚举所有 escalated 超龄记录（desk 干预 worker 用）。
   func (s *Service) ListEscalatedAges() []HoldAge {
	   if s == nil {
		   return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]HoldAge, 0)
	   for _, a := range s.ages {
		  if a.Stage == AgeStageEscalated {
			  out = append(out, a.Copy())
		  }
	   }
	   return out
  }

// ListAllAges 枚举所有超龄记录（巡检用）。

  func (s *Service) ListAllAges() []HoldAge {
	   if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]HoldAge, 0, len(s.ages))
	  for _, a := range s.ages {
		  out = append(out, a.Copy())
	  }
	  return out
  }

// PatrolProof 巡检收口证明哈希，检测是否被篡改。
   func (s *Service) PatrolProof(planID string) (CloseProof, error) {
	   if s == nil {
		  return CloseProof{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  proof := s.proofOfPlanLocked(strings.TrimSpace(planID))
	  if proof == nil {
		   return CloseProof{}, ErrProof
	  }
	  now := s.now()
	   t := now
	   proof.TamperCheckedAt = &t
	   if err := VerifyProofHash(*proof); err != nil {
		  proof.TamperDetected = true
		   proof.TamperDetail = fmt.Sprintf("hash mismatch at %s: %v", now.Format(time.RFC3339), err)
		   return proof.Copy(), ErrProofTamper
	  }
	   proof.TamperDetected = false
	   proof.TamperDetail = ""
	   return proof.Copy(), nil
   }

// InjectConflictForTest 辅助方法：将计划强制置为 conflict 状态（仅用于单元）。
// 代码不应调用此方法；冲突状态仅在并发竞态场景下产生。

  func (s *Service) InjectConflictForTest(planID string) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  plan,
		  ok := s.plans[strings.TrimSpace(planID)]
	   if !ok {
		  return ErrPlan
	  }
	   if plan.Status.Terminal() {
		   return ErrPlanTerminal
	   }
	  now := s.now()
	  plan.Status = PlanStatusConflict
	   plan.ConflictNote = "injected for test"
	  t := now
	   plan.TerminalAt = &t
	  plan.Version++
	   plan.UpdatedAt = now
	  return nil
  }

// SLACfg 返回当前 SLA 配置（只读）。
   func (s *Service) SLACfg() SLAConfig {
	   if s == nil {
		  return DefaultSLAConfig
	  }
	  return s.sla
   }

// evaluateAgeStage 根据当前 hold 年龄与 SLA 阈值评估超龄阶段。
  func evaluateAgeStage(holdAgeSecs, slaSecs int64, warningRatio float64) AgeStage {
	   if holdAgeSecs >= slaSecs {
		  return AgeStageEscalated
	   }
	   warnThreshold := int64(float64(slaSecs) * warningRatio)
	  if holdAgeSecs >= warnThreshold {
		   return AgeStageWarning
	  }
	   return AgeStageNormal
  }
