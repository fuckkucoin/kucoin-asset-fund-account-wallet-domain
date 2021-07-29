// @FileName: engine.go
// @Summary: invleasegate 核心引擎——预检租约签发、提交复验、建议翻转守卫、fail-closed 持久化。
// - Engine 不持有出金/归集业务逻辑；只决策是否允许提交，由上层调用实际出金路径。
// - Engine 不改写 invdrift 建议状态机；只通过 DriftChecker 只读查询 HasActiveAdvice。
// - PrecheckBind：DriftGate.Check 放行后签发 InvPrecheckLease（含 AdviceFingerprint）。
// checker 不可用时签发被 fail-closed 拒绝，返回 ErrCheckerDown。
// - CommitRecheck：提交路径必须携带合法未过期租约；复验当前建议指纹；漂移拒并落 InvGateDriftProof。
// - AdviceFlipGuard：后台巡检，租约期内建议翻转时推进租约到 blocked 并落 proof。
// - FailClosedCommit：已签发租约在 checker 故障时提交，fail-closed（blocked + proof）。
// 不变量：
// - PrecheckBind 返回 nil 时，租约状态为 issued。
// - CommitRecheck 返回 nil 时，租约状态为 committed。
// - 任何拒绝路径必须同步或最终落 InvGateDriftProof。
// - BlockLease 为幂等操作；同一 lease 已 blocked 时返回 nil。
// @Author: KianFord
// @Created: 2021.07.28

package invleasegate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	defaultLeaseTTL = 300 * time.Second // 预检租约默认 TTL：5 分钟
	maxLeaseTTL = 1800 * time.Second // 最大 TTL：30 分钟
	minLeaseTTL = 30 * time.Second   // 最小 TTL：30 秒
)

// DriftChecker 封堵建议只读查询接口（由 invdrift.Service 实现）。
// Engine 通过此接口判断是否存在活跃封堵建议。

type DriftChecker interface {
	// HasActiveAdvice 返回指定资产是否存在 proposed 建议（只读）。
	HasActiveAdvice(ctx context.Context, asset string) (bool, error)
}

// EngineOption Engine 配置选项。
type EngineOption func(*Engine)

// WithLeaseTTL 设置预检租约默认 TTL。
  func WithLeaseTTL(d time.Duration) EngineOption {
	  return func(e *Engine) {
		   if d >= minLeaseTTL && d <= maxLeaseTTL {
			  e.leaseTTL = d
		   }
	   }
  }

// WithClock 注入时钟（用）。

  func WithClock(fn func() time.Time) EngineOption {
	  return func(e *Engine) {
		  if fn != nil {
			   e.clock = fn
		   }
	   }
}

// Engine invleasegate 核心引擎。
type Engine struct {
	store Store
	checker DriftChecker
	leaseTTL time.Duration
	clock    func() time.Time
}

// NewEngine 构造 Engine。
// checker 为 nil 时 PrecheckBind 始终返回 ErrCheckerDown（fail-closed）。
   func NewEngine(store Store, checker DriftChecker, opts ...EngineOption) *Engine {
	   if store == nil {
		  store = NewMemStore()
	   }
	   e := &Engine{
		  store:   store,
		   checker: checker,
		  leaseTTL: defaultLeaseTTL,
		   clock:    func() time.Time { return time.Now().UTC() },
	   }
	  for _, o := range opts {
		   o(e)
	   }
	   return e
  }

// ─── 预检绑定（Precheck Bind Lease） ─────────────────────────────────────────
// PrecheckBindReq PrecheckBind 请求参数。
type PrecheckBindReq struct {
	Asset string
	Actor string
	LeaseTTL time.Duration // 0 表示使用 Engine 默认值
}

// PrecheckBindResult PrecheckBind 成功结果。

type PrecheckBindResult struct {
	Lease *InvPrecheckLease
}

