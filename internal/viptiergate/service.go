// 功能：viptiergate 顶层服务聚合——VIP 档位世代闸与漂移封堵引擎门面。
// Service 将四个子系统整合为统一对外接口：
// LeaseFSM — TierLease 档位切换租约 CAS 状态机
// QuoteVault — QuoteBind 报价世代绑定
// InflightFreezeGuard — 在途冻额只读互斥
// DriftDetector — DriftCase 漂移检测与四眼收口
// DriftProofVault — DriftProof 漂移证明构建与验证
// 核心调用流（抬档路径，以 Pin 为例）：
// 1. GatedPin(req) → 检查是否存在活跃 DriftCase → ErrDriftBlocked
// 2. GatedPin(req) → 读取 CurrentGeneration，传入 ExpectedGen
// 3. LeaseFSM.Acquire(CAS BumpGeneration) → 获得 gen+1 与 TierLease(open)
// 4. QuoteVault.VoidAll(userID) → 作废所有旧世代 open QuoteBind
// 5. LeaseFSM.Bind(lease, gen+1) → 租约推进至 bound
// 6. （可选）LeaseFSM.Switch → worker 确认所有在途报价清零后推进至 switched
// 核心调用流（降档路径，以 ClearPin 为例）：
// 1. GatedClearPin(req) → InflightFreezeGuard.CheckUser → ErrInflightFreeze
// 2. GatedClearPin(req) → QuoteVault.CountActive → ErrInflightQuote
// 3. 通过互斥检查后，调用 vipfee.Service.ClearPin（外部）
// 4. LeaseFSM.Acquire(CAS)；QuoteVault.VoidAll
// QuoteWithdraw 绑定流：
// 1. GatedQuoteWithdraw(userID, asset, gross) → vipfee.Service.QuoteWithdraw（外部）
// 2. QuoteVault.Issue(req) → 颁发绑定当前世代的 QuoteBind
// 3. 返回 QuoteBind.ID 给调用方，调用方在结算时携带 QuoteBind.ID
// 结算验证流：
// 1. SettleQuote(req) → QuoteVault.Settle → 世代 → ErrQuoteGenMismatch 或 settled
// 2. 若漂移检测触发 → DriftDetector.Open → DriftCase(open)
// 3. DriftProofVault.Build + Verify → DriftDetector.AttachProof → DriftCase(proven)
// 4. DriftDetector.ConfirmClose ×2（四眼）→ DriftCase(closed)

// 作者：Knox
// 日期：2021.12.18

package viptiergate

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Service 是 viptiergate 引擎的顶层门面。

type Service struct {
	fsm    *LeaseFSM
	quotes *QuoteVault
	freeze *InflightFreezeGuard
	drift *DriftDetector
	proof *DriftProofVault
	metrics *GateMetrics
	store   Store
	clock func() time.Time
}

// NewService 构造 Service。store 为 nil 时使用内存 Store（场景）。
  func NewService(store Store) *Service {
	  if store == nil {
		  store = NewMemStore()
	  }
	   clock := func() time.Time { return time.Now().UTC() }
	  return &Service{
		  fsm: newLeaseFSM(store, clock),
		   quotes: newQuoteVault(store,
			   clock),
		   freeze: newInflightFreezeGuard(store, clock),
		   drift: newDriftDetector(store, clock),
		  proof: newDriftProofVault(store,
			  clock),
		   metrics: newGateMetrics(),
		   store: store,
		  clock: clock,
	  }
   }

