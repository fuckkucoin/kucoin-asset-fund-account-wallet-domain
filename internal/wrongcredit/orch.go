// @FileName: orch.go
// @Summary: 错贷记追回冲销编排器 FSM——驱动 WrongCreditCase 从 open 到 close 的全流程。
// FSM 转移图：
// open ──[score]──────────────────────────────────────► scored
// scored ──[approve_freeze]────────────────────────────► freeze_available
// freeze_available ──[froze_ok]────────────────────────► frozen
// frozen ──[approve_revert]────────────────────────────► reversal_entries
// reversal_entries ──[revert_ok]───────────────────────► close (全额)
// reversal_entries ──[partial_ok]──────────────────────► partial_claw
// partial_claw ──[bad_debt_ok]─────────────────────────► bad_debt_hold
// bad_debt_hold ──[bad_debt_ok]────────────────────────► close (坏账完成)
// * ──[waive]────────────────────────────────────────────► waived (任意非终态)
// * ──[abort]────────────────────────────────────────────► aborted (错误/互斥冲突)
// scored ──[reopen_score]──────────────────────────────► open (补充证据重评)
// - 编排器只驱动 Step 转移；Phase 映射由 phaseFor 维护。
// - 业务账本操作（冻结/冲销分录）通过 LedgerApplier 接口注入，方便替换。
// - 互斥检查在 StepFreezeAvailable 入口进行；ClaimWrongCredit 失败直接 abort。
// - 所有 Advance 调用须提供 fenceEpoch+fenceToken（乐观锁），不匹配时拒绝。
// - 编排器可在任何步骤中断后从当前 Step 续运（幂等/可中断）。
// 不变量：
// - Advance 不可跳步；只允许 FSM 边表中定义的转移。
// - Terminal Step（close/waived/aborted）不再接受任何事件。
// - fenceEpoch 在每次成功推进后单调递增，token 同步刷新。
// @Author: Kinsley
// @Created: 2021.12.29

package wrongcredit

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/fsm"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

// ─── FSM 边表 ─────────────────────────────────────────────────────────────────

var orchFSM = fsm.New([]fsm.Edge[Step, Event]{
	// 分值
	{From: StepOpen,
		Event: EventScore, To: StepScored},
	// 重新分值（补充证据后）
	{From: StepScored, Event: EventReopenScore, To: StepOpen},
	// 批准冻结
	{From: StepScored, Event: EventApproveFreeze, To: StepFreezeAvailable},
	// 冻结完成
	{From: StepFreezeAvailable, Event: EventFrozeOK, To: StepFrozen},
	// 批准冲销
	{From: StepFrozen, Event: EventApproveRevert, To: StepReversalEntries},
	// 全额冲销完成
	{From: StepReversalEntries, Event: EventRevertOK, To: StepClose},
	// 部分追回完成
	{From: StepReversalEntries, Event: EventPartialOK, To: StepPartialClaw},
	// 坏账挂账完成（来自部分追回）
	{From: StepPartialClaw, Event: EventBadDebtOK, To: StepBadDebtHold},
	// 坏账挂账完成（来自全坏账路径）
	{From: StepReversalEntries,
		Event: EventBadDebtOK, To: StepBadDebtHold},
	// 坏账关闭
	{From: StepBadDebtHold, Event: EventBadDebtOK, To: StepClose},
	// Waive：任意非终态均可豁免（编排器处理时需先校验非终态）
	{From: StepOpen, Event: EventWaive, To: StepWaived},
	{From: StepScored, Event: EventWaive, To: StepWaived},
	{From: StepFreezeAvailable,
		Event: EventWaive, To: StepWaived},
	{From: StepFrozen, Event: EventWaive, To: StepWaived},
	{From: StepReversalEntries, Event: EventWaive, To: StepWaived},
	{From: StepPartialClaw, Event: EventWaive, To: StepWaived},
	{From: StepBadDebtHold, Event: EventWaive, To: StepWaived},
	// Abort：任意非终态均可终止
	{From: StepOpen, Event: EventAbort, To: StepAborted},
	{From: StepScored, Event: EventAbort, To: StepAborted},
	{From: StepFreezeAvailable, Event: EventAbort, To: StepAborted},
	{From: StepFrozen, Event: EventAbort, To: StepAborted},
	{From: StepReversalEntries, Event: EventAbort, To: StepAborted},
	{From: StepPartialClaw,
		Event: EventAbort, To: StepAborted},
	{From: StepBadDebtHold, Event: EventAbort, To: StepAborted},
}, StepClose, StepWaived, StepAborted)

