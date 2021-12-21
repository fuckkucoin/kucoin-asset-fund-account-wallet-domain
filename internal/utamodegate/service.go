// Service 将五个子系统整合为统一对外接口：
// GenerationEngine — ModeGeneration CAS 世代管理
// LeaseFSM — ModeLease 模式切换租约状态机
// InflightBindGuard — LiabilityBind 在途负债/抵押互斥
// DriftDetector — ModeDriftCase 漂移检测与四眼收口
// DriftProofVault — ModeDriftProof 漂移证明构建与验证
// 核心调用流（SetMode 路径）：
// 1. SetMode(userID, toMode) → 检查是否存在阻塞性 DriftCase → ErrDriftBlocked
// 2. SetMode → 读取 CurrentGeneration（CAS 前置条件）
// 3. SetMode → InflightBindGuard.CountOpenBinds
// 若 count > 0 → LeaseFSM.Acquire(blocked) → ErrInflightLiability（租约入待切换队列）
// 4. SetMode → LeaseFSM.Acquire(CAS BumpGen, open/switched) → 新世代 + ModeLease
// 5. SetMode → LeaseFSM.Bind(leaseID, newGen)
// 6. worker 后续：在途负债清零后 LeaseFSM.Switch（推进至 switched）
// 核心调用流（借出/抵押路径）：
// 1. RegisterBind(req) → GenerationEngine.CurrentGen → 颁发 GenToken
// 2. RegisterBind → InflightBindGuard.Register(req, gen) → LiabilityBind(open)
// 漂移发现流（worker 驱动）：
// 1. 发现跨模式键 BindOpen → DriftDetector.Open → ModeDriftCase(open)
// 2. DriftProofVault.Build → DriftDetector.AttachProof → DriftCase(proven)
// 3. ConfirmClose × 2（四眼） → DriftCase(closed)
// 产出标识（供其他包只读消费）：
// mode_lease_id — ModeLease.ID（模式切换租约 ID）
// mode_drift_proof_id — ModeDriftProof.ID（漂移证明 ID）
// 与 isobook / 强平 的隔离：
// Service 不读写 isobook.debt_ratio_gen 或 DebtRatio 字段；
// 不参与 LTV 强平扣押；不改写 ledger Apply / fence 内核。

// Author: Blaise
// Date: 2021.12.16

package utamodegate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Service 是 utamodegate 引擎的顶层门面。
type Service struct {
	gen *GenerationEngine
	fsm *LeaseFSM
	bind *InflightBindGuard
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
	  genEng := newGenerationEngine(store, clock)
	  return &Service{
		   gen: genEng,
		  fsm: newLeaseFSM(store,
			   genEng, clock),
		   bind: newInflightBindGuard(store, clock),
		  drift:   newDriftDetector(store, clock),
		  proof:   newDriftProofVault(store, clock),
		   metrics: newGateMetrics(),
		  store: store,
		   clock: clock,
	  }
   }