// WithClock 替换时钟（注入）。

  func (s *Service) WithClock(clock func() time.Time) *Service {
	   s.clock = clock
	   s.fsm.clock = clock
	  s.quotes.clock = clock
	   s.freeze.clock = clock
	   s.drift.clock = clock
	  s.proof.clock = clock
	  return s
  }

  // ─────────────────────────────────────────────────────────────────────────────
   // 抬档门控（Pin/RecordVolume 触发的档位上升）
   // ─────────────────────────────────────────────────────────────────────────────
   // GatedUpgrade 是抬档操作的门控入口（被 vipfee 的 GatedPin/GatedRecordVolume 调用）。
  // 调用方需在外层完成 vipfee.Service.Pin/RecordVolume 的业务落账，
  // 本方法负责：CAS 世代递增 + 作废旧世代 QuoteBind + 创建 TierLease。
   // 入参 expectedGen 是调用方读取的当前世代（CAS 前置条件）。
   // 若世代已被并发操作推进，返回 ErrStaleGen，调用方应重读世代后重试。

   func (s *Service) GatedUpgrade(ctx context.Context, req AcquireLeaseReq) (*TierLease, int64, error) {
	   if strings.TrimSpace(req.UserID) == "" {
		   return nil, 0, ErrUserInvalid
	  }
	  // 升档前检查是否存在阻塞性漂移案。
	   blocked,
		   caseID, err := s.drift.IsBlocked(ctx, req.UserID)
	   if err != nil {
		   return nil, 0, err
	   }
	   if blocked {
		  s.metrics.driftBlocked.Add(1)
		  return nil, 0, fmt.Errorf("%w: case_id=%s", ErrDriftBlocked, caseID)
	  }
	   // CAS 世代递增 + 创建租约。
	   lease, gen, err := s.fsm.Acquire(ctx, req)
	  if err != nil {
		   if err == ErrStaleGen {
			  s.metrics.staleGen.Add(1)
		  }
		  return nil, 0, err
	   }
	   s.metrics.leaseAcquired.Add(1)
	  // 作废旧世代 QuoteBind。
	   voided, verr := s.quotes.VoidAll(ctx, req.UserID)
	  if verr != nil {
		   // 作废失败不回滚 CAS（世代已推进）；记录指标，worker 下次扫描会清理。
		   s.metrics.voidQuoteFailure.Add(1)
	  } else {
		   s.metrics.quotesVoided.Add(int64(voided))
	   }
	  // 绑定租约（open → bound）。
	   if _, berr := s.fsm.Bind(ctx, BindLeaseReq{LeaseID: lease.ID, Generation: gen}); berr != nil {
		  // Bind 失败不影响 CAS 结果，租约保持 open 由 worker 推进。
		  s.metrics.leaseBindFailure.Add(1)
	   } else {
		  s.metrics.leaseBound.Add(1)
	  }
	  return lease, gen, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// 降档门控（ClearPin/RecordVolume 触发的档位下降）
// ─────────────────────────────────────────────────────────────────────────────
// GatedDowngrade 是降档操作的门控入口（被 vipfee 的 GatedClearPin 调用）。
// 互斥检查顺序：
// 1. InflightFreezeGuard.CheckUser → ErrInflightFreeze（活跃冻额存在）
// 2. QuoteVault.CountActive → ErrInflightQuote（活跃报价存在）
// 3. 通过检查 → 执行 CAS 世代递增 + 创建 TierLease（blocked 状态）
// 若检查通过但 CAS 失败（ErrStaleGen），说明并发操作已推进世代，调用方重试。
// 若检查失败，返回阻塞错误；外部调用方可选择创建 pending 队列记录。

   func (s *Service) GatedDowngrade(ctx context.Context, req AcquireLeaseReq) (*TierLease, int64, error) {
	   if strings.TrimSpace(req.UserID) == "" {
		   return nil, 0, ErrUserInvalid
	   }
	   // 冻额互斥检查。
	  if err := s.freeze.CheckUser(ctx, req.UserID); err != nil {
		  s.metrics.freezeBlocked.Add(1)
		  return nil, 0, err
	  }
	  // 在途 Quote 互斥检查。
	   activeQuotes, err := s.quotes.CountActive(ctx, req.UserID)
	   if err != nil {
		   return nil, 0, err
	  }
	   if activeQuotes > 0 {
		  s.metrics.quoteBlocked.Add(1)
		  return nil, 0, ErrInflightQuote
	  }
	  // 互斥检查通过，执行 CAS 递增。
	   lease, gen, err := s.fsm.Acquire(ctx, req)
	   if err != nil {
		   return nil, 0, err
	   }
	   s.metrics.leaseAcquired.Add(1)
	   s.metrics.leaseBound.Add(1)
	   return lease, gen, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// QuoteBind 颁发与结算
// ─────────────────────────────────────────────────────────────────────────────
// IssueQuote 颁发 QuoteBind——将报价 BPS 与当前世代绑定。
// 由 vipfee.QuoteWithdraw 完成 BPS 计算后调用。

  func (s *Service) IssueQuote(ctx context.Context, req IssueQuoteReq) (*QuoteBind, error) {
	   qb, err := s.quotes.Issue(ctx, req)
	  if err != nil {
		  return nil, err
	  }
	   s.metrics.quotesIssued.Add(1)
	  return qb, nil
   }

// SettleQuote 提交 QuoteBind 结算，验证世代令牌。
// 若世代已切换（ErrQuoteGenMismatch），自动开立 DriftQuoteGenStale 类型的 DriftCase。
   func (s *Service) SettleQuote(ctx context.Context, req SettleQuoteReq) (*QuoteBind, error) {
	   qb, err := s.quotes.Settle(ctx, req)
	   if err == ErrQuoteGenMismatch {
		   s.metrics.quoteGenMismatch.Add(1)
		  // 自动开立漂移案：QuoteBind 世代与当前世代不符。
		   currentGen, _ := s.store.CurrentGeneration(ctx, req.UserID)
		  if storedQB, gerr := s.store.GetQuoteBind(ctx, req.QuoteBindID); gerr == nil {
			  currentTierGen,
				   _ := s.store.GetGeneration(ctx, req.UserID)
			   expectedTier := TierStandard
			   if currentTierGen != nil {
				   expectedTier = currentTierGen.LastTier
			   }
			  _, _ = s.drift.Open(ctx, OpenDriftCaseReq{
				  UserID: req.UserID,
				   Asset: storedQB.Asset,
				   Kind:         DriftQuoteGenStale,
				  ObservedTier: storedQB.Tier,
				   ExpectedTier: expectedTier,
				   ObservedBPS:  storedQB.BPS,
				   ExpectedBPS: 0, // 期望 BPS 未知时填 0，由四眼收口时人工确认
				   Amount:      storedQB.Gross,
				   QuoteBindID: req.QuoteBindID,
				  Generation: currentGen,
				  Note:       "auto-opened by SettleQuote: quote_gen=" + fmt.Sprintf("%d", storedQB.Generation) + " current_gen=" + fmt.Sprintf("%d", currentGen),
			   })
		  }
		   return nil, err
	  }
	  if err != nil {
		   return nil, err
	   }
	  s.metrics.quotesSettled.Add(1)
	  return qb, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// 冻额管理（只读观察）
// ─────────────────────────────────────────────────────────────────────────────
// RegisterFreeze 记录出金冻额观察快照（由出金模块 push）。

   func (s *Service) RegisterFreeze(ctx context.Context,
	   userID, asset, amount string, bps int, tier Tier, generation int64) (*InflightFreeze, error) {
	  return s.freeze.RegisterFreeze(ctx, userID, asset, amount, bps, tier, generation)
  }

// ReleaseFreeze 标记冻额已释放（由出金模块回调）。
  func (s *Service) ReleaseFreeze(ctx context.Context, freezeID string) error {
	   return s.freeze.ReleaseFreeze(ctx, freezeID)
   }

// ─────────────────────────────────────────────────────────────────────────────
// 漂移案管理
// ─────────────────────────────────────────────────────────────────────────────
// OpenDriftCase 手动开立漂移案（ops/worker 调用）。

  func (s *Service) OpenDriftCase(ctx context.Context, req OpenDriftCaseReq) (*DriftCase, error) {
	   dc,
		   err := s.drift.Open(ctx, req)
	   if err != nil {
		  return nil, err
	  }
	  s.metrics.driftCasesOpened.Add(1)
	   return dc, nil
   }

// BuildAndVerifyProof 为 DriftCase 构建并验证漂移证明，验证通过后自动将案件推进至 proven。
   func (s *Service) BuildAndVerifyProof(ctx context.Context, caseID string) (*DriftProof, error) {
	   p, err := s.proof.Build(ctx, caseID)
	  if err != nil {
		  return nil, err
	  }
	  verified, verr := s.proof.Verify(ctx, p.ID)
	   if verr != nil {
		  s.metrics.proofVerifyFail.Add(1)
		  return p, verr
	  }
	   // 证明通过，推进案件至 proven。
	  if _, aerr := s.drift.AttachProof(ctx, caseID, p.ID); aerr != nil {
		   s.metrics.proofVerifyFail.Add(1)
		  return verified, aerr
	  }
	   s.metrics.proofVerified.Add(1)
	  return verified, nil
   }

// ConfirmDriftClose 四眼收口确认。
  func (s *Service) ConfirmDriftClose(ctx context.Context, req ConfirmDriftReq) (*DriftCase, error) {
	   dc, err := s.drift.ConfirmClose(ctx, req)
	  if err != nil {
		   return nil, err
	  }
	   if dc.Status == DriftClosed {
		   s.metrics.driftCasesClosed.Add(1)
	   }
	  return dc, nil
   }

// ExportProof 导出漂移证明（含在线验签）。
   func (s *Service) ExportProof(ctx context.Context, proofID string) (*DriftProof, error) {
	   return s.proof.Export(ctx, proofID)
   }

// ─────────────────────────────────────────────────────────────────────────────
// 查询接口
// ─────────────────────────────────────────────────────────────────────────────
// GetGeneration 返回用户当前档位世代信息。

  func (s *Service) GetGeneration(ctx context.Context, userID string) (*TierGeneration, error) {
	   return s.store.GetGeneration(ctx, userID)
  }

// GetLease 按 lease_id 查询。
   func (s *Service) GetLease(ctx context.Context, leaseID string) (*TierLease, error) {
	  return s.fsm.GetLease(ctx, leaseID)
   }

// GetQuoteBind 按 quote_bind_id 查询。
  func (s *Service) GetQuoteBind(ctx context.Context, id string) (*QuoteBind, error) {
	   return s.quotes.GetByID(ctx, id)
   }

// GetDriftCase 按 drift_case_id 查询。

  func (s *Service) GetDriftCase(ctx context.Context, caseID string) (*DriftCase, error) {
	  return s.drift.GetCase(ctx,
		   caseID)
   }

// GetDriftProof 按 proof_id 查询。

   func (s *Service) GetDriftProof(ctx context.Context, proofID string) (*DriftProof, error) {
	   return s.proof.GetByID(ctx,
		  proofID)
   }

// ListActiveLeasesForUser 返回用户所有活跃租约。

  func (s *Service) ListActiveLeasesForUser(ctx context.Context, userID string) ([]*TierLease, error) {
	   return s.fsm.ActiveByUser(ctx, userID)
  }

// ListOpenDriftCasesForUser 返回用户所有 open/proven 漂移案。
   func (s *Service) ListOpenDriftCasesForUser(ctx context.Context, userID string) ([]*DriftCase, error) {
	   return s.drift.ListOpenByUser(ctx, userID)
   }

// IsSettlementBlocked 综合检查：用户是否存在任何阻止后续结算的条件。
// 返回 (blocked bool, reason string)。

   func (s *Service) IsSettlementBlocked(ctx context.Context, userID string) (bool, string, error) {
	  blocked, caseID, err := s.drift.IsBlocked(ctx, userID)
	  if err != nil {
		   return false, "", err
	   }
	  if blocked {
		   return true, "drift_case=" + caseID, nil
	  }
	  return false, "", nil
  }

// Stats 返回引擎统计快照（供 /healthz 或 Prometheus 使用）。

  func (s *Service) Stats() map[string]int64 {
	   return s.metrics.Snapshot()
  }