// phaseFor Step → Phase 映射；编排器推进 Step 后同步更新 Phase。
  func phaseFor(s Step) Phase {
	  switch s {
	   case StepOpen, StepScored:
		  return PhaseInvestigating
	  case StepFreezeAvailable, StepFrozen:
		   return PhaseFrozenPending
	  case StepReversalEntries:
		   return PhaseFrozenPending // 冲销中，仍显示 frozen_pending
	  case StepClose:
		   // close 对应 reversed 或 partial，由 closePhase 字段精化。
		   return PhaseReversed
	   case StepPartialClaw, StepBadDebtHold:
		  return PhasePartial
	  case StepWaived:
		   return PhaseWaived
	  case StepAborted:
		  return PhaseInvestigating // aborted 回到 investigating，待人工处理
	  default:
		  return PhaseInvestigating
	   }
  }

// ─── LedgerApplier 接口（依赖注入） ──────────────────────────────────────────
// LedgerApplier 抽象账本写操作；实现调 ledger.Service，可用 FakeLedger。

type LedgerApplier interface {
	// ApplyJournal 提交一批分录到账本；幂等键防止重复提交。
	// 返回实际生效的 transferID（或已有的 transferID，幂等场景）。
	ApplyJournal(ctx context.Context, idem string, entries []ledger.JournalEntry) (string, error)
	// AvailableBalance 查询用户 spot 账户的可用余额。
	AvailableBalance(ctx context.Context, userID, asset string) (decimal.Decimal, error)
}

// ─── 编排器 ──────────────────────────────────────────────────────────────────
// Orchestrator 错贷记追回编排器；进程内单例，所有操作线程安全。
type Orchestrator struct {
	mu    sync.Mutex
	cases map[string]*WrongCreditCase // key = caseID
	byIdem   map[string]string             // idempotencyKey → caseID
	evs      map[string][]EvidenceEnvelope // key = caseID
	timeline map[string][]TimelineEvent    // key = caseID
	seqN map[string]int // timeline 序号计数器
	disc  *Discovery
	mutex *CreditMutex
	ledger LedgerApplier
	halt bool
	clock func() time.Time
}

// NewOrchestrator 构造编排器（发现引擎和互斥锁须外部注入）。
  func NewOrchestrator(disc *Discovery, mutex *CreditMutex) *Orchestrator {
	  return &Orchestrator{
		   cases:    map[string]*WrongCreditCase{},
		  byIdem:   map[string]string{},
		  evs:      map[string][]EvidenceEnvelope{},
		  timeline: map[string][]TimelineEvent{},
		  seqN:     map[string]int{},
		   disc:  disc,
		  mutex: mutex,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// BindLedger 注入账本适配器（可在 New 之后调用，时注入 fake）。
  func (o *Orchestrator) BindLedger(l LedgerApplier) *Orchestrator {
	   if o != nil {
		   o.ledger = l
	  }
	  return o
  }

// SetHalt 开关维护暂停标志；halt=true 时拒绝所有写操作。
   func (o *Orchestrator) SetHalt(on bool) {
	  if o == nil {
		   return
	  }
	  o.mu.Lock()
	  o.halt = on
	  o.mu.Unlock()
   }

   func (o *Orchestrator) now() time.Time {
	  if o == nil || o.clock == nil {
		  return time.Now().UTC()
	   }
	   return o.clock()
  }

// ─── 开案 ─────────────────────────────────────────────────────────────────────
// Open 开立错贷记追回案件；幂等（相同 idempotencyKey 返回已有案件）。

   func (o *Orchestrator) Open(req OpenReq) (WrongCreditCase,
	   error) {
	  if o == nil {
		  return WrongCreditCase{}, ErrUnavailable
	   }
	  if o.isHalted() {
		  return WrongCreditCase{}, ErrHalt
	   }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return WrongCreditCase{}, ErrUser
	  }
	   asset, err := validate.Asset(req.Asset)
	   if err != nil {
		   return WrongCreditCase{}, ErrAsset
	  }
	  amt,
		   err := validatePositiveDecimal(req.CreditedAmt)
	   if err != nil {
		  return WrongCreditCase{}, ErrAmount
	   }
	  idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		   return WrongCreditCase{}, ErrIdem
	  }
	  cref := strings.TrimSpace(req.CreditRef)
	   if cref == "" {
		   return WrongCreditCase{}, ErrCreditRef
	  }
	   actor, err := validate.Actor(req.OpenedBy)
	  if err != nil {
		   return WrongCreditCase{}, ErrActor
	   }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	  // 幂等检查。
	   if id, ok := o.byIdem[idem]; ok {
		  cur := o.cases[id]
		   if cur.UserID != uid || cur.Asset != asset || cur.CreditRef != cref {
			  return WrongCreditCase{}, ErrIdem
		  }
		   return cur.Copy(), nil
	  }
	  // 同一 CreditRef 不能有多个 OPEN 案件。
	   for _, c := range o.cases {
		  if c.CreditRef == cref && !c.Phase.Terminal() {
			  return WrongCreditCase{}, ErrDuplicate
		  }
	  }
	  now := o.now()
	  row := &WrongCreditCase{
		   ID:             uuid.NewString(),
		   IdempotencyKey: idem,
		  UserID:         uid,
		   CreditRef:      cref,
		  Asset: asset,
		  CreditedAmt: amt.String(),
		  Confidence: ConfUnscored,
		  RiskTier:   RiskTier1,
		   Phase:      PhaseInvestigating,
		   Step:       StepOpen,
		   FenceEpoch: 1,
		  FenceToken: persist.NewToken(),
		  OpenedBy:   actor,
		  Note:       strings.TrimSpace(req.Note),
		  CreatedAt:  now,
		  UpdatedAt:  now,
	   }
	   o.cases[row.ID] = row
	  o.byIdem[idem] = row.ID
	  o.appendTimeline(row.ID, "open", actor, "", "", StepOpen, StepOpen, req.Note)
	   return row.Copy(), nil
  }