// WithClock 替换时钟（注入）。

  func (s *Service) WithClock(clock func() time.Time) *Service {
	  s.clock = clock
	  s.gen.clock = clock
	   s.fsm.clock = clock
	   s.bind.clock = clock
	   s.drift.clock = clock
	  s.proof.clock = clock
	   return s
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // 模式切换门控（SetMode 主路径）
   // ─────────────────────────────────────────────────────────────────────────────
   // SetModeResult 模式切换结果。

  type SetModeResult struct {
	   Lease *ModeLease `json:"lease"`
	   Generation   int64 `json:"generation"`
	  DriftBlocked bool  `json:"drift_blocked,omitempty"`
	   DriftCaseID string `json:"drift_case_id,omitempty"`
   }

   // SetMode 门控式模式切换入口。
  // 调用前置条件：
   // 1. 不存在阻塞性 ModeDriftCase（ErrDriftBlocked）。
   // 2. 不存在未结 LiabilityBind（ErrInflightLiability → 租约进入 blocked 队列）。
  // 3. CAS 世代与调用方预读值一致（ErrStaleGen）。
  // 返回 SetModeResult，Lease.Status 为 open（立即切换）或 blocked（入队）。
  func (s *Service) SetMode(ctx context.Context, userID, toModeStr, triggerKind, triggerRef string) (*SetModeResult, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return nil, ErrUserInvalid
	   }
	  toMode := MarginMode(strings.ToUpper(strings.TrimSpace(toModeStr)))
	  if toMode != MarginModeUnified && toMode != MarginModeClassic {
		   return nil, ErrModeInvalid
	   }
	   s.metrics.modeSwitchTotal.Add(1)
	  // 1. 检查是否存在阻塞性漂移案。
	   blocked, caseID, err := s.drift.IsBlocked(ctx, userID)
	   if err != nil {
		  return nil, err
	   }
	  if blocked {
		  s.metrics.driftBlocked.Add(1)
		   s.metrics.modeSwitchDenied.Add(1)
		   return &SetModeResult{DriftBlocked: true, DriftCaseID: caseID}, ErrDriftBlocked
	   }
	   // 2. 读取当前世代（CAS 前置条件）。
	  mg, err := s.gen.GetGeneration(ctx, userID)
	  if err != nil {
		   return nil, err
	   }
	  currentMode := mg.CurrentMode
	   if currentMode == "" {
		   currentMode = MarginModeUnified
	   }
	  if currentMode == toMode {
		  s.metrics.modeSwitchDenied.Add(1)
		   return nil, ErrSameMode
	  }
	  // 3. 检查在途负债/抵押数量。
	  inflightCount, err := s.bind.CountOpenBinds(ctx, userID)
	   if err != nil {
		   return nil, err
	  }
	   // 4. 构建 TriggerRef（幂等键）。
	   // 调用方若不提供显式 TriggerRef，自动生成含 UUID 的唯一键（防并发碰撞）。
	  if triggerRef == "" {
		  triggerRef = fmt.Sprintf("%s|mode|%s→%s|%d|%s",
			  userID, string(currentMode), string(toMode), s.clock().UnixMilli(), uuid.NewString())
	  }
	  req := AcquireLeaseReq{
		  UserID:   userID,
		   FromMode: currentMode,
		  ToMode: toMode,
		   ExpectedGen: mg.Generation,
		  TriggerKind: triggerKind,
		   TriggerRef:  triggerRef,
	  }
	  // 5. 申请租约（含 CAS + 在途互斥）。
	  lease, newGen, acqErr := s.fsm.Acquire(ctx, req, inflightCount, false)
	  if acqErr != nil {
		   if acqErr == ErrInflightLiability {
			   s.metrics.leaseBlocked.Add(1)
			   s.metrics.modeSwitchPending.Add(1)
			  return &SetModeResult{Lease: lease, Generation: req.ExpectedGen}, ErrInflightLiability
		   }
		  if acqErr == ErrStaleGen {
			   s.metrics.genStale.Add(1)
		  }
		  s.metrics.modeSwitchDenied.Add(1)
		  return nil, acqErr
	  }
	   s.metrics.leaseAcquired.Add(1)
	   s.metrics.genBumped.Add(1)
	  // 6. Bind：推进租约至 bound（在途清零后可直接进 bound）。
	   if inflightCount == 0 {
		   lease, err = s.fsm.Bind(ctx, lease.ID, newGen)
		   if err == nil {
			  s.metrics.leaseBound.Add(1)
		  }
	   }
	   s.metrics.modeSwitchSuccess.Add(1)
	  return &SetModeResult{Lease: lease, Generation: newGen}, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// 世代查询
// ─────────────────────────────────────────────────────────────────────────────
// GetGeneration 查询用户当前模式世代。

  func (s *Service) GetGeneration(ctx context.Context, userID string) (*ModeGeneration, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  return s.gen.GetGeneration(ctx,
		   userID)
  }

// IssueToken 颁发世代令牌（借出/抵押调用前获取）。
  func (s *Service) IssueToken(ctx context.Context, userID string) (*GenToken, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   return s.gen.IssueToken(ctx, userID)
   }

// ValidateToken 验证世代令牌是否有效（借出/抵押落账前检查）。

  func (s *Service) ValidateToken(ctx context.Context, userID string, tokenGen int64) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	   err := s.gen.ValidateToken(ctx, userID, tokenGen)
	  if err != nil {
		  s.metrics.genValidFail.Add(1)
	  } else {
		  s.metrics.genValidOk.Add(1)
	   }
	   return err
  }

