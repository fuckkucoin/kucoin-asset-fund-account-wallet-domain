// 功能：probetokengate.Engine：探针+令牌联合租约挡写核心编排。
// 职责（只写本域，不回写相邻域核心 FSM）：
// 1. JointCheck：联合执行 writegate.CheckGate + 签发 ProbeTokenLease；
// Check 成功后原子签发 probe_token_lease_id，固化探针指纹与令牌世代。
// 2. BindLease：写提交首次携带 probe_token_lease_id；
// checked → bound，绑定 commit_ref，复验探针+令牌未漂移。
// 3. CommitCheck：写提交最终核验；复验探针指纹与令牌世代，漂移则置 revoked
// 并落 ProbeTokenDriftProof；通过则 consumed 终态。
// 4. RevokeOnProbeFlip：探针翻转通知，批量吊销指定 scope 的活跃租约。
// 5. RevokeOnPoisonRevoke：毒价撤令通知，批量吊销活跃租约并落漂移证明。
// 6. ExpireLease：TTL 超限租约置 expired（LeaseExpiryWorker 驱动）。
// 7. ProveProof / ConfirmProof / DismissProof：证明生命周期管理。
// 8. ExportProof：证明审计摘要导出。
// 并发安全：Store 负责行级原子性；Engine 层不维护内存锁。
// 不改账：Engine 不调用 ledger；不调用 withdraw 广播 FSM。
// 只读钩子：ProbeMutexReader / GenTokenReader（writegate 包接口）。
// 隔离：不改写 pricepoison.PoisonEngine 内核；不改写 custodyhalt。

// 作者：Rook
// 日期：2023.06.21

package probetokengate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/writegate"
)

// ─── 配置 ─────────────────────────────────────────────────────────────────
// Config Engine 配置项。
type Config struct {
	// LeaseTTL 联合租约有效期（默认 120s；Check→写提交必须在此时间内完成）。
	// 过短会导致正常写提交超时；过长会扩大探针翻转窗口。建议 60–120s。
	LeaseTTL time.Duration
	// RevokeBatchSize 探针翻转/撤令批量吊销每批处理量（默认 200）。
	RevokeBatchSize int
	// ProofBatchSize 证明固化批量处理量（默认 200）。
	ProofBatchSize int
	// AllowCommitWithoutLease 是否允许无 lease 的写提交（必须 false，fail-closed）。
	// 仅集成可设 true；环境禁止。
	AllowCommitWithoutLease bool
}

  func (c Config) withDefaults() Config {
	   if c.LeaseTTL <= 0 {
		   c.LeaseTTL = 120 * time.Second
	   }
	  if c.RevokeBatchSize <= 0 {
		  c.RevokeBatchSize = 200
	   }
	  if c.ProofBatchSize <= 0 {
		  c.ProofBatchSize = 200
	   }
	   return c
   }

// ─── Engine ────────────────────────────────────────────────────────────────
// Engine 探针令牌联合租约核心引擎。
type Engine struct {
	store Store
	checker *writegate.Checker
	cfg     Config
	clock   func() time.Time
}

// NewEngine 初始化 Engine。
// 参数：
// - store：持久化实现。
// - checker：writegate.Checker（含 ProbeMutexReader + GenTokenReader + PoisonProofReader）。
// - cfg：Engine 配置。

   func NewEngine(store Store, checker *writegate.Checker, cfg Config) *Engine {
	  return &Engine{
		  store:   store,
		  checker: checker,
		  cfg:     cfg.withDefaults(),
		  clock: time.Now,
	   }
   }

// WithClock 替换时钟（注入）。

   func (e *Engine) WithClock(fn func() time.Time) *Engine {
	   e.clock = fn
	  return e
   }

   func (e *Engine) now() time.Time {
	  if e.clock != nil {
		   return e.clock()
	  }
	  return time.Now().UTC()
   }

// ─── 辅助方法 ─────────────────────────────────────────────────────────────
// computeProofHash 计算漂移证明指纹（唯一性保证）。
  func computeProofHash(leaseID string, driftType DriftType, oldProbeFP, newProbeFP string, oldGen, newGen int64) string {
	   raw := fmt.Sprintf("%s|%s|%s|%s|%d|%d", leaseID, driftType, oldProbeFP, newProbeFP, oldGen, newGen)
	   sum := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", sum[:])
   }