// ─── 证据附加 ─────────────────────────────────────────────────────────────────
// AttachEvidence 向案件附加证据；不触发自动重评（须显式调用 Score）。
   func (o *Orchestrator) AttachEvidence(req AttachEvidenceReq) (EvidenceEnvelope, error) {
	  if o == nil {
		  return EvidenceEnvelope{}, ErrUnavailable
	  }
	  if err := ValidateEvidence(req); err != nil {
		  return EvidenceEnvelope{}, err
	   }
	   caseID := strings.TrimSpace(req.CaseID)
	  if caseID == "" {
		  return EvidenceEnvelope{},
			  ErrNotFound
	  }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	  c, ok := o.cases[caseID]
	   if !ok {
		  return EvidenceEnvelope{}, ErrNotFound
	  }
	  if c.Phase.Terminal() {
		   return EvidenceEnvelope{}, ErrTerminal
	   }
	  now := o.now()
	   env := EvidenceEnvelope{
		  ID: uuid.NewString(),
		   CaseID: caseID,
		   Source: req.Source,
		   SHA256: strings.TrimSpace(req.SHA256),
		   Kind:   strings.TrimSpace(req.Kind),
		   Note:    strings.TrimSpace(req.Note),
		  Payload: req.Payload,
		   Actor:     strings.TrimSpace(req.Actor),
		   Score:     clampScore(req.Score),
		   CreatedAt: now,
	  }
	   o.evs[caseID] = append(o.evs[caseID], env)
	   o.appendTimeline(caseID, "evidence_attach", env.Actor, "", "", c.Step, c.Step,
		   "kind="+env.Kind+" source="+string(env.Source))
	  return env, nil
   }

