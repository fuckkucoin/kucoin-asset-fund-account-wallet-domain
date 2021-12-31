// 功能：WrongCredit 服务门面——组合编排器、持久化、发现引擎和互斥锁，
// 提供对外统一 API（被 HTTP handler 调用）。
// - Service 不直接持有账本引用；通过 Orchestrator.BindLedger 注入 LedgerApplier。
// - Service 不缓存案件状态；Orchestrator 内存是权威；Store 为持久化备份。
// - Hydrate 在进程启动时从 Store 重建 Orchestrator 内存（仅 open 案件）。
// 不变量：
// - Service 内所有写操作先写 Orchestrator 内存，再异步持久化（当前实现同步写）。
// - 互斥锁内存状态在 Hydrate 时从 Store 还原，保证重启不丢失互斥信息。

// 作者：Kinsley
// 日期：2021.12.30

package wrongcredit

import (
	"context"
	"strings"
	"sync"
)

// Service 错贷记追回服务门面；进程内单例，线程安全。

type Service struct {
	mu   sync.Mutex
	orch *Orchestrator
	store *Store
	halt  bool
}

// NewService 构造服务（发现引擎、互斥锁、KV 存储由外部注入）。
   func NewService(disc *Discovery, mutex *CreditMutex, store *Store) *Service {
	  return &Service{
		   orch: NewOrchestrator(disc, mutex),
		   store: store,
	   }
  }

// BindLedger 注入账本适配器（可选；为 nil 时编排器跳过账本操作）。
   func (svc *Service) BindLedger(l LedgerApplier) *Service {
	   if svc != nil {
		   svc.orch.BindLedger(l)
	  }
	   return svc
  }

// SetHalt 设置维护暂停标志。
  func (svc *Service) SetHalt(on bool) {
	   if svc == nil {
		   return
	   }
	   svc.mu.Lock()
	   svc.halt = on
	  svc.mu.Unlock()
	   svc.orch.SetHalt(on)
  }

   func (svc *Service) checkHalt() error {
	   if svc == nil {
		  return ErrUnavailable
	   }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	  if svc.halt {
		  return ErrHalt
	   }
	   return nil
   }

// ─── 对外操作 ─────────────────────────────────────────────────────────────────
// Open 开立案件；同步持久化。

  func (svc *Service) Open(ctx context.Context, req OpenReq) (WrongCreditCase, error) {
	  if err := svc.checkHalt(); err != nil {
		   return WrongCreditCase{}, err
	   }
	  c, err := svc.orch.Open(req)
	   if err != nil {
		   return WrongCreditCase{}, err
	  }
	  if svc.store != nil {
		  if _, serr := svc.store.SaveCase(ctx, c); serr != nil {
			   // 持久化失败不回滚内存；运营可通过 Hydrate 恢复（最终一致）。
			   _ = serr
		  }
	   }
	   recordPhaseTransition("", c.Phase, c.Asset)
	  return c, nil
   }

// AttachEvidence 附加证据；同步持久化。

  func (svc *Service) AttachEvidence(ctx context.Context, req AttachEvidenceReq) (EvidenceEnvelope, error) {
	   if err := svc.checkHalt(); err != nil {
		   return EvidenceEnvelope{}, err
	   }
	   e, err := svc.orch.AttachEvidence(req)
	   if err != nil {
		   return EvidenceEnvelope{}, err
	  }
	  if svc.store != nil {
		   _ = svc.store.SaveEvidence(ctx, e)
	   }
	  recordEvidenceAttached(e.Source)
	   return e, nil
   }

// Score 触发分值；更新 Orchestrator 和持久化。
  func (svc *Service) Score(ctx context.Context, caseID, actor, usdRate string) (WrongCreditCase, error) {
	   if err := svc.checkHalt(); err != nil {
		   return WrongCreditCase{}, err
	  }
	  prev, _ := svc.orch.Get(caseID)
	  c, err := svc.orch.Score(caseID, actor, usdRate)
	   if err != nil {
		   return WrongCreditCase{}, err
	  }
	   if svc.store != nil {
		  _, _ = svc.store.SaveCase(ctx, c)
	   }
	   recordScoreResult(c.Confidence, c.RiskTier)
	   recordPhaseTransition(prev.Phase, c.Phase, c.Asset)
	  recordOrchStep(c.Step)
	   return c, nil
   }

// ApproveFreeze 运营批准冻结；更新持久化。

  func (svc *Service) ApproveFreeze(ctx context.Context, req ApproveReq) (WrongCreditCase, error) {
	  if err := svc.checkHalt(); err != nil {
		   return WrongCreditCase{}, err
	   }
	   prev, _ := svc.orch.Get(req.CaseID)
	  c, err := svc.orch.ApproveFreeze(req)
	  if err != nil {
		  return WrongCreditCase{}, err
	   }
	  if svc.store != nil {
		   _, _ = svc.store.SaveCase(ctx, c)
	   }
	  recordPhaseTransition(prev.Phase, c.Phase, c.Asset)
	  recordOrchStep(c.Step)
	  return c, nil
  }