// PrecheckBind DriftGate.Check 放行后签发预检租约。
// 流程：
// 1. 验证 Asset/Actor；
// 2. 查询 checker.HasActiveAdvice——不可用时 fail-closed，返回 ErrCheckerDown；
// 3. 存在活跃建议时返回 ErrActiveAdvice；
// 4. 签发 InvPrecheckLease（state=issued，含 AdviceFingerprint 快照）；
// 5. 持久化到 Store。
  func (e *Engine) PrecheckBind(ctx context.Context, req PrecheckBindReq) (PrecheckBindResult, error) {
	  asset, err := normalizeAsset(req.Asset)
	   if err != nil {
		  return PrecheckBindResult{}, ErrAsset
	   }
	  actor, err := normalizeActor(req.Actor)
	   if err != nil {
		  return PrecheckBindResult{}, ErrActor
	   }
	  // Fail-closed：checker 不可用时拒绝签发
	   if e.checker == nil {
		  return PrecheckBindResult{},
			  ErrCheckerDown
	   }
	   hasActive, checkErr := e.checker.HasActiveAdvice(ctx, asset)
	  if checkErr != nil {
		   // checker 查询失败，fail-closed
		  return PrecheckBindResult{}, ErrCheckerDown
	  }
	   if hasActive {
		   return PrecheckBindResult{},
			   ErrActiveAdvice
	  }
	   // 采集建议指纹快照
	  fp := AdviceFingerprint{
		  Asset:     asset,
		  HasActive: false,
		  AdviceHash: "",
		  CapturedAt: e.clock(),
	  }
	   now := e.clock()
	  ttl := req.LeaseTTL
	   if ttl < minLeaseTTL || ttl > maxLeaseTTL {
		   ttl = e.leaseTTL
	  }
	  lease := &InvPrecheckLease{
		  LeaseID: uuid.NewString(),
		  Asset: asset,
		  Actor: actor,
		   State:              LeaseIssued,
		   FingerprintAtIssue: fp,
		   IssuedAt:           now,
		  ExpiresAt:          now.Add(ttl),
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.SaveLease(ctx, lease); err != nil {
		  return PrecheckBindResult{}, err
	  }
	  PrecheckLeaseTotal.WithLabelValues("issued").Inc()
	  cp := lease.Copy()
	   return PrecheckBindResult{Lease: &cp}, nil
  }

// ─── 提交复验（Commit Recheck） ────────────────────────────────────────────────
// CommitRecheckReq CommitRecheck 请求参数。
type CommitRecheckReq struct {
	LeaseID string // inv_precheck_id（必填）
	Asset   string // 提交路径的资产（用于交叉校验）
	Actor   string
	CommitRef string // 出金/归集业务 ID（可选，仅供追踪）
}

// CommitRecheckResult CommitRecheck 成功结果。

type CommitRecheckResult struct {
	Lease *InvPrecheckLease
}

// CommitRecheck 出金/超额归集提交前的预检租约复验。
// 流程：
// 1. 按 LeaseID 查询租约；不存在返回 ErrNoLease；
// 2. 资产校验（提交路径资产必须与租约资产一致）；
// 3. 过期检测（ExpiresAt <= now）：expired+proof；
// 4. 终态检测（blocked/committed/expired）：各自对应错误；
// 5. 查询当前 checker.HasActiveAdvice——不可用时 fail-closed（blocked+proof）；
// 6. 建议翻转检测（指纹核对）：flip 时 blocked+proof；
// 7. 验证通过：推进租约到 committed。
  func (e *Engine) CommitRecheck(ctx context.Context, req CommitRecheckReq) (CommitRecheckResult, error) {
	   if strings.TrimSpace(req.LeaseID) == "" {
		  return CommitRecheckResult{}, ErrNoLease
	   }
	  lease, err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   return CommitRecheckResult{}, ErrNoLease
	  }
	   // 资产交叉校验
	  if req.Asset != "" {
		   asset, asErr := normalizeAsset(req.Asset)
		  if asErr != nil {
			   return CommitRecheckResult{}, ErrAsset
		   }
		   if asset != lease.Asset {
			   return CommitRecheckResult{}, ErrAsset
		   }
	  }
	  now := e.clock()
	   // 过期检测（即使状态仍为 issued/bound，ExpiresAt 已过也必须拒绝）
	   if !now.Before(lease.ExpiresAt) && !lease.State.Terminal() {
		   _ = e.blockLeaseWithProof(ctx, lease, DriftCommitNoLease, req.Actor, req.CommitRef,
			   "lease expired at commit time", nil, nil)
		  return CommitRecheckResult{}, ErrLeaseExpired
	  }
	   switch lease.State {
	  case LeaseCommitted:
		  return CommitRecheckResult{}, ErrLeaseTerminal
	  case LeaseExpired:
		  return CommitRecheckResult{}, ErrLeaseExpired
	  case LeaseBlocked:
		  return CommitRecheckResult{}, ErrLeaseBlocked
	   case LeaseIssued, LeaseBound:
		  // 接着复验
	   default:
		   return CommitRecheckResult{}, ErrLeaseTerminal
	  }
	  // checker fail-closed
	   if e.checker == nil {
		  _ = e.blockLeaseWithProof(ctx,
			   lease, DriftCheckerDown, req.Actor, req.CommitRef,
			  "checker unavailable at commit", &lease.FingerprintAtIssue, nil)
		  return CommitRecheckResult{}, ErrCheckerDown
	  }
	  hasActive, checkErr := e.checker.HasActiveAdvice(ctx, lease.Asset)
	   if checkErr != nil {
		   _ = e.blockLeaseWithProof(ctx,
			  lease, DriftCheckerDown, req.Actor, req.CommitRef,
			   "checker query failed at commit", &lease.FingerprintAtIssue, nil)
		  return CommitRecheckResult{}, ErrCheckerDown
	   }
	  // 采集提交时指纹
	  currentFP := AdviceFingerprint{
		   Asset:      lease.Asset,
		  HasActive:  hasActive,
		  AdviceHash: adviceHash(hasActive),
		  CapturedAt: now,
	  }
	   // 建议翻转检测：无→有 或 有→无 均视为翻转
	  if !lease.FingerprintAtIssue.Equal(currentFP) {
		  _ = e.blockLeaseWithProof(ctx, lease, DriftCommitFingerMismatch, req.Actor, req.CommitRef,
			   "advice fingerprint mismatch at commit", &lease.FingerprintAtIssue, &currentFP)
		   return CommitRecheckResult{}, ErrFingerprintMismatch
	   }
	  // 若当前有活跃建议（即使指纹与签发时一致，理论上不可能走到这里，但防御）
	  if hasActive {
		  _ = e.blockLeaseWithProof(ctx, lease, DriftAdviceFlip, req.Actor, req.CommitRef,
			  "active advice at commit", &lease.FingerprintAtIssue, &currentFP)
		   return CommitRecheckResult{}, ErrAdviceFlip
	   }
	  // 验证通过：推进到 committed
	  committedAt := now
	  patch := lease.Copy()
	   patch.State = LeaseCommitted
	   patch.CommittedAt = &committedAt
	  patch.CommitRef = req.CommitRef
	   patch.UpdatedAt = now
	  if err := e.store.UpdateLeaseState(ctx, lease.LeaseID, lease.State, LeaseCommitted, &patch); err != nil {
		   return CommitRecheckResult{}, err
	   }
	  PrecheckLeaseTotal.WithLabelValues("committed").Inc()
	   cp := patch.Copy()
	   return CommitRecheckResult{Lease: &cp}, nil
  }

