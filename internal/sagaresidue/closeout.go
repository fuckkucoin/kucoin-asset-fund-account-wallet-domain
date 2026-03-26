package sagaresidue

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// LedgerWriter 只写接口：执行收口腿的 ledger 划转。
// 注入ledger.Service 时仅使用 Create / CoverFundingFromWallet 路径。

type LedgerWriter interface {
	// ReverseTransfer 对指定 idemKey 的 Transfer 发起冲正（反向划转）。
	// revIdemKey 为原 idemKey+"-rev"；幂等安全。
	ReverseTransfer(ctx context.Context, origIdemKey, revIdemKey string) (*ledger.Transfer, error)
	// SuspenseTransfer 将金额转入悬账科目。
	// 悬账账户为 system.suspense，方向由 SuspenseDirection 决定。
	SuspenseTransfer(ctx context.Context,
		userID, asset, amount, idemKey string) (*ledger.Transfer, error)
}

// IdemBlockStore 幂等块写入接口（用于 FenceBlock 腿）。
type IdemBlockStore interface {
	// Block 在 sr_idem_blocks 表写入一条拒写记录；已存在则幂等成功。
	Block(ctx context.Context, idemKey, reason string) error
}

// CloseoutOrchestrator 收口编排器，不持有可变内部状态（线程安全）。

type CloseoutOrchestrator struct {
	ledger LedgerWriter
	idemBlock IdemBlockStore
	mutexMgr  *MutexManager
	store     Store
}

// NewCloseoutOrchestrator 构造收口编排器。
  func NewCloseoutOrchestrator(
	   lw LedgerWriter,
	  ib IdemBlockStore,
	   mm *MutexManager,
	   st Store,
   ) *CloseoutOrchestrator {
	   return &CloseoutOrchestrator{
		  ledger:    lw,
		  idemBlock: ib,
		   mutexMgr:  mm,
		  store: st,
	  }
  }