// ─────────────────────────────────────────────────────────────────────────────
// 在途负债/抵押绑定
// ─────────────────────────────────────────────────────────────────────────────
// RegisterBind 注册借出/抵押操作的世代绑定（借出/抵押 API 在落账前调用）。
// generation 参数来自调用方预先通过 IssueToken 获取的 GenToken.Generation。
// 若世代令牌与当前世代不符（模式已切换），返回 ErrStaleGen，拒绝注册。
  func (s *Service) RegisterBind(ctx context.Context, req RegisterBindReq, generation int64) (*LiabilityBind, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   // 验证令牌世代有效性（防止旧世代写入）
	   if err := s.gen.ValidateToken(ctx, req.UserID, generation); err != nil {
		   s.metrics.genValidFail.Add(1)
		  return nil, err
	   }
	   s.metrics.genValidOk.Add(1)
	  bind,
		  err := s.bind.Register(ctx, req, generation)
	   if err != nil {
		  return nil, err
	   }
	   s.metrics.bindRegistered.Add(1)
	  return bind, nil
  }

// ReleaseBind 释放借出/抵押操作的世代绑定（还款/抵押释放时调用）。

  func (s *Service) ReleaseBind(ctx context.Context, req ReleaseBindReq) (*LiabilityBind, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  bind, err := s.bind.Release(ctx, req)
	  if err != nil {
		   return nil, err
	  }
	   s.metrics.bindReleased.Add(1)
	   return bind, nil
   }

// GetBind 查询单条 LiabilityBind。

   func (s *Service) GetBind(ctx context.Context, bindID string) (*LiabilityBind, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  return s.bind.GetBind(ctx, bindID)
   }

// ─────────────────────────────────────────────────────────────────────────────
// 漂移案管理
// ─────────────────────────────────────────────────────────────────────────────
// OpenDriftCase 开立漂移案（worker 或 ops 手动触发）。
  func (s *Service) OpenDriftCase(ctx context.Context, req OpenDriftCaseReq) (*ModeDriftCase, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	  dc, err := s.drift.Open(ctx, req)
	  if err != nil {
		  return nil, err
	  }
	  s.metrics.driftCaseOpened.Add(1)
	  return dc, nil
  }

// BuildAndAttachProof 构建漂移证明并将案件推进至 proven。
// 原子封装：Build + Verify + AttachProof。
   func (s *Service) BuildAndAttachProof(ctx context.Context, req BuildProofReq) (*ModeDriftProof, *ModeDriftCase, error) {
	   if s == nil {
		   return nil, nil, ErrUnavailable
	  }
	  proof, err := s.proof.BuildAndVerify(ctx, req)
	  if err != nil {
		   if err == ErrProofDigestMismatch {
			   s.metrics.proofMismatch.Add(1)
		   }
		   return nil, nil, err
	   }
	   s.metrics.proofBuilt.Add(1)
	  s.metrics.proofVerified.Add(1)
	   dc, err := s.drift.AttachProof(ctx, req.DriftCaseID, proof.ID)
	   if err != nil {
		  return nil, nil, err
	   }
	   s.metrics.driftCaseProven.Add(1)
	   return proof, dc, nil
   }