// ─── 建议翻转守卫（Advice Flip Guard） ────────────────────────────────────────
// AdviceFlipGuardReq AdviceFlipGuard 请求参数（供 Worker 调用）。
type AdviceFlipGuardReq struct {
	LeaseID string
	Actor string // 巡检系统 actor
}

// AdviceFlipGuard 对单条活跃租约检测建议翻转，翻转时推进到 blocked 并落 proof。
// Worker 按固定间隔批量调用；也可由外部触发单条检测。
// 已终态租约直接返回 nil（幂等）。
   func (e *Engine) AdviceFlipGuard(ctx context.Context, req AdviceFlipGuardReq) error {
	  lease, err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   return err
	  }
	   if lease.State.Terminal() {
		   return nil
	   }
	  now := e.clock()
	  // 过期检测
	   if !now.Before(lease.ExpiresAt) {
		  return e.expireLease(ctx,
			   lease, now)
	   }
	  if e.checker == nil {
		   // checker 不可用：fail-closed，封堵已签发租约
		  return e.blockLeaseWithProof(ctx, lease, DriftCheckerDown, req.Actor, "",
			   "checker unavailable during flip guard", &lease.FingerprintAtIssue, nil)
	   }
	   hasActive, checkErr := e.checker.HasActiveAdvice(ctx, lease.Asset)
	  if checkErr != nil {
		  return e.blockLeaseWithProof(ctx, lease, DriftCheckerDown, req.Actor, "",
			   "checker query failed during flip guard", &lease.FingerprintAtIssue, nil)
	   }
	   currentFP := AdviceFingerprint{
		  Asset: lease.Asset,
		  HasActive: hasActive,
		   AdviceHash: adviceHash(hasActive),
		  CapturedAt: now,
	  }
	  if !lease.FingerprintAtIssue.Equal(currentFP) {
		   AdviceFlipTotal.WithLabelValues(lease.Asset).Inc()
		  return e.blockLeaseWithProof(ctx, lease, DriftAdviceFlip, req.Actor, "",
			   "advice flip detected during guard scan", &lease.FingerprintAtIssue, &currentFP)
	  }
	   return nil
   }