// BuildPlan 基于最新 ResidueReport 为 ResidueRun 构建收口计划。
// 规则：
// - 若 ResidueRun 已有活跃 Plan（State = closing），返回 ErrPlanConflict。
// - 双人授权检查（Authorizer1 ≠ Authorizer2，均非空）。
// - 每条确认的 ResidueItem 生成两条腿：
// 腿 A：Reverse（原幂等键 + "-rev"）；若 HopState 不可冲正则改 Suspense。
// 腿 B：FenceBlock（原幂等键 + "-fence"），始终添加。

  func (o *CloseoutOrchestrator) BuildPlan(ctx context.Context, req CloseoutReq, report *ResidueReport, rrun *ResidueRun) (*CloseoutPlan, []CloseoutLeg, error) {
	   if o == nil {
		   return nil, nil, ErrUnavailable
	  }
	  // ── 1. 双人授权校验 ───────────────────────────────────────────────────────
	   if err := checkDualAuth(req.Authorizer1, req.Authorizer2); err != nil {
		   return nil, nil, err
	  }
	  // ── 2. 幂等键前缀校验 ─────────────────────────────────────────────────────
	  idemPfx := strings.TrimSpace(req.IdemPrefix)
	   if idemPfx == "" {
		  return nil, nil, ErrIdemEmpty
	  }
	  // ── 3. 检查是否存在活跃 Plan ──────────────────────────────────────────────
	  existing, err := o.store.GetActivePlan(ctx, rrun.ID)
	   if err == nil && existing != nil {
		  return nil, nil, ErrPlanConflict
	  }
	  // ── 4. 构建腿列表 ─────────────────────────────────────────────────────────
	   now := time.Now().UTC()
	   var legs []CloseoutLeg
	   seq := 1
	  for _, item := range report.Items {
		   if !item.IsConfirmed {
			  // 未经双人确认的 item 暂不收口
			   continue
		  }
		   // ── 腿 A：冲正 or 挂账 ────────────────────────────────────────────
		  legKind,
			  legDir := legKindForHopState(item.HopState)
		   legA := CloseoutLeg{
			  ID: newID(),
			  PlanID: "", // 由 Plan ID 在创建后回填
			  ResidueRunID: rrun.ID,
			   SeqNo: seq,
			   Kind:  legKind,
			   Step:  item.Step,
			   IdemSuffix: item.IdemKey + "-rev",
			  Amount: item.Amount,
			   Asset:  item.Asset,
			   Direction: legDir,
			   State: LegStatePending,
			  RetryCount: 0,
			   CreatedAt:  now,
			  UpdatedAt: now,
		  }
		  legs = append(legs, legA)
		   seq++
		  // ── 腿 B：挡写封印 ─────────────────────────────────────────────────
		   legB := CloseoutLeg{
			   ID: newID(),
			  PlanID: "",
			   ResidueRunID: rrun.ID,
			   SeqNo:        seq,
			  Kind:         LegKindFenceBlock,
			  Step: item.Step,
			   IdemSuffix: item.IdemKey + "-fence",
			  Amount:     "0",
			   Asset:     item.Asset,
			  Direction: "",
			   State:      LegStatePending,
			   RetryCount: 0,
			  CreatedAt:  now,
			  UpdatedAt:  now,
		  }
		   legs = append(legs, legB)
		  seq++
	   }
	   if len(legs) == 0 {
		   return nil, nil, fmt.Errorf("sagaresidue build plan: no confirmed items to close out")
	   }
	   // ── 5. 创建 Plan ──────────────────────────────────────────────────────────
	   plan := &CloseoutPlan{
		   ID:           newID(),
		  ResidueRunID: rrun.ID,
		   SagaRunID:    rrun.SagaRunID,
		  IdemPrefix:  idemPfx,
		  Authorizer1: req.Authorizer1,
		  Authorizer2: req.Authorizer2,
		  Generation:  rrun.MutexGeneration,
		   TotalLegs:   len(legs),
		   AppliedLegs: 0,
		  FailedLegs:  0,
		  SkippedLegs: 0,
		  State:     RunStateClosing,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   // 回填 PlanID
	   for i := range legs {
		   legs[i].PlanID = plan.ID
	  }
	   return plan, legs, nil
  }

// ExecutePlan 执行收口计划中所有 pending 腿。
// 调用前须已通过 MutexManager.Acquire 持有世代锁。
// holderID 须与 Acquire 时一致，generation 须匹配 DB 当前世代。
// 执行逻辑：
// 1. 加载计划的所有 pending 腿（按 SeqNo 排序）；
// 2. 逐腿执行：
// - Reverse：调用 LedgerWriter.ReverseTransfer；
// - Suspense：调用 LedgerWriter.SuspenseTransfer；
// - FenceBlock：调用 IdemBlockStore.Block；
// 3. 每腿执行后，更新腿状态至 DB，并 Refresh 互斥锁；
// 4. ctx 取消时立即停止（Leg 保持 pending，下次 Resume 接着）；
// 5. 所有腿执行完毕后尝试生成 CloseProof（见 Prover）。
  func (o *CloseoutOrchestrator) ExecutePlan(ctx context.Context, plan *CloseoutPlan, generation int64, holderID string) error {
	   if o == nil || o.store == nil {
		  return ErrUnavailable
	   }
	  // ── 1. 加载 pending 腿 ────────────────────────────────────────────────────
	  legs, err := o.store.ListLegs(ctx, LegListReq{PlanID: plan.ID, State: LegStatePending, Limit: 1000})
	  if err != nil {
		   return fmt.Errorf("sagaresidue execute plan load legs: %w", err)
	   }
	  // 按 SeqNo 排序（DB 查询已排序，此处确保）
	  sortLegsBySeq(legs)
	  for _, leg := range legs {
		  // 检查 ctx
		  select {
		   case <-ctx.Done():
			  return ctx.Err()
		  default:
		  }
		   // 执行单腿
		   if err := o.executeLeg(ctx, plan, &leg); err != nil {
			  // 失败后更新状态并接着（max retry 后标 failed）
			  leg.FailReason = err.Error()
			  leg.RetryCount++
			  if leg.RetryCount >= MaxLegRetry {
				  leg.State = LegStateFailed
			  }
			   leg.UpdatedAt = time.Now().UTC()
			  _ = o.store.SaveLeg(ctx, &leg)
			   // failed 腿不中断整个 plan，接着下一腿
			  continue
		  }
		  // 成功：刷新互斥锁 TTL
		  if o.mutexMgr != nil {
			  _ = o.mutexMgr.Refresh(ctx, plan.SagaRunID, generation, holderID)
		   }
	  }
	  // ── 2. 重新统计 Plan 状态 ──────────────────────────────────────────────────
	  if err := o.refreshPlanCounters(ctx, plan); err != nil {
		  return err
	  }
	  return nil
   }

// executeLeg 执行单条收口腿，幂等。
   func (o *CloseoutOrchestrator) executeLeg(ctx context.Context, plan *CloseoutPlan, leg *CloseoutLeg) error {
	   idemKey := plan.IdemPrefix + "-" + leg.IdemSuffix
	  now := time.Now().UTC()
	   switch leg.Kind {
	   case LegKindReverse:
		   // 冲正：原幂等键 = leg.IdemSuffix（已包含 "-rev" 后缀）
		  origIdemKey := strings.TrimSuffix(leg.IdemSuffix, "-rev")
		   tx,
			   err := o.ledger.ReverseTransfer(ctx, origIdemKey, idemKey)
		   if err != nil {
			   return fmt.Errorf("reverse transfer idem=%s: %w", idemKey, err)
		   }
		  leg.LedgerEntryID = tx.ID
		   leg.State = LegStateApplied
		   t := now
		   leg.AppliedAt = &t
		  leg.UpdatedAt = now
		  return o.store.SaveLeg(ctx, leg)
	  case LegKindSuspense:
		   amt, err := decimal.NewFromString(leg.Amount)
		  if err != nil {
			  return fmt.Errorf("suspense amount parse %s: %w", leg.Amount, err)
		   }
		   if amt.IsZero() {
			   leg.State = LegStateSkipped
			   leg.SkipReason = "zero amount"
			  leg.UpdatedAt = now
			  return o.store.SaveLeg(ctx, leg)
		  }
		   tx, err := o.ledger.SuspenseTransfer(ctx, "system.suspense", leg.Asset, leg.Amount, idemKey)
		   if err != nil {
			   return fmt.Errorf("suspense transfer idem=%s: %w", idemKey, err)
		   }
		   leg.LedgerEntryID = tx.ID
		  leg.State = LegStateApplied
		   t := now
		   leg.AppliedAt = &t
		   leg.UpdatedAt = now
		   return o.store.SaveLeg(ctx, leg)
	  case LegKindFenceBlock:
		  reason := fmt.Sprintf("saga-run=%s plan=%s leg=%s", plan.SagaRunID, plan.ID, leg.ID)
		   if err := o.idemBlock.Block(ctx, idemKey, reason); err != nil {
			   return fmt.Errorf("fence block idem=%s: %w", idemKey, err)
		   }
		   leg.State = LegStateApplied
		   t := now
		  leg.AppliedAt = &t
		   leg.UpdatedAt = now
		   return o.store.SaveLeg(ctx, leg)
	   default:
		   return fmt.Errorf("unknown leg kind: %s", leg.Kind)
	  }
  }

// SkipLeg 运营授权跳过一条 pending 腿（需双人授权）。

  func (o *CloseoutOrchestrator) SkipLeg(ctx context.Context, req SkipLegReq) error {
	  if o == nil || o.store == nil {
		  return ErrUnavailable
	   }
	  if err := checkDualAuth(req.Authorizer1, req.Authorizer2); err != nil {
		  return err
	   }
	  skipReason := strings.TrimSpace(req.SkipReason)
	   if skipReason == "" {
		   return fmt.Errorf("sagaresidue skip leg: reason required")
	  }
	  leg,
		   err := o.store.GetLeg(ctx, req.LegID)
	   if err != nil {
		  return ErrLegNotFound
	  }
	   if leg.State != LegStatePending {
		  return ErrLegNotPending
	  }
	   leg.State = LegStateSkipped
	   leg.SkipReason = fmt.Sprintf("[%s+%s] %s", req.Authorizer1, req.Authorizer2, skipReason)
	   leg.UpdatedAt = time.Now().UTC()
	   return o.store.SaveLeg(ctx, leg)
  }

// refreshPlanCounters 重新统计 Plan 的 applied/failed/skipped 腿数，并更新 DB。
   func (o *CloseoutOrchestrator) refreshPlanCounters(ctx context.Context, plan *CloseoutPlan) error {
	  legs,
		   err := o.store.ListLegs(ctx, LegListReq{PlanID: plan.ID, Limit: 10000})
	  if err != nil {
		   return err
	  }
	   a, f, s, p := 0, 0, 0, 0
	  for _, l := range legs {
		   switch l.State {
		   case LegStateApplied:
			  a++
		   case LegStateFailed:
			   f++
		   case LegStateSkipped:
			   s++
		   case LegStatePending:
			   p++
		  }
	   }
	   plan.AppliedLegs = a
	  plan.FailedLegs = f
	  plan.SkippedLegs = s
	   plan.UpdatedAt = time.Now().UTC()
	   // 若无 pending 腿，Plan 进入 closed（即使有 failed 腿，后续由运营处理）
	  if p == 0 && f == 0 {
		   plan.State = RunStateClosed
	  } else if p == 0 && f > 0 {
		   plan.State = RunStateEscalated
	   }
	   return o.store.SavePlan(ctx, plan)
  }

// ── 辅助函数 ─────────────────────────────────────────────────────────────────
// legKindForHopState 根据 ledger hop 的状态决定收口腿类型。
// 可安全冲正的状态：credited、confirmed、chain_submitted
// 不可直接冲正的状态：initiated、spot_frozen（挂账替代）

   func legKindForHopState(hopState string) (LegKind, string) {
	  switch ledger.State(hopState) {
	   case ledger.StateCredited,
		   ledger.StateConfirmed, ledger.StateChainSubmitted:
		   return LegKindReverse, "reverse"
	   default:
		  return LegKindSuspense, "suspense"
	  }
  }

// checkDualAuth 验证双人授权规则。
  func checkDualAuth(a1, a2 string) error {
	   a1 = strings.TrimSpace(a1)
	  a2 = strings.TrimSpace(a2)
	  if a1 == "" || a2 == "" {
		   return ErrDualAuth
	   }
	  if strings.EqualFold(a1, a2) {
		   return ErrDualAuth
	  }
	   return nil
   }

// sortLegsBySeq 按 SeqNo 升序排列腿（原地排序）。

  func sortLegsBySeq(legs []CloseoutLeg) {
	  for i := 1; i < len(legs); i++ {
		  for j := i; j > 0 && legs[j].SeqNo < legs[j-1].SeqNo; j-- {
			  legs[j], legs[j-1] = legs[j-1], legs[j]
		   }
	  }
   }