// scopeFromLease 从租约信息重建 writegate.CheckScope（供复验时传入 Reader）。

  func scopeFromLease(l *ProbeTokenLease) writegate.CheckScope {
	   return writegate.CheckScope{
		   Kind:      writegate.ScopeKind(l.ScopeKind),
		  SubjectID: l.SubjectID,
		  Chain: l.Chain,
		  Asset: l.Asset,
	   }
  }

// ─── JointCheck ───────────────────────────────────────────────────────────
// JointCheckRequest 联合 Check + 签发租约请求。

type JointCheckRequest struct {
	// Scope 写作用域（必填）。
	Scope writegate.CheckScope
	// IdemKey 幂等键（防重复签发；推荐 = SHA256(scope_key + subject_id + request_id)）。
	IdemKey string
}

// JointCheckResult 联合 Check 结果。

type JointCheckResult struct {
	// Lease 签发的联合租约（JointCheck 放行时非空）。
	Lease *ProbeTokenLease
	// Snap 联合 Check 快照（含探针指纹与令牌世代）。
	Snap writegate.WriteGateSnap
	// Allowed Check 是否放行（false 时 Lease 为 nil）。
	Allowed bool
	// RejectReason 拒绝原因（Allowed=false 时非空）。
	RejectReason string
}

// JointCheck 执行联合 Check 并签发 ProbeTokenLease。
// 流程：
// 1. 调用 writegate.Checker.CheckGate（读 ProbeMutex + GenToken，非原子）。
// 2. Check 放行后，将快照指纹写入 ProbeTokenLease（state=checked）。
// 3. 返回 lease_id 供写提交携带。
// 幂等：IdemKey 已存在时返回原租约（不新建）。
// 不变量：
// - Check 拒绝（Allowed=false）时不签发租约。
// - 签发后的租约持有 ProbeFingerprint + TokenGeneration，供 CommitCheck 复验。

   func (e *Engine) JointCheck(ctx context.Context, req JointCheckRequest) (*JointCheckResult, error) {
	  if err := req.Scope.Validate(); err != nil {
		   return nil, err
	  }
	  // 幂等键命中，返回已有租约。
	   if req.IdemKey != "" {
		   if existing, err := e.store.GetLeaseByIdem(ctx, req.IdemKey); err == nil {
			  return &JointCheckResult{
				   Lease:   existing,
				   Allowed: true,
			  }, nil
		  } else if !errors.Is(err, ErrNotFound) {
			   return nil, err
		   }
	  }
	   // 调用 writegate 联合 Check（非原子读探针+令牌）。
	   checkResult,
		   err := e.checker.CheckGate(ctx, req.Scope)
	  if err != nil {
		  reason := "check_error"
		   if checkResult != nil {
			  reason = checkResult.Snap.RejectReason
		   }
		  return &JointCheckResult{
			   Allowed: false,
			   RejectReason: reason,
		   }, err
	   }
	  if !checkResult.Snap.Allowed {
		   return &JointCheckResult{
			   Snap: checkResult.Snap,
			  Allowed:      false,
			  RejectReason: checkResult.Snap.RejectReason,
		   }, nil
	  }
	  // Check 放行，签发联合租约。
	   now := e.now()
	  lease := &ProbeTokenLease{
		  ID:        uuid.NewString(),
		  ScopeKey:  req.Scope.ScopeKey(),
		  ScopeKind: string(req.Scope.Kind),
		  SubjectID: strings.TrimSpace(req.Scope.SubjectID),
		   Chain:            strings.TrimSpace(req.Scope.Chain),
		  Asset:            strings.TrimSpace(req.Scope.Asset),
		   ProbeFingerprint: checkResult.Probe.StateFingerprint,
		   ProbePairID:      checkResult.Probe.PairID,
		  IsProbingAtIssue: checkResult.Probe.IsProbing,
		   TokenGeneration:  checkResult.Token.Generation,
		  TokenHash:        checkResult.Token.TokenHash,
		  SnapFingerprint:  checkResult.Snap.SnapFingerprint,
		  State:            LeaseStateChecked,
		   IdemKey:   req.IdemKey,
		   IssuedAt:  now,
		   ExpiresAt: now.Add(e.cfg.LeaseTTL),
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   if err := e.store.InsertLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("probetokengate: insert lease: %w", err)
	   }
	  return &JointCheckResult{
		  Lease: lease,
		   Snap:    checkResult.Snap,
		  Allowed: true,
	   }, nil
  }