// ConfirmClose 四眼收口确认。
   func (s *Service) ConfirmClose(ctx context.Context, req ConfirmCloseReq) (*ModeDriftCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   dc, err := s.drift.ConfirmClose(ctx, req)
	  if err != nil {
		  return nil, err
	  }
	   if dc.Status == DriftClosed {
		   s.metrics.driftCaseClosed.Add(1)
	  }
	   return dc, nil
   }

// GetDriftCase 查询单条漂移案。
   func (s *Service) GetDriftCase(ctx context.Context, caseID string) (*ModeDriftCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   return s.drift.GetCase(ctx, caseID)
   }

// ListBlockingDriftCases 返回用户所有阻塞性漂移案（open 或 proven）。
  func (s *Service) ListBlockingDriftCases(ctx context.Context, userID string) ([]*ModeDriftCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.drift.ListBlockingCases(ctx,
		   userID)
   }

// IsSettlementBlocked 检查用户是否有阻塞性漂移案（供外部包只读消费）。

   func (s *Service) IsSettlementBlocked(ctx context.Context, userID string) (bool, string, error) {
	   if s == nil {
		   return false, "", ErrUnavailable
	   }
	  return s.drift.IsBlocked(ctx, userID)
  }

// ─────────────────────────────────────────────────────────────────────────────
// 证明查询与导出
// ─────────────────────────────────────────────────────────────────────────────
// GetProof 查询漂移证明。

  func (s *Service) GetProof(ctx context.Context, proofID string) (*ModeDriftProof, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  return s.proof.GetProof(ctx, proofID)
  }

// ExportProof 导出漂移证明摘要（核验存档，不含快照原文）。
  func (s *Service) ExportProof(ctx context.Context, proofID string) (map[string]any, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   return s.proof.Export(ctx, proofID)
   }

// ─────────────────────────────────────────────────────────────────────────────
// 租约查询
// ─────────────────────────────────────────────────────────────────────────────
// GetLease 查询单条租约。
   func (s *Service) GetLease(ctx context.Context, leaseID string) (*ModeLease, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  return s.fsm.GetLease(ctx, leaseID)
   }

// ListActiveLeasesForUser 查询用户所有活跃租约。

  func (s *Service) ListActiveLeasesForUser(ctx context.Context, userID string) ([]*ModeLease, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  return s.fsm.GetByUser(ctx, userID)
  }

// ─────────────────────────────────────────────────────────────────────────────
// 统计
// ─────────────────────────────────────────────────────────────────────────────
// Stats 返回引擎计数器快照。
   func (s *Service) Stats() map[string]int64 {
	   if s == nil {
		   return map[string]int64{}
	   }
	   return s.metrics.Snapshot()
   }

// ─────────────────────────────────────────────────────────────────────────────
// Worker 内部接口（供 Worker 调用，不对外暴露）
// ─────────────────────────────────────────────────────────────────────────────
// workerMarkExpiredBinds 标记过期的 BindOpen 为 BindExpired（供 worker 调用）。

  func (s *Service) workerMarkExpiredBinds(ctx context.Context, batchSize int) ([]*LiabilityBind, error) {
	   expired, err := s.bind.MarkExpired(ctx, batchSize)
	   if err != nil {
		   return nil, err
	   }
	   s.metrics.bindExpired.Add(int64(len(expired)))
	  return expired, nil
  }

// workerScanBlockedLeases 批量扫描 blocked 状态租约（供 worker 调用）。
   func (s *Service) workerScanBlockedLeases(ctx context.Context, limit int) ([]*ModeLease, error) {
	  return s.store.ListLeasesByStatus(ctx, LeaseBlocked, limit)
   }