// ApproveFreeze2 Tier3 第二审批；更新持久化。

  func (svc *Service) ApproveFreeze2(ctx context.Context, req ApproveReq) (WrongCreditCase, error) {
	  if err := svc.checkHalt(); err != nil {
		   return WrongCreditCase{}, err
	  }
	   c, err := svc.orch.ApproveFreeze2(req)
	  if err != nil {
		  return WrongCreditCase{}, err
	  }
	  if svc.store != nil {
		   _, _ = svc.store.SaveCase(ctx, c)
	  }
	   return c, nil
  }

// ExecuteFreeze 执行冻结分录；更新持久化。

   func (svc *Service) ExecuteFreeze(ctx context.Context, caseID string) (WrongCreditCase, error) {
	  if err := svc.checkHalt(); err != nil {
		   return WrongCreditCase{}, err
	  }
	   prev, _ := svc.orch.Get(caseID)
	  c, err := svc.orch.ExecuteFreeze(ctx, caseID)
	  if err != nil {
		   return WrongCreditCase{}, err
	  }
	   if svc.store != nil {
		  _, _ = svc.store.SaveCase(ctx, c)
		  if svc.orch.mutex != nil {
			   claims := svc.orch.mutex.Snapshot()
			  for _, cl := range claims {
				  if cl.CreditRef == c.CreditRef {
					   _ = svc.store.SaveMutexClaim(ctx, cl)
					  break
				   }
			   }
		   }
	  }
	   recordPhaseTransition(prev.Phase, c.Phase, c.Asset)
	   recordOrchStep(c.Step)
	  recordMutexEvent("claim", "wrong_credit")
	  return c, nil
  }

// ApproveRevert 运营批准执行冲销；更新持久化。
  func (svc *Service) ApproveRevert(ctx context.Context, req ApproveReq) (WrongCreditCase, error) {
	  if err := svc.checkHalt(); err != nil {
		  return WrongCreditCase{}, err
	   }
	  prev, _ := svc.orch.Get(req.CaseID)
	   c, err := svc.orch.ApproveRevert(req)
	   if err != nil {
		  return WrongCreditCase{}, err
	  }
	  if svc.store != nil {
		  _, _ = svc.store.SaveCase(ctx, c)
	  }
	   recordPhaseTransition(prev.Phase, c.Phase, c.Asset)
	   recordOrchStep(c.Step)
	   return c, nil
  }

// ExecuteReversal 执行冲销分录（全额/部分/全坏账自动选择）；更新持久化。

  func (svc *Service) ExecuteReversal(ctx context.Context, caseID string) (WrongCreditCase, error) {
	   if err := svc.checkHalt(); err != nil {
		   return WrongCreditCase{}, err
	  }
	   prev, _ := svc.orch.Get(caseID)
	   c, err := svc.orch.ExecuteReversal(ctx, caseID)
	   if err != nil {
		   return WrongCreditCase{}, err
	  }
	  if svc.store != nil {
		   _, _ = svc.store.SaveCase(ctx, c)
		   if c.Phase.Terminal() {
			   _ = svc.store.DeleteMutexClaim(ctx, c.CreditRef)
		   }
	   }
	  recordPhaseTransition(prev.Phase, c.Phase, c.Asset)
	  recordOrchStep(c.Step)
	   switch c.Phase {
	   case PhaseReversed:
		  recordReversalResult("full")
	   case PhasePartial:
		  recordReversalResult("partial")
	  case PhaseBadDebt:
		  recordReversalResult("bad_debt")
	   }
	  if c.Phase.Terminal() {
		  recordMutexEvent("release", "wrong_credit")
	  }
	  return c, nil
  }

// Waive 运营豁免案件；更新持久化，释放互斥锁持久化记录。

  func (svc *Service) Waive(ctx context.Context, req WaiveReq) (WrongCreditCase, error) {
	  if err := svc.checkHalt(); err != nil {
		   return WrongCreditCase{}, err
	  }
	  prev,
		   _ := svc.orch.Get(req.CaseID)
	  c, err := svc.orch.Waive(ctx, req)
	   if err != nil {
		  return WrongCreditCase{}, err
	   }
	   if svc.store != nil {
		   _, _ = svc.store.SaveCase(ctx, c)
		   _ = svc.store.DeleteMutexClaim(ctx,
			   c.CreditRef)
	   }
	  recordPhaseTransition(prev.Phase, c.Phase, c.Asset)
	   recordMutexEvent("release", "waive")
	   return c, nil
  }

// ─── 查询接口 ─────────────────────────────────────────────────────────────────
// Get 获取案件；先从 Orchestrator 内存，回退到 Store。

  func (svc *Service) Get(ctx context.Context, caseID string) (WrongCreditCase, error) {
	   if svc == nil {
		   return WrongCreditCase{}, ErrUnavailable
	  }
	   c, err := svc.orch.Get(caseID)
	   if err == nil {
		  return c, nil
	  }
	  if svc.store == nil {
		  return WrongCreditCase{},
			  ErrNotFound
	   }
	   return svc.store.GetCase(ctx, caseID)
   }