// ─── BindLease ────────────────────────────────────────────────────────────
// BindRequest 写提交绑定请求（携带 probe_token_lease_id）。
type BindRequest struct {
	// LeaseID 联合租约 ID（必填；缺失则 ErrNoLease）。
	LeaseID string
	// CommitRef 写提交单据 ID（必填）。
	CommitRef string
	// Scope 写作用域（用于 lease.ScopeKey）。
	Scope writegate.CheckScope
}

// BindResult BindLease 结果。

type BindResult struct {
	// Lease 更新后的租约（state=bound）。
	Lease *ProbeTokenLease
	// Bind 写入的 WriteScopeBind 记录。
	Bind *WriteScopeBind
}

// BindLease 写提交首次携带 lease_id，绑定 commit_ref（checked → bound）。
// 前置：
// - LeaseID 必须存在且状态为 checked（尚未绑定任何写提交）。
// - Scope 必须与 lease.ScopeKey 一致。
// - CommitRef 必须非空。
// - 租约未过期（TTL 未超限）。
// 副作用（仅写本域）：
// - 更新 ProbeTokenLease state（checked → bound）。
// - 写入 WriteScopeBind（lease_id ↔ commit_ref）。
// 注意：BindLease 不执行探针/令牌复验；复验在 CommitCheck 阶段完成。
// 此设计允许调用方在实际写操作前绑定租约，CommitCheck 在写操作完成后消费。

   func (e *Engine) BindLease(ctx context.Context, req BindRequest) (*BindResult, error) {
	  if req.LeaseID == "" {
		   return nil, ErrNoLease
	   }
	   if req.CommitRef == "" {
		  return nil, fmt.Errorf("probetokengate: commit_ref required")
	  }
	  lease,
		   err := e.store.GetLeaseByID(ctx, req.LeaseID)
	  if err != nil {
		   if errors.Is(err, ErrNotFound) {
			   return nil, ErrNoLease
		  }
		  return nil, err
	   }
	   // 作用域。
	  if req.Scope.SubjectID != "" && lease.SubjectID != strings.TrimSpace(req.Scope.SubjectID) {
		   return nil, ErrSubjectMismatch
	   }
	   if req.Scope.Kind != "" && lease.ScopeKey != req.Scope.ScopeKey() {
		   return nil, ErrScopeMismatch
	   }
	  // 终态检查。
	   now := e.now()
	  switch lease.State {
	   case LeaseStateConsumed:
		   return nil, ErrLeaseConsumed
	   case LeaseStateExpired:
		  return nil, ErrLeaseExpired
	  case LeaseStateRevoked:
		  return nil, ErrLeaseRevoked
	   case LeaseStateBound:
		   // 已绑定：返回已有绑定记录（幂等）。
		   bind, bindErr := e.store.GetBindByLeaseID(ctx, lease.ID)
		   if bindErr != nil {
			   return nil, bindErr
		  }
		  if bind.CommitRef != req.CommitRef {
			  return nil, ErrCommitRefMismatch
		  }
		   return &BindResult{Lease: lease, Bind: bind}, nil
	   }
	   // TTL 检查。
	  if lease.Expired(now) {
		   return nil, ErrLeaseExpired
	   }
	   // checked → bound。
	  t := now
	   lease.State = LeaseStateBound
	  lease.BoundAt = &t
	   lease.CommitRef = req.CommitRef
	   lease.UpdatedAt = now
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		  return nil, err
	  }
	  // 写入 WriteScopeBind。
	  bind := &WriteScopeBind{
		  ID: uuid.NewString(),
		  LeaseID:   lease.ID,
		  CommitRef: req.CommitRef,
		  ScopeKey: lease.ScopeKey,
		   SubjectID: lease.SubjectID,
		  BoundAt:   now,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.InsertBind(ctx, bind); err != nil {
		   return nil, fmt.Errorf("probetokengate: insert bind: %w", err)
	   }
	   return &BindResult{Lease: lease,
		   Bind: bind}, nil
   }

// ─── CommitCheck ──────────────────────────────────────────────────────────
// CommitCheckRequest 写提交最终核验请求。

type CommitCheckRequest struct {
	// LeaseID 联合租约 ID（必填；缺失则 ErrNoLease，fail-closed）。
	LeaseID string
	// CommitRef 写提交单据 ID（bound 状态时须与 lease.CommitRef 一致）。
	CommitRef string
	// Scope 写作用域（用于复验探针+令牌）。
	Scope writegate.CheckScope
}