// workerUnblockLease 尝试将 blocked 租约推进至 switched（在途负债清零后）。

  func (s *Service) workerUnblockLease(ctx context.Context, leaseID string) (*ModeLease, error) {
	   // 检查在途负债是否已清零
	   lease, err := s.fsm.GetLease(ctx, leaseID)
	   if err != nil {
		   return nil, err
	  }
	  count,
		   err := s.bind.CountOpenBinds(ctx, lease.UserID)
	  if err != nil {
		  return nil, err
	   }
	  if count > 0 {
		   return lease, nil // 仍有在途，保持 blocked
	  }
	  // CAS 递增世代（此时在途已清零）。
	   mg, err := s.gen.GetGeneration(ctx, lease.UserID)
	  if err != nil {
		  return nil, err
	  }
	   newGen, err := s.gen.BumpGen(ctx, lease.UserID, mg.Generation, lease.ToMode)
	   if err != nil {
		  return nil, err
	  }
	  s.metrics.genBumped.Add(1)
	  // 将租约世代更新为新世代号后持久化，然后推进状态机。
	   lease.Generation = newGen
	  lease.Status = LeaseBound
	  lease.Note = "unblocked by worker: in-flight liabilities cleared; CAS bumped"
	  lease.UpdatedAt = s.clock()
	  if err := s.store.SaveLease(ctx, lease); err != nil {
		   return nil, err
	  }
	   s.metrics.leaseUnblocked.Add(1)
	   s.metrics.leaseBound.Add(1)
	   // 推进至 switched
	  switched, err := s.fsm.Switch(ctx, lease.ID)
	  if err != nil {
		  return nil, err
	   }
	   s.metrics.leaseSwitched.Add(1)
	   return switched, nil
   }

// workerScanBoundLeases 批量扫描 bound 状态租约，尝试推进至 switched（供 worker 调用）。
  func (s *Service) workerScanBoundLeases(ctx context.Context, limit int) ([]*ModeLease, error) {
	  leases, err := s.store.ListLeasesByStatus(ctx, LeaseBound, limit)
	  if err != nil {
		  return nil, err
	  }
	  for _, l := range leases {
		  // 检查在途负债是否已清零
		  count, err := s.bind.CountOpenBinds(ctx, l.UserID)
		   if err != nil {
			  continue
		   }
		   if count > 0 {
			  continue
		  }
		  switched, err := s.fsm.Switch(ctx, l.ID)
		   if err == nil && switched != nil {
			   s.metrics.leaseSwitched.Add(1)
		   }
	  }
	   return leases, nil
   }

// workerScanDriftCases 批量扫描 open 漂移案，尝试自动构建证明（供 worker 调用）。
  func (s *Service) workerScanDriftCases(ctx context.Context, limit int) ([]*ModeDriftCase, error) {
	   return s.drift.ScanOpenCases(ctx, limit)
   }

// workerOpenDriftFromBind 对跨模式键的过期 BindOpen 触发漂移案（供 worker 调用）。

   func (s *Service) workerOpenDriftFromBind(ctx context.Context, b *LiabilityBind, currentMode MarginMode) error {
	   // 若绑定时的模式键与当前模式不一致，产出漂移案。
	  if b.MarginMode == currentMode {
		   return nil // 无漂移
	  }
	   mg, err := s.gen.GetGeneration(ctx, b.UserID)
	   if err != nil {
		   return err
	  }
	   req := OpenDriftCaseReq{
		  UserID: b.UserID,
		  Asset: b.Currency,
		   Kind:    DriftKindLiabilityKey,
		   OldMode: b.MarginMode,
		  NewMode:       currentMode,
		   OldGeneration: b.Generation,
		   NewGeneration: mg.Generation,
		   LiabilityAmount: b.Amount,
		   BindID:          b.ID,
		   Note:            fmt.Sprintf("bind %s expired with stale mode key", b.ID),
	   }
	   _, err = s.drift.Open(ctx, req)
	   if err != nil {
		   return err
	  }
	  s.metrics.bindDriftFound.Add(1)
	  s.metrics.driftCaseOpened.Add(1)
	   return nil
  }