// List 列举案件。

  func (svc *Service) List(ctx context.Context, req ListReq) []WrongCreditCase {
	   if svc == nil {
		   return nil
	   }
	  if svc.store != nil && svc.store.hasSQL() {
		   rows, err := svc.store.ListCases(ctx, req)
		   if err == nil && len(rows) > 0 {
			   return rows
		   }
	  }
	  return svc.orch.List(req)
   }

// Evidence 获取案件证据列表。
  func (svc *Service) Evidence(ctx context.Context, caseID string) []EvidenceEnvelope {
	  if svc == nil {
		   return nil
	   }
	   evs := svc.orch.Evidence(caseID)
	  if len(evs) > 0 || svc.store == nil {
		   return evs
	   }
	   storeEvs, _ := svc.store.ListEvidence(ctx, caseID)
	  return storeEvs
  }

// Timeline 获取时间轴事件。
  func (svc *Service) Timeline(ctx context.Context, caseID string) []TimelineEvent {
	   if svc == nil {
		   return nil
	   }
	   tl := svc.orch.Timeline(caseID)
	  if len(tl) > 0 || svc.store == nil {
		   return tl
	   }
	   storeTL, _ := svc.store.ListTimeline(ctx, caseID)
	  return storeTL
   }

// MutexSnapshot 返回当前互斥锁快照（运营查询）。
  func (svc *Service) MutexSnapshot() []creditClaim {
	   if svc == nil || svc.orch == nil || svc.orch.mutex == nil {
		   return nil
	  }
	  return svc.orch.mutex.Snapshot()
  }

// ─── 启动 Hydrate ─────────────────────────────────────────────────────────────
// Hydrate 进程启动时从 Store 重建内存状态（open 案件 + 互斥锁）。
// 幂等：重复调用不会损坏内存状态（仅追加不存在的记录）。

   func (svc *Service) Hydrate(ctx context.Context) error {
	   if svc == nil || svc.store == nil {
		   return nil
	  }
	  // 重建互斥锁内存状态。
	   claims, err := svc.store.LoadMutexClaims(ctx)
	  if err != nil {
		  return err
	   }
	  if svc.orch != nil && svc.orch.mutex != nil {
		   for _, cl := range claims {
			  switch cl.Kind {
			   case ClaimKindWrongCredit:
				  _ = svc.orch.mutex.ClaimWrongCredit(cl.CreditRef, cl.Owner)
			   case ClaimKindReorg:
				   _ = svc.orch.mutex.ClaimReorg(cl.CreditRef, cl.Owner)
			  }
		   }
	   }
	   if svc.store != nil && svc.store.hasSQL() && svc.orch != nil {
		   cases, err := svc.store.sql.listActiveCases(ctx)
		   if err != nil {
			   return err
		   }
		   for _, c := range cases {
			   evs, _ := svc.store.ListEvidence(ctx, c.ID)
			   tl, _ := svc.store.ListTimeline(ctx, c.ID)
			   svc.orch.Restore(c, evs, tl)
		   }
	   }
	   return nil
  }

// ─── ClaimReorgForBatch 供 reorg-reversal 批次使用 ───────────────────────────
// ClaimReorgForBatch 供 reorg-reversal 批次在执行 unwind 前调用，锁定 credit_ref。
// 若已被 wrong-credit 案件锁定，返回 ErrMutexConflict。

   func (svc *Service) ClaimReorgForBatch(ctx context.Context, creditRef, batchID string) error {
	   if svc == nil {
		   return ErrUnavailable
	   }
	  if svc.orch == nil || svc.orch.mutex == nil {
		   return nil
	   }
	   if err := svc.orch.mutex.ClaimReorg(creditRef, batchID); err != nil {
		   return err
	  }
	   if svc.store != nil {
		  cl := creditClaim{
			   CreditRef: creditRef,
			   Kind:      ClaimKindReorg,
			  Owner: batchID,
		  }
		  _ = svc.store.SaveMutexClaim(ctx, cl)
	   }
	  recordMutexEvent("claim", "reorg")
	   return nil
   }

// ReleaseReorgBatch 供 reorg-reversal 批次在完成或终止后释放锁。
   func (svc *Service) ReleaseReorgBatch(ctx context.Context, creditRef, batchID string) {
	  if svc == nil {
		   return
	  }
	  if svc.orch != nil && svc.orch.mutex != nil {
		  svc.orch.mutex.Release(creditRef, batchID)
	  }
	   if svc.store != nil {
		   _ = svc.store.DeleteMutexClaim(ctx,
			  creditRef)
	  }
	   recordMutexEvent("release", "reorg")
  }

// IsLockedByReorg 供 reorg 批次查询某 credit_ref 是否被 wrong-credit 锁定。
  func (svc *Service) IsLockedByWrongCredit(creditRef string) bool {
	   if svc == nil || svc.orch == nil || svc.orch.mutex == nil {
		   return false
	   }
	  return strings.TrimSpace(creditRef) != "" && svc.orch.mutex.IsLockedByWrongCredit(creditRef)
   }