// CommitCheckResult CommitCheck 结果。

type CommitCheckResult struct {
	// Lease 更新后的租约（通过时 state=consumed；漂移时 state=revoked）。
	Lease *ProbeTokenLease
	// Drifted 是否检测到探针/令牌漂移（true 时 DriftProofID 非空）。
	Drifted bool
	// DriftProofID 关联的 ProbeTokenDriftProof.ID（仅 Drifted=true 时非空）。
	DriftProofID string
	// DriftType 漂移类型（仅 Drifted=true 时非空）。
	DriftType DriftType
}

// CommitCheck 写提交最终核验（复验探针指纹 + 令牌世代，漂移则拒绝并落证明）。
// 前置：
// - LeaseID 必须存在且状态为 checked 或 bound（允许未经 BindLease 直接 CommitCheck）。
// - 无 LeaseID 则 fail-closed（ErrNoLease）。
// - 租约未过期。
// 复验逻辑：
// 1. 读取当前探针状态，与 lease.ProbeFingerprint ；不匹配则漂移（probe_flip）。
// 2. 读取当前令牌世代，与 lease.TokenGeneration ；
// 世代抬升 → token_advance；令牌已撤令 → poison_revoke。
// 3. 漂移则置 revoked + 落 ProbeTokenDriftProof + 返回 ErrProbeDrift/ErrTokenDrift。
// 4. 通过则 bound/checked → consumed，更新 WriteScopeBind.ConsumedAt。
// 隔离：不调用 ledger Apply；不调用 withdraw 广播 FSM。
   func (e *Engine) CommitCheck(ctx context.Context, req CommitCheckRequest) (*CommitCheckResult, error) {
	  if req.LeaseID == "" {
		   if !e.cfg.AllowCommitWithoutLease {
			   return nil, ErrNoLease
		   }
		  // AllowCommitWithoutLease=true 仅供集成；直接放行。
		   return &CommitCheckResult{}, nil
	   }
	   lease, err := e.store.GetLeaseByID(ctx, req.LeaseID)
	   if err != nil {
		  if errors.Is(err, ErrNotFound) {
			  return nil, ErrNoLease
		  }
		   return nil, err
	   }
	   // 终态检查。
	  now := e.now()
	   switch lease.State {
	  case LeaseStateConsumed:
		  return &CommitCheckResult{Lease: lease}, ErrLeaseConsumed
	   case LeaseStateExpired:
		   return &CommitCheckResult{Lease: lease},
			   ErrLeaseExpired
	   case LeaseStateRevoked:
		  proofID := lease.DriftProofID
		   return &CommitCheckResult{Lease: lease, Drifted: true, DriftProofID: proofID}, ErrLeaseRevoked
	   }
	  // TTL 实时检查。
	   if lease.Expired(now) {
		  return &CommitCheckResult{Lease: lease}, ErrLeaseExpired
	   }
	  // commit_ref （bound 状态且 CommitRef 已填写时）。
	  if lease.State == LeaseStateBound && req.CommitRef != "" && lease.CommitRef != req.CommitRef {
		  return nil, ErrCommitRefMismatch
	   }
	  // 重建 scope 供 Reader 使用。
	  scope := scopeFromLease(lease)
	   // ── 复验探针状态 ───────────────────────────────────────────────────────
	   probeState, probeErr := e.checker.ReadProbeState(ctx, scope)
	   if probeErr != nil {
		  return nil, fmt.Errorf("probetokengate: read probe state: %w", probeErr)
	   }
	  probeDrifted := lease.ProbeFingerprint != "" &&
		   probeState.StateFingerprint != lease.ProbeFingerprint
		   // ── 复验令牌世代 ───────────────────────────────────────────────────────
	  tokenSnap, tokenErr := e.checker.ReadGenToken(ctx, scope)
	  if tokenErr != nil {
		  return nil, fmt.Errorf("probetokengate: read gen token: %w", tokenErr)
	   }
	  tokenDrifted := tokenSnap.Generation != lease.TokenGeneration || !tokenSnap.IsValid()
	   // ── 漂移处理 ──────────────────────────────────────────────────────────
	   if probeDrifted || tokenDrifted {
		   // 确定漂移类型（probe_flip 先）。
		  var dType DriftType
		   var driftErr error
		  if probeDrifted {
			  dType = DriftTypeProbeFlip
			   driftErr = ErrProbeDrift
		  } else if tokenSnap.State == writegate.TokenStateRevoked {
			   dType = DriftTypePoisonRevoke
			   driftErr = ErrTokenDrift
		  } else {
			  dType = DriftTypeTokenAdvance
			   driftErr = ErrTokenDrift
		  }
		   proof, proofOpenErr := e.openDriftProof(ctx, lease, probeState, tokenSnap, dType, now)
		  if proofOpenErr != nil && !errors.Is(proofOpenErr, ErrDuplicate) {
			   return nil, proofOpenErr
		   }
		  t := now
		  lease.State = LeaseStateRevoked
		  lease.RevokedAt = &t
		   lease.RevokeReason = string(dType)
		  if proof != nil {
			   lease.DriftProofID = proof.ID
		   }
		   lease.UpdatedAt = now
		   if upErr := e.store.UpdateLease(ctx, lease); upErr != nil {
			  return nil, upErr
		  }
		   proofID := ""
		  if proof != nil {
			  proofID = proof.ID
		  }
		   return &CommitCheckResult{
			   Lease: lease,
			  Drifted: true,
			   DriftProofID: proofID,
			  DriftType:    dType,
		   }, driftErr
	  }
	  // ── 通过：consumed 终态 ────────────────────────────────────────────────
	   t := now
	   lease.State = LeaseStateConsumed
	  lease.ConsumedAt = &t
	  lease.UpdatedAt = now
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		  return nil, err
	   }
	  // 更新 WriteScopeBind.ConsumedAt（若存在绑定记录）。
	  if bind, bindErr := e.store.GetBindByLeaseID(ctx, lease.ID); bindErr == nil {
		   ct := now
		   bind.ConsumedAt = &ct
		  bind.UpdatedAt = now
		  _ = e.store.UpdateBind(ctx, bind)
	   }
	   return &CommitCheckResult{Lease: lease}, nil
  }