// ─── 内部工具 ──────────────────────────────────────────────────────────────────
// blockLeaseWithProof 原子封堵租约并落漂移证明。
// 若租约已处于终态，幂等返回 nil（不重复落证）。

   func (e *Engine) blockLeaseWithProof(
	   ctx context.Context,
	  lease *InvPrecheckLease,
	  kind DriftKind,
	   actor, commitRef, note string,
	  fpIssue, fpCommit *AdviceFingerprint,
  ) error {
	  if lease.State.Terminal() {
		  return nil
	  }
	  now := e.clock()
	   blockedAt := now
	  patch := lease.Copy()
	   patch.State = LeaseBlocked
	  patch.BlockedAt = &blockedAt
	  patch.BlockReason = string(kind)
	   patch.UpdatedAt = now
	   // CAS 推进租约状态；若已被并发封堵（ErrInvalidTransition），幂等返回
	  if err := e.store.UpdateLeaseState(ctx, lease.LeaseID, lease.State, LeaseBlocked, &patch); err != nil {
		   if err == ErrInvalidTransition {
			   return nil
		  }
		   return err
	   }
	  PrecheckLeaseTotal.WithLabelValues("blocked").Inc()
	   // 落漂移证明（幂等：ErrAlreadyProven 时忽略）
	   proof := &InvGateDriftProof{
		  ProofID: uuid.NewString(),
		  LeaseID: lease.LeaseID,
		   Asset:   lease.Asset,
		   Kind:    kind,
		   State:   ProofOpen,
		  Actor: actor,
		   CommitRef: commitRef,
		   Note:      note,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if fpIssue != nil {
		  cp := *fpIssue
		  proof.FingerprintAtIssue = &cp
	   }
	   if fpCommit != nil {
		  cp := *fpCommit
		  proof.FingerprintAtCommit = &cp
	   }
	  if err := e.store.SaveProof(ctx, proof); err != nil && err != ErrAlreadyProven {
		   // 证明落地失败不阻断封堵结果；记录指标后返回
		  DriftProofTotal.WithLabelValues(string(kind), "fail").Inc()
		   return nil
	   }
	  DriftProofTotal.WithLabelValues(string(kind), "ok").Inc()
	  return nil
   }

// expireLease 将租约推进到 expired 状态（Worker 调用）。

   func (e *Engine) expireLease(ctx context.Context, lease *InvPrecheckLease, now time.Time) error {
	  if lease.State.Terminal() {
		  return nil
	  }
	   expiredAt := now
	   patch := lease.Copy()
	   patch.State = LeaseExpired
	   patch.ExpiredAt = &expiredAt
	   patch.UpdatedAt = now
	   if err := e.store.UpdateLeaseState(ctx, lease.LeaseID, lease.State, LeaseExpired, &patch); err != nil {
		   if err == ErrInvalidTransition {
			   return nil
		  }
		   return err
	   }
	  PrecheckLeaseTotal.WithLabelValues("expired").Inc()
	  return nil
   }

// CloseProof 审计确认后关闭漂移证明（open→closed）。

   func (e *Engine) CloseProof(ctx context.Context, proofID string, actor string) (*InvGateDriftProof, error) {
	   proof, err := e.store.GetProof(ctx, proofID)
	   if err != nil {
		   return nil, err
	   }
	   if proof.State == ProofClosed {
		  cp := proof.Copy()
		  return &cp, nil
	  }
	  now := e.clock()
	   closedAt := now
	  patch := proof.Copy()
	   patch.State = ProofClosed
	   patch.ClosedAt = &closedAt
	   patch.UpdatedAt = now
	   if err := e.store.UpdateProofState(ctx, proofID, ProofOpen, ProofClosed, &patch); err != nil {
		   return nil, err
	   }
	  cp := patch.Copy()
	  return &cp, nil
  }

// GetLease 查询租约（只读）。
  func (e *Engine) GetLease(ctx context.Context, leaseID string) (*InvPrecheckLease, error) {
	  return e.store.GetLease(ctx, leaseID)
   }

// GetProof 查询漂移证明（只读）。
   func (e *Engine) GetProof(ctx context.Context, proofID string) (*InvGateDriftProof, error) {
	  return e.store.GetProof(ctx, proofID)
   }

// GetProofByLease 按租约 ID 查询漂移证明（只读）。
  func (e *Engine) GetProofByLease(ctx context.Context, leaseID string) (*InvGateDriftProof, error) {
	  return e.store.GetProofByLease(ctx, leaseID)
   }

// ListLeasesByAsset 查询资产下的租约（只读）。
   func (e *Engine) ListLeasesByAsset(ctx context.Context, asset string, limit int) ([]*InvPrecheckLease, error) {
	   return e.store.ListLeasesByAsset(ctx, asset, limit)
   }

// ListOpenProofs 查询 open 状态的漂移证明（只读）。
  func (e *Engine) ListOpenProofs(ctx context.Context, limit int) ([]*InvGateDriftProof, error) {
	  return e.store.ListOpenProofs(ctx, limit)
   }

// ─── 工具函数 ──────────────────────────────────────────────────────────────────
// normalizeAsset 规范化资产标识（大写 + trim）；空字符串返回 ErrAsset。
   func normalizeAsset(s string) (string, error) {
	   s = strings.ToUpper(strings.TrimSpace(s))
	  if s == "" || len(s) > 16 {
		  return "", ErrAsset
	  }
	  return s, nil
  }

// normalizeActor 规范化操作人（trim）；空字符串返回 ErrActor。
   func normalizeActor(s string) (string, error) {
	   s = strings.TrimSpace(s)
	  if s == "" || len(s) > 128 {
		  return "", ErrActor
	   }
	   return s, nil
  }

// adviceHash 根据 hasActive 状态生成简单的指纹摘要（中应由 invdrift.Service 提供完整摘要）。
// 此处仅以 hasActive 状态本身作为稳定 hash，确保翻转可检测。
   func adviceHash(hasActive bool) string {
	  if !hasActive {
		   return ""
	   }
	  // 有活跃建议时，取状态标记的 sha256 hex（前缀标识，实际场景由 invdrift 提供排序后 ID 列表的 hash）
	  sum := sha256.Sum256([]byte("has_active"))
	   return fmt.Sprintf("%x", sum[:8])
  }