// ─── 分值 ─────────────────────────────────────────────────────────────────────
// Score 触发发现引擎对当前证据集合重分值；更新 Confidence/RiskTier/ScoreNote。
// 分值后 Step 由 open → scored；scored 状态再次调用会重分值但不改变 Step。
   func (o *Orchestrator) Score(caseID, actor, usdRate string) (WrongCreditCase, error) {
	   if o == nil {
		  return WrongCreditCase{}, ErrUnavailable
	   }
	  if o.isHalted() {
		  return WrongCreditCase{}, ErrHalt
	  }
	   if _, err := validate.Actor(actor); err != nil {
		   return WrongCreditCase{},
			   ErrActor
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   c, ok := o.cases[strings.TrimSpace(caseID)]
	  if !ok {
		  return WrongCreditCase{}, ErrNotFound
	  }
	  if c.Phase.Terminal() {
		   return WrongCreditCase{}, ErrTerminal
	  }
	  // 允许 open/scored 状态分值；其他步骤不允许。
	   if c.Step != StepOpen && c.Step != StepScored {
		  return WrongCreditCase{}, ErrIllegalTransition
	  }
	  evs := o.evs[c.ID]
	  sr, tier, err := o.disc.Eval(evs, c.CreditedAmt, usdRate, c.RiskTier)
	  if err != nil {
		   return WrongCreditCase{}, err
	   }
	   fromStep := c.Step
	   c.Confidence = sr.Confidence
	   c.RiskTier = tier
	   c.ScoreNote = sr.Note
	   c.ScoreVersion++
	   if c.Step == StepOpen {
		   next, ferr := orchFSM.Apply(c.Step, EventScore)
		  if ferr != nil {
			   return WrongCreditCase{}, ErrIllegalTransition
		  }
		  c.Step = next
		   c.Phase = phaseFor(next)
	  }
	   o.bumpFenceLocked(c)
	  o.appendTimeline(c.ID, "score", actor, Phase(""), Phase(""), fromStep, c.Step,
		   "conf="+string(c.Confidence)+" tier="+string(c.RiskTier))
	  return c.Copy(), nil
   }

// ─── 批准冻结 ─────────────────────────────────────────────────────────────────
// ApproveFreeze 运营批准冻结用户可用余额（须置信度 ≥ high）。
// Tier3 须先有第二审批人（ApproveFreeze 自身只记第一人，须再调 ApproveFreeze2）。

   func (o *Orchestrator) ApproveFreeze(req ApproveReq) (WrongCreditCase, error) {
	   if o == nil {
		   return WrongCreditCase{}, ErrUnavailable
	  }
	  if o.isHalted() {
		  return WrongCreditCase{}, ErrHalt
	  }
	   if _, err := validate.Actor(req.Actor); err != nil {
		  return WrongCreditCase{}, ErrActor
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  c, err := o.requireCaseLocked(req.CaseID, req.FenceEpoch, req.FenceToken)
	   if err != nil {
		   return WrongCreditCase{}, err
	  }
	   if c.Step != StepScored {
		   return WrongCreditCase{}, ErrIllegalTransition
	  }
	   if !c.Confidence.ActionableConfidence() {
		  return WrongCreditCase{}, ErrInsufficientConf
	   }
	   now := o.now()
	   c.ApprovedBy = req.Actor
	  c.ApprovedAt = &now
	   next, ferr := orchFSM.Apply(c.Step, EventApproveFreeze)
	  if ferr != nil {
		   return WrongCreditCase{}, ErrIllegalTransition
	   }
	   c.Step = next
	  c.Phase = phaseFor(next)
	   o.bumpFenceLocked(c)
	   o.appendTimeline(c.ID, "approve_freeze", req.Actor, Phase(""), Phase(""), StepScored, c.Step, "")
	  return c.Copy(), nil
  }

// ApproveFreeze2 Tier3 第二审批人；须与第一审批人不同。
  func (o *Orchestrator) ApproveFreeze2(req ApproveReq) (WrongCreditCase, error) {
	   if o == nil {
		  return WrongCreditCase{}, ErrUnavailable
	   }
	  if o.isHalted() {
		  return WrongCreditCase{}, ErrHalt
	  }
	  if _, err := validate.Actor(req.Actor); err != nil {
		  return WrongCreditCase{}, ErrActor
	   }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	   c, err := o.requireCaseLocked(req.CaseID, req.FenceEpoch, req.FenceToken)
	   if err != nil {
		  return WrongCreditCase{}, err
	  }
	  if c.Step != StepFreezeAvailable {
		  return WrongCreditCase{},
			   ErrIllegalTransition
	  }
	  if !c.RiskTier.DualApprovalRequired() {
		  return WrongCreditCase{}, ErrDualApprovalRequired
	   }
	  if c.ApprovedBy == req.Actor {
		  return WrongCreditCase{}, ErrSameApprover
	   }
	   now := o.now()
	  c.ApprovedBy2 = req.Actor
	  c.ApprovedAt2 = &now
	   o.bumpFenceLocked(c)
	   o.appendTimeline(c.ID, "approve_freeze_2", req.Actor, Phase(""), Phase(""), c.Step, c.Step, "tier3_dual")
	   return c.Copy(), nil
  }

// ─── 执行冻结 ─────────────────────────────────────────────────────────────────
// ExecuteFreeze 编排器执行冻结分录（账本写操作）；幂等。
// Tier3 须双人批准后方可调用。

   func (o *Orchestrator) ExecuteFreeze(ctx context.Context, caseID string) (WrongCreditCase, error) {
	  if o == nil {
		   return WrongCreditCase{}, ErrUnavailable
	   }
	  if o.isHalted() {
		  return WrongCreditCase{}, ErrHalt
	   }
	  o.mu.Lock()
	  c, ok := o.cases[strings.TrimSpace(caseID)]
	   if !ok {
		   o.mu.Unlock()
		  return WrongCreditCase{},
			  ErrNotFound
	  }
	  if c.Step != StepFreezeAvailable {
		   o.mu.Unlock()
		   return WrongCreditCase{}, ErrIllegalTransition
	   }
	   if c.RiskTier.DualApprovalRequired() && (c.ApprovedBy2 == "" || c.ApprovedBy == c.ApprovedBy2) {
		  o.mu.Unlock()
		  return WrongCreditCase{}, ErrDualApprovalRequired
	  }
	  // 互斥检查：锁定 credit_ref。
	   if err := o.mutex.ClaimWrongCredit(c.CreditRef, c.ID); err != nil {
		  o.bumpFenceLocked(c)
		   _ = o.abortLocked(c, "mutex_claim_failed: "+err.Error())
		   o.mu.Unlock()
		   return WrongCreditCase{}, err
	  }
	  uid, asset, amt := c.UserID, c.Asset, c.CreditedAmt
	  freezeIdem := "wrongcredit-freeze-" + c.ID
	   o.mu.Unlock()
	  // 账本冻结（在锁外执行 IO 操作，避免长时间持有 mu）。
	   creditedDec, _ := decimal.NewFromString(amt)
	   freezeLines := FreezeLines(uuid.NewString(), uid, asset, creditedDec, 1)
	   if o.ledger != nil {
		   _, err := o.ledger.ApplyJournal(ctx, freezeIdem, freezeLines)
		  if err != nil {
			  o.mu.Lock()
			  _ = o.abortLocked(c, "freeze_journal_failed: "+err.Error())
			   o.mu.Unlock()
			   o.mutex.Release(c.CreditRef, c.ID)
			  return WrongCreditCase{}, wrapErr(ErrJournal, err)
		  }
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  next, ferr := orchFSM.Apply(c.Step, EventFrozeOK)
	   if ferr != nil {
		  return WrongCreditCase{}, ErrIllegalTransition
	   }
	   c.Step = next
	   c.Phase = phaseFor(next)
	   o.bumpFenceLocked(c)
	   o.appendTimeline(c.ID, "freeze_ok", "system", Phase(""), Phase(""), StepFreezeAvailable, c.Step, "idem="+freezeIdem)
	  return c.Copy(), nil
  }

// ─── 批准冲销 ─────────────────────────────────────────────────────────────────
// ApproveRevert 运营批准执行冲销分录。
  func (o *Orchestrator) ApproveRevert(req ApproveReq) (WrongCreditCase, error) {
	   if o == nil {
		  return WrongCreditCase{}, ErrUnavailable
	   }
	   if o.isHalted() {
		  return WrongCreditCase{}, ErrHalt
	  }
	   if _, err := validate.Actor(req.Actor); err != nil {
		   return WrongCreditCase{}, ErrActor
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  c,
		  err := o.requireCaseLocked(req.CaseID, req.FenceEpoch, req.FenceToken)
	   if err != nil {
		   return WrongCreditCase{}, err
	   }
	   if c.Step != StepFrozen {
		   return WrongCreditCase{}, ErrIllegalTransition
	   }
	   next, ferr := orchFSM.Apply(c.Step, EventApproveRevert)
	  if ferr != nil {
		   return WrongCreditCase{},
			  ErrIllegalTransition
	  }
	   c.Step = next
	   c.Phase = phaseFor(next)
	  o.bumpFenceLocked(c)
	  o.appendTimeline(c.ID, "approve_revert", req.Actor, Phase(""), Phase(""), StepFrozen, c.Step, "")
	   return c.Copy(), nil
   }

// ─── 执行冲销 ─────────────────────────────────────────────────────────────────
// ExecuteReversal 查询可用余额，构造冲销分录并提交账本；自动选择全额/部分/全坏账路径。
// 幂等：同一 caseID 重复调用检查当前 Step，已到 close 则直接返回。
  func (o *Orchestrator) ExecuteReversal(ctx context.Context, caseID string) (WrongCreditCase, error) {
	   if o == nil {
		  return WrongCreditCase{}, ErrUnavailable
	   }
	   if o.isHalted() {
		   return WrongCreditCase{}, ErrHalt
	  }
	  o.mu.Lock()
	   c,
		   ok := o.cases[strings.TrimSpace(caseID)]
	  if !ok {
		  o.mu.Unlock()
		  return WrongCreditCase{}, ErrNotFound
	   }
	  if stepTerminal(c.Step) {
		   cp := c.Copy()
		  o.mu.Unlock()
		  return cp, nil
	  }
	  if c.Step != StepReversalEntries {
		  o.mu.Unlock()
		  return WrongCreditCase{}, ErrIllegalTransition
	  }
	  uid, asset, creditedAmt := c.UserID, c.Asset, c.CreditedAmt
	   id := c.ID
	  o.mu.Unlock()
	  // 查询当前冻结余额（代替 available，因为 ExecuteFreeze 已将 creditedAmt 冻入 frozen）。
	  credited,
		  _ := decimal.NewFromString(creditedAmt)
	   available := credited // 默认假设冻结金额足够；业务场景通过 ledger 查询 frozen
	   if o.ledger != nil {
		  avail, err := o.ledger.AvailableBalance(ctx, uid, asset)
		  if err == nil {
			  available = avail
		   }
	  }
	  reversalTransferID := uuid.NewString()
	   reversalIdem := "wrongcredit-reversal-" + id
	  lines,
		  clawAmt, remainAmt, err := BuildReversalLines(reversalTransferID, uid, asset, credited, available, 1)
	  if err != nil {
		  o.mu.Lock()
		   _ = o.abortLocked(c, "build_reversal_lines: "+err.Error())
		   o.mu.Unlock()
		  return WrongCreditCase{}, err
	  }
	  if o.ledger != nil {
		   if _, err2 := o.ledger.ApplyJournal(ctx, reversalIdem, lines); err2 != nil {
			   o.mu.Lock()
			  _ = o.abortLocked(c, "reversal_journal_failed: "+err2.Error())
			   o.mu.Unlock()
			  return WrongCreditCase{}, wrapErr(ErrJournal, err2)
		   }
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	  c.ReversalRef = reversalTransferID
	   c.ClawedAmt = clawAmt.String()
	   c.RemainAmt = remainAmt.String()
	   var nextEvent Event
	   var nextStep Step
	   switch {
	  case remainAmt.IsZero():
		  // 全额追回。
		   nextEvent = EventRevertOK
		  nextStep = StepClose
	   case clawAmt.IsZero():
		  // 全坏账。
		  nextEvent = EventBadDebtOK
		  nextStep = StepBadDebtHold
	  default:
		   // 部分追回。
		  nextEvent = EventPartialOK
		  nextStep = StepPartialClaw
	  }
	   next,
		  ferr := orchFSM.Apply(c.Step, nextEvent)
	   if ferr != nil {
		  return WrongCreditCase{}, ErrIllegalTransition
	   }
	  _ = nextStep
	   c.Step = next
	  c.Phase = phaseFor(next)
	   // 部分/全坏账场景：BadDebtHold → Close 需要再推进一步。
	  if c.Step == StepPartialClaw || c.Step == StepBadDebtHold {
		  next2, ferr2 := orchFSM.Apply(c.Step, EventBadDebtOK)
		  if ferr2 == nil {
			   c.Step = next2
			   c.Phase = phaseFor(next2)
		  }
	  }
	  // 坏账 close → 更新 phase 为 partial 或 bad_debt。
	  if c.Step == StepClose {
		   if !remainAmt.IsZero() {
			  c.Phase = PhasePartial
		   } else {
			  c.Phase = PhaseReversed
		  }
		  if clawAmt.IsZero() {
			   c.Phase = PhaseBadDebt
		  }
	  }
	   o.bumpFenceLocked(c)
	  o.appendTimeline(c.ID, "reversal_ok", "system", Phase(""), c.Phase, StepReversalEntries, c.Step,
		  "claw="+clawAmt.String()+" remain="+remainAmt.String())
	  // 释放互斥锁（终态）。
	   if stepTerminal(c.Step) {
		   o.mutex.Release(c.CreditRef, c.ID)
	  }
	   return c.Copy(), nil
   }

// ─── 豁免 ─────────────────────────────────────────────────────────────────────
// Waive 运营豁免：解除冻结（若已冻结），标记案件为 waived，释放互斥锁。
  func (o *Orchestrator) Waive(ctx context.Context, req WaiveReq) (WrongCreditCase, error) {
	   if o == nil {
		  return WrongCreditCase{}, ErrUnavailable
	   }
	   if o.isHalted() {
		   return WrongCreditCase{}, ErrHalt
	   }
	   if _, err := validate.Actor(req.Actor); err != nil {
		  return WrongCreditCase{}, ErrActor
	   }
	  reason := strings.TrimSpace(req.Note)
	   if reason == "" || len(reason) > 512 {
		   return WrongCreditCase{}, ErrReason
	  }
	  o.mu.Lock()
	   c, err := o.requireCaseLocked(req.CaseID, req.FenceEpoch, req.FenceToken)
	  if err != nil {
		   o.mu.Unlock()
		  return WrongCreditCase{}, err
	   }
	   if c.Phase.Terminal() {
		   o.mu.Unlock()
		   return WrongCreditCase{}, ErrTerminal
	  }
	   frozenStep := c.Step == StepFrozen || c.Step == StepFreezeAvailable
	  uid, asset, amt := c.UserID, c.Asset, c.CreditedAmt
	   id := c.ID
	   o.mu.Unlock()
	   // 若已冻结，先解冻。
	   if frozenStep && o.ledger != nil {
		   unfreezeTransferID := uuid.NewString()
		   unfreezeIdem := "wrongcredit-unfreeze-" + id
		   amtDec, _ := decimal.NewFromString(amt)
		  lines := UnfreezeLines(unfreezeTransferID, uid, asset, amtDec, 1)
		   if _, err2 := o.ledger.ApplyJournal(ctx, unfreezeIdem, lines); err2 != nil {
			   return WrongCreditCase{},
				   wrapErr(ErrJournal, err2)
		  }
	   }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	   now := o.now()
	   c.WaivedBy = req.Actor
	   c.WaivedAt = &now
	  c.WaivedNote = reason
	   next, ferr := orchFSM.Apply(c.Step, EventWaive)
	   if ferr != nil {
		   return WrongCreditCase{}, ErrIllegalTransition
	   }
	  c.Step = next
	  c.Phase = PhaseWaived
	  o.bumpFenceLocked(c)
	   o.appendTimeline(c.ID, "waive", req.Actor, Phase(""), PhaseWaived, StepWaived, StepWaived, reason)
	  o.mutex.Release(c.CreditRef, c.ID)
	  return c.Copy(), nil
   }

// ─── 查询 ─────────────────────────────────────────────────────────────────────
// Get 按 caseID 获取案件副本。
  func (o *Orchestrator) Get(caseID string) (WrongCreditCase, error) {
	  if o == nil {
		   return WrongCreditCase{}, ErrUnavailable
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   c, ok := o.cases[strings.TrimSpace(caseID)]
	  if !ok {
		   return WrongCreditCase{}, ErrNotFound
	  }
	  return c.Copy(), nil
   }

// GetByIdem 按幂等键获取案件副本。
  func (o *Orchestrator) GetByIdem(idem string) (WrongCreditCase, error) {
	   if o == nil {
		  return WrongCreditCase{}, ErrUnavailable
	  }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	  id, ok := o.byIdem[strings.TrimSpace(idem)]
	  if !ok {
		   return WrongCreditCase{}, ErrNotFound
	  }
	  c, ok2 := o.cases[id]
	   if !ok2 {
		  return WrongCreditCase{}, ErrNotFound
	   }
	  return c.Copy(), nil
  }

// Restore 从 wrong_credit_* 表 Hydrate 未终态案件到内存（幂等）。
  func (o *Orchestrator) Restore(c WrongCreditCase, evs []EvidenceEnvelope, tl []TimelineEvent) {
	  if o == nil {
		  return
	  }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	  cp := c.Copy()
	  if _, ok := o.cases[cp.ID]; ok {
		  return
	  }
	  o.cases[cp.ID] = &cp
	  if cp.IdempotencyKey != "" {
		  o.byIdem[cp.IdempotencyKey] = cp.ID
	  }
	  dupEvs := make([]EvidenceEnvelope, len(evs))
	  for i, e := range evs {
		  dupEvs[i] = e.Copy()
	  }
	  o.evs[cp.ID] = dupEvs
	  dupTL := make([]TimelineEvent, len(tl))
	  copy(dupTL, tl)
	  o.timeline[cp.ID] = dupTL
	  maxSeq := o.seqN[cp.ID]
	  for _, ev := range tl {
		  if ev.Seq > maxSeq {
			  maxSeq = ev.Seq
		  }
	  }
	  o.seqN[cp.ID] = maxSeq
  }

// List 列举案件（支持 phase/asset/user_id/credit_ref 过滤）。
   func (o *Orchestrator) List(req ListReq) []WrongCreditCase {
	   if o == nil {
		   return nil
	  }
	  limit := req.Limit
	  if limit <= 0 {
		   limit = 50
	  }
	  if limit > 200 {
		  limit = 200
	  }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   out := make([]WrongCreditCase, 0, limit)
	   for _, c := range o.cases {
		  if req.UserID != "" && c.UserID != req.UserID {
			   continue
		   }
		  if req.Phase != "" && string(c.Phase) != req.Phase {
			   continue
		   }
		  if req.Asset != "" && c.Asset != req.Asset {
			  continue
		   }
		  if req.CreditRef != "" && c.CreditRef != req.CreditRef {
			  continue
		  }
		  out = append(out, c.Copy())
		  if len(out) >= limit {
			   break
		   }
	   }
	   return out
   }

// Evidence 获取案件的证据列表。
  func (o *Orchestrator) Evidence(caseID string) []EvidenceEnvelope {
	  if o == nil {
		  return nil
	  }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	   evs := o.evs[strings.TrimSpace(caseID)]
	   out := make([]EvidenceEnvelope, len(evs))
	   for i, e := range evs {
		   out[i] = e.Copy()
	  }
	  return out
   }

// Timeline 获取案件时间轴事件列表。
  func (o *Orchestrator) Timeline(caseID string) []TimelineEvent {
	   if o == nil {
		   return nil
	   }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	  tl := o.timeline[strings.TrimSpace(caseID)]
	  out := make([]TimelineEvent, len(tl))
	   copy(out, tl)
	  return out
   }

// ─── 内部辅助 ─────────────────────────────────────────────────────────────────

  func (o *Orchestrator) isHalted() bool {
	  o.mu.Lock()
	  defer o.mu.Unlock()
	  return o.halt
  }

// requireCaseLocked 在持锁状态下查询案件并校验 fence；调用方须自行 Lock。

  func (o *Orchestrator) requireCaseLocked(caseID string, epoch int64, token string) (*WrongCreditCase, error) {
	  c, ok := o.cases[strings.TrimSpace(caseID)]
	  if !ok {
		   return nil, ErrNotFound
	   }
	   if c.Phase.Terminal() {
		   return nil, ErrTerminal
	   }
	  if c.FenceEpoch != epoch || c.FenceToken != token || token == "" {
		  return nil, ErrFence
	   }
	  return c, nil
   }

// bumpFenceLocked 在持锁状态下递增 fence（epoch+1，刷新 token）。

  func (o *Orchestrator) bumpFenceLocked(c *WrongCreditCase) {
	   c.FenceEpoch++
	   c.FenceToken = persist.NewToken()
	   c.UpdatedAt = o.now()
  }

// abortLocked 在持锁状态下将案件标记为 aborted（仅在内部错误时调用）。
  func (o *Orchestrator) abortLocked(c *WrongCreditCase, note string) error {
	  if c == nil {
		  return ErrUnavailable
	  }
	   next, err := orchFSM.Apply(c.Step, EventAbort)
	  if err != nil {
		  return err
	   }
	  c.Step = next
	   c.Phase = phaseFor(next)
	   c.Note = note
	   o.bumpFenceLocked(c)
	  o.appendTimeline(c.ID, "abort", "system", Phase(""), c.Phase, StepAborted, StepAborted, note)
	   return nil
  }

// appendTimeline 追加时间轴事件（持锁内调用）。

   func (o *Orchestrator) appendTimeline(
	   caseID, kind, actor string,
	   fromPhase, toPhase Phase,
	  fromStep, toStep Step,
	   note string,
   ) {
	   o.seqN[caseID]++
	   ev := TimelineEvent{
		   CaseID: caseID,
		  Seq:    o.seqN[caseID],
		   Kind:   kind,
		  Actor:     actor,
		   FromPhase: fromPhase,
		  ToPhase:  toPhase,
		  FromStep: fromStep,
		   ToStep:   toStep,
		  Note:     note,
		  At:       o.now(),
	   }
	   o.timeline[caseID] = append(o.timeline[caseID], ev)
   }

// validatePositiveDecimal 解析并校验金额 > 0。

  func validatePositiveDecimal(s string) (decimal.Decimal, error) {
	  d,
		   err := decimal.NewFromString(strings.TrimSpace(s))
	  if err != nil || d.IsNegative() || d.IsZero() {
		  return decimal.Zero, ErrAmount
	   }
	   return d, nil
  }