// openDriftProof 开启漂移证明（幂等：已存在则返回原证明）。

  func (e *Engine) openDriftProof(
	   ctx context.Context,
	   lease *ProbeTokenLease,
	  probeState writegate.ProbeState,
	  tokenSnap writegate.GenTokenSnap,
	   dType DriftType,
	  now time.Time,
   ) (*ProbeTokenDriftProof, error) {
	  idem := fmt.Sprintf("drift:%s:%s:%d", lease.ID, dType, tokenSnap.Generation)
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		  return existing, nil
	  } else if !errors.Is(err, ErrNotFound) {
		  return nil, err
	  }
	   hash := computeProofHash(
		   lease.ID, dType,
		   lease.ProbeFingerprint, probeState.StateFingerprint,
		  lease.TokenGeneration, tokenSnap.Generation,
	   )
	   proof := &ProbeTokenDriftProof{
		  ID:      uuid.NewString(),
		  LeaseID: lease.ID,
		   ScopeKey:  lease.ScopeKey,
		   SubjectID: lease.SubjectID,
		   DriftType: dType,
		  OldProbeFingerprint: lease.ProbeFingerprint,
		   NewProbeFingerprint: probeState.StateFingerprint,
		  OldTokenGeneration: lease.TokenGeneration,
		  NewTokenGeneration: tokenSnap.Generation,
		  PoisonRevoked:      tokenSnap.State == writegate.TokenStateRevoked,
		  State: ProofStateOpen,
		  ProofHash: hash,
		   IdemKey:  idem,
		   OpenedAt: now,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := e.store.InsertProof(ctx, proof); err != nil {
		  return nil, err
	   }
	  return proof, nil
  }

// ─── ReadProbeState / ReadGenToken 转发方法 ───────────────────────────────
// ReadProbeState 通过 Checker 转发只读探针状态读取（供 CommitCheck 内部使用）。
  func (e *Engine) ReadProbeState(ctx context.Context, scope writegate.CheckScope) (writegate.ProbeState, error) {
	   result,
		  err := e.checker.CheckGate(ctx, scope)
	  if err != nil && result == nil {
		  return writegate.ProbeState{}, err
	  }
	  if result != nil {
		   return result.Probe, nil
	  }
	  return writegate.ProbeState{}, nil
  }

// ReadGenToken 通过 Checker 转发只读令牌世代读取（供 CommitCheck 内部使用）。

   func (e *Engine) ReadGenToken(ctx context.Context, scope writegate.CheckScope) (writegate.GenTokenSnap, error) {
	  result, err := e.checker.CheckGate(ctx, scope)
	  if err != nil && result == nil {
		  return writegate.GenTokenSnap{}, err
	   }
	  if result != nil {
		  return result.Token, nil
	   }
	   return writegate.GenTokenSnap{},
		   nil
   }

// ─── RevokeOnProbeFlip ────────────────────────────────────────────────────
// ProbeFlipEvent 探针翻转事件（由 ProbeMutex.MarkProbe/ClearProbe 触发）。
type ProbeFlipEvent struct {
	// ScopeKey 受影响写作用域键（可为空，表示全局翻转）。
	ScopeKey string
	// PairID 探针对 ID。
	PairID string
	// NewProbeFingerprint 翻转后的新探针指纹。
	NewProbeFingerprint string
	// Actor 触发翻转的操作人/系统标识。
	Actor string
}

// RevokeOnProbeFlip 探针翻转通知：批量吊销指定 scope 的活跃租约并落漂移证明。
// 触发场景：ProbeMutex.MarkProbe(pair) 或 ClearProbe 后发布事件。
// 语义：Check 放行后探针翻转 → 旧租约全部失效；持旧 lease 的写提交必须拒绝。
// 副作用（仅写本域）：
// - 批量将 checked/bound 租约置 revoked。
// - 为每个吊销租约开 ProbeTokenDriftProof（drift_type=probe_flip）。
  func (e *Engine) RevokeOnProbeFlip(ctx context.Context, ev ProbeFlipEvent) (int, error) {
	   now := e.now()
	   leases, err := e.store.ListActiveLeasesByScope(ctx, ev.ScopeKey, e.cfg.RevokeBatchSize)
	   if err != nil {
		  return 0, fmt.Errorf("probetokengate: list leases for probe flip revoke: %w", err)
	  }
	  revoked := 0
	  for _, lease := range leases {
		   if lease.State.Closed() {
			  continue
		   }
		  // 仅吊销指纹不匹配的租约（指纹已匹配说明此次翻转不影响该租约）。
		  if ev.NewProbeFingerprint != "" && lease.ProbeFingerprint == ev.NewProbeFingerprint {
			  continue
		  }
		  // 开漂移证明。
		   fakeTokenSnap := writegate.GenTokenSnap{
			  Generation: lease.TokenGeneration,
			   State:      writegate.TokenStateActive,
		   }
		  fakeProbeSate := writegate.ProbeState{
			  StateFingerprint: ev.NewProbeFingerprint,
		  }
		  proof, proofErr := e.openDriftProof(ctx, lease, fakeProbeSate, fakeTokenSnap, DriftTypeProbeFlip, now)
		  if proofErr != nil && !errors.Is(proofErr, ErrDuplicate) {
			  return revoked, proofErr
		   }
		   t := now
		  lease.State = LeaseStateRevoked
		  lease.RevokedAt = &t
		  lease.RevokeReason = string(RevokeReasonProbeFlip)
		  if proof != nil {
			  lease.DriftProofID = proof.ID
		  }
		  lease.UpdatedAt = now
		   if upErr := e.store.UpdateLease(ctx, lease); upErr != nil {
			   return revoked, upErr
		  }
		  revoked++
	   }
	   return revoked, nil
   }

// ─── RevokeOnPoisonRevoke ─────────────────────────────────────────────────
// PoisonRevokeEvent 毒价撤令事件（由毒价引擎发布，只读消费）。
type PoisonRevokeEvent struct {
	// ScopeKey 受影响写作用域键（可为空，表示全局撤令）。
	ScopeKey string
	// NewTokenGeneration 撤令后的新令牌世代。
	NewTokenGeneration int64
	// Actor 触发撤令的操作人/系统标识。
	Actor string
}

// RevokeOnPoisonRevoke 毒价撤令通知：批量吊销活跃租约并落漂移证明（poison_revoke）。
// 触发场景：毒价引擎执行令牌撤令后发布事件（只读消费，不改写 PoisonEngine）。
// 语义：Check 放行后毒价令牌撤令 → 旧租约全部失效，持旧 lease 的写提交必须拒绝。
// 副作用（仅写本域）：
// - 批量将 checked/bound 租约置 revoked。
// - 为每个吊销租约开 ProbeTokenDriftProof（drift_type=poison_revoke）。
  func (e *Engine) RevokeOnPoisonRevoke(ctx context.Context, ev PoisonRevokeEvent) (int, error) {
	   now := e.now()
	  leases, err := e.store.ListActiveLeasesByScope(ctx, ev.ScopeKey, e.cfg.RevokeBatchSize)
	   if err != nil {
		  return 0, fmt.Errorf("probetokengate: list leases for poison revoke: %w", err)
	   }
	   revoked := 0
	   for _, lease := range leases {
		   if lease.State.Closed() {
			   continue
		   }
		  // 仅吊销旧世代租约（世代已等于新世代说明不受影响）。
		   if lease.TokenGeneration == ev.NewTokenGeneration {
			  continue
		  }
		   // 开漂移证明（poison_revoke）。
		  fakeTokenSnap := writegate.GenTokenSnap{
			   Generation: ev.NewTokenGeneration,
			   State:      writegate.TokenStateRevoked,
		  }
		  fakeProbeSate := writegate.ProbeState{
			  StateFingerprint: lease.ProbeFingerprint,
		   }
		   proof, proofErr := e.openDriftProof(ctx, lease, fakeProbeSate, fakeTokenSnap, DriftTypePoisonRevoke, now)
		  if proofErr != nil && !errors.Is(proofErr, ErrDuplicate) {
			  return revoked, proofErr
		  }
		   t := now
		  lease.State = LeaseStateRevoked
		  lease.RevokedAt = &t
		   lease.RevokeReason = string(RevokeReasonPoisonRevoke)
		  if proof != nil {
			  lease.DriftProofID = proof.ID
		   }
		  lease.UpdatedAt = now
		   if upErr := e.store.UpdateLease(ctx, lease); upErr != nil {
			   return revoked, upErr
		   }
		  revoked++
	  }
	  return revoked, nil
  }

// ─── ExpireLease ──────────────────────────────────────────────────────────
// ExpireLease TTL 超限租约置 expired（LeaseExpiryWorker 调用）。
// 幂等：已终态则静默返回。

   func (e *Engine) ExpireLease(ctx context.Context, leaseID string) error {
	   lease, err := e.store.GetLeaseByID(ctx, leaseID)
	  if err != nil {
		  return err
	   }
	  if lease.State.Closed() {
		   return nil // 已终态，幂等
	   }
	  now := e.now()
	  lease.State = LeaseStateExpired
	   lease.RevokeReason = string(RevokeReasonExpiry)
	  lease.UpdatedAt = now
	  return e.store.UpdateLease(ctx, lease)
  }

// ─── RevokeLease 手动吊销 ─────────────────────────────────────────────────
// RevokeLease 手动吊销活跃租约（运营/管理员操作）。
// 幂等：已终态则静默返回。

  func (e *Engine) RevokeLease(ctx context.Context, leaseID, reason string) (*ProbeTokenLease, error) {
	   lease, err := e.store.GetLeaseByID(ctx, leaseID)
	  if err != nil {
		  return nil, err
	  }
	  if lease.State.Closed() {
		  return lease, nil // 幂等
	   }
	  now := e.now()
	   t := now
	  lease.State = LeaseStateRevoked
	  lease.RevokedAt = &t
	   lease.RevokeReason = reason
	   lease.UpdatedAt = now
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		   return nil, err
	  }
	   return lease, nil
   }

// ─── Proof 管理 ───────────────────────────────────────────────────────────
// ProofSummary 漂移证明审计摘要（不含敏感认证材料）。

type ProofSummary struct {
	ProofID string `json:"proof_id"`
	LeaseID  string `json:"lease_id"`
	ScopeKey string `json:"scope_key"`
	SubjectID string `json:"subject_id"`
	DriftType           string `json:"drift_type"`
	OldProbeFingerprint string `json:"old_probe_fingerprint"`
	NewProbeFingerprint string `json:"new_probe_fingerprint"`
	OldTokenGeneration int64 `json:"old_token_generation"`
	NewTokenGeneration int64  `json:"new_token_generation"`
	PoisonRevoked      bool   `json:"poison_revoked"`
	State              string `json:"state"`
	ProofHash string     `json:"proof_hash"`
	OpenedAt  time.Time  `json:"opened_at"`
	ProvenAt  *time.Time `json:"proven_at,omitempty"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	ClosedBy  string     `json:"closed_by,omitempty"`
}

// ExportProof 导出漂移证明审计摘要（只读，供审计/运营）。
   func (e *Engine) ExportProof(ctx context.Context, proofID string) (*ProofSummary, error) {
	  p, err := e.store.GetProofByID(ctx, proofID)
	  if err != nil {
		  return nil, err
	   }
	  return proofToSummary(p), nil
   }

   func proofToSummary(p *ProbeTokenDriftProof) *ProofSummary {
	   return &ProofSummary{
		  ProofID: p.ID,
		   LeaseID:  p.LeaseID,
		  ScopeKey: p.ScopeKey,
		   SubjectID: p.SubjectID,
		  DriftType:           string(p.DriftType),
		   OldProbeFingerprint: p.OldProbeFingerprint,
		   NewProbeFingerprint: p.NewProbeFingerprint,
		  OldTokenGeneration:  p.OldTokenGeneration,
		  NewTokenGeneration: p.NewTokenGeneration,
		  PoisonRevoked: p.PoisonRevoked,
		  State: string(p.State),
		   ProofHash: p.ProofHash,
		  OpenedAt: p.OpenedAt,
		   ProvenAt: p.ProvenAt,
		   ClosedAt: p.ClosedAt,
		  ClosedBy: p.ClosedBy,
	   }
   }

// ProveProof 固化证明（open → proven，由 ProofWorker 调用）。
   func (e *Engine) ProveProof(ctx context.Context, proofID string) (*ProbeTokenDriftProof, error) {
	   p, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		   return nil, err
	  }
	   if p.State == ProofStateProven || p.State == ProofStateClosed {
		   return p, nil // 幂等
	  }
	   if p.State != ProofStateOpen {
		  return nil, fmt.Errorf("%w: from=%s to=proven", ErrInvalidTransit, p.State)
	   }
	   now := e.now()
	   t := now
	   p.State = ProofStateProven
	  p.ProvenAt = &t
	  p.UpdatedAt = now
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		   return nil, err
	  }
	   return p, nil
   }

// ConfirmProof 管理员确认证明（proven → closed）。
   func (e *Engine) ConfirmProof(ctx context.Context, proofID, closedBy, notes string) (*ProbeTokenDriftProof, error) {
	   p, err := e.store.GetProofByID(ctx, proofID)
	  if err != nil {
		   return nil, err
	  }
	   if p.State == ProofStateClosed {
		  return p, nil // 幂等
	  }
	  if p.State != ProofStateProven {
		   return nil, fmt.Errorf("%w: from=%s to=closed", ErrInvalidTransit, p.State)
	  }
	   now := e.now()
	   t := now
	  p.State = ProofStateClosed
	   p.ClosedAt = &t
	  p.ClosedBy = closedBy
	   p.Notes = notes
	  p.UpdatedAt = now
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		  return nil, err
	   }
	  return p, nil
   }

// DismissProof 驳回证明（open/proven → closed，无需四眼）。

  func (e *Engine) DismissProof(ctx context.Context, proofID, closedBy, notes string) (*ProbeTokenDriftProof, error) {
	  p,
		   err := e.store.GetProofByID(ctx, proofID)
	  if err != nil {
		   return nil, err
	   }
	   if p.State == ProofStateClosed {
		  return p, nil // 幂等
	   }
	   now := e.now()
	   t := now
	  p.State = ProofStateClosed
	   p.ClosedAt = &t
	   p.ClosedBy = closedBy
	  p.Notes = notes
	   p.UpdatedAt = now
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		  return nil, err
	   }
	  return p, nil
   }

// GetLease 查询租约（只读）。
   func (e *Engine) GetLease(ctx context.Context, leaseID string) (*ProbeTokenLease, error) {
	   return e.store.GetLeaseByID(ctx,
		   leaseID)
  }

// GetProof 查询证明（只读）。
   func (e *Engine) GetProof(ctx context.Context, proofID string) (*ProbeTokenDriftProof, error) {
	   return e.store.GetProofByID(ctx, proofID)
   }

// ListActiveLeasesByScope 列举 scope 维度的活跃租约（只读）。
  func (e *Engine) ListActiveLeasesByScope(ctx context.Context, scopeKey string, limit int) ([]*ProbeTokenLease, error) {
	   return e.store.ListActiveLeasesByScope(ctx, scopeKey, limit)
   }

// ListProofsByState 按状态列举证明（只读）。
  func (e *Engine) ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*ProbeTokenDriftProof, error) {
	   return e.store.ListProofsByState(ctx, state, limit)
  }

// GetBind 查询写作用域绑定记录（只读）。

   func (e *Engine) GetBind(ctx context.Context, leaseID string) (*WriteScopeBind, error) {
	  return e.store.GetBindByLeaseID(ctx, leaseID)
  }
