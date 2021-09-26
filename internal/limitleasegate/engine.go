package limitleasegate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/limitgate"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

// ─── 配置 ─────────────────────────────────────────────────────────────────
// Config Engine 配置项。

type Config struct {
	// LeaseTTL 限额预检租约有效期（默认 120s；Check→占用提交必须在此时间内完成）。
	// 过短会导致正常占用提交超时；过长会扩大策略代次翻转窗口。建议 60–120s。
	LeaseTTL time.Duration
	// RevokeBatchSize 策略代次翻转批量吊销每批处理量（默认 200）。
	RevokeBatchSize int
	// ProofBatchSize 证明固化批量处理量（默认 200）。
	ProofBatchSize int
	// AllowCommitWithoutLease 是否允许无 lease 的占用提交（必须 false，fail-closed）。
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
// Engine 限额预检租约核心引擎。
type Engine struct {
	store Store
	reg *policy.Registry
	use *usage.Store
	cfg Config
	clock func() time.Time
}

// NewEngine 初始化 Engine。
// 参数：
// - store：持久化实现（使用 SQLStore；使用 MemStore）。
// - reg：policy.Registry 只读消费，获取当前策略代次与限额参数。
// - use：usage.Store 只读消费，获取 TodaySum/Velocity 快照。
// - cfg：Engine 配置。
  func NewEngine(store Store, reg *policy.Registry, use *usage.Store, cfg Config) *Engine {
	  return &Engine{
		  store: store,
		   reg:   reg,
		  use: use,
		  cfg: cfg.withDefaults(),
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
   func computeProofHash(leaseID string, dt DriftType, oldPVer, newPVer int, oldSum, newSum string) string {
	  raw := fmt.Sprintf("%s|%s|%d|%d|%s|%s", leaseID, dt, oldPVer, newPVer, oldSum, newSum)
	   sum := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", sum[:])
  }

// usageKind 将策略 Kind 映射到 usage.Kind。
  func usageKind(kind string) usage.Kind {
	   switch strings.ToLower(kind) {
	   case "withdraw":
		   return usage.KindWithdraw
	   case "deposit":
		   return usage.KindDeposit
	  default:
		  return usage.KindTransfer
	  }
  }

// policyKind 将字符串映射到 policy.Kind。
  func policyKind(kind string) policy.Kind {
	  switch strings.ToLower(kind) {
	  case "withdraw":
		   return policy.KindWithdraw
	   case "deposit":
		  return policy.KindDeposit
	  default:
		  return policy.KindTransfer
	  }
   }

// ─── IssueCheckLease ──────────────────────────────────────────────────────
// IssueRequest 签发限额预检租约请求。
type IssueRequest struct {
	// UserID 用户 ID（必填）。
	UserID string
	// Asset 资产代码（必填）。
	Asset string
	// Kind 操作类型：withdraw / transfer / deposit（必填）。
	Kind string
	// Amount 本次占用请求金额（十进制字符串，必填）。
	Amount string
	// Dest 提现目标地址（withdraw 必填；其他可为空）。
	Dest string
	// Allowlisted 用户是否在白名单。
	Allowlisted bool
	// IdemKey 幂等键（防重复签发；推荐 = SHA256(user+asset+kind+amount+request_id)）。
	IdemKey string
}

// IssueResult 签发结果。

type IssueResult struct {
	// Lease 签发的预检租约（Check 放行时非空）。
	Lease *LimitCheckLease
	// Snap 签发时占用快照（含 PolicyVer/TodaySum/Velocity）。
	Snap UsageSnapshot
	// Allowed Check 是否放行（false 时 Lease 为 nil）。
	Allowed bool
	// RejectReason 拒绝原因（Allowed=false 时非空）。
	RejectReason string
}

// IssueCheckLease 执行 limitgate.Check 并签发 LimitCheckLease。
// 流程：
// 1. 调用 limitgate.Check（合成 policy+usage，非原子读）。
// 2. Check 放行后，将 PolicyVer、TodaySum、Velocity 快照写入 LimitCheckLease（state=checked）。
// 3. 返回 limit_check_lease_id 供占用提交携带。
// 幂等：IdemKey 已存在时返回原租约（不新建）。
// 不变量：
// - Check 拒绝（Allowed=false）时不签发租约。
// - 签发后的租约持有 PolicyVer + TodaySumAtIssue + VelocityAtIssue，供 CommitRecheck 复验。
  func (e *Engine) IssueCheckLease(ctx context.Context, req IssueRequest) (*IssueResult, error) {
	  if strings.TrimSpace(req.UserID) == "" {
		   return nil, fmt.Errorf("limitleasegate: user_id required")
	  }
	   if strings.TrimSpace(req.Asset) == "" {
		  return nil, fmt.Errorf("limitleasegate: asset required")
	  }
	   if strings.TrimSpace(req.Amount) == "" {
		  return nil, fmt.Errorf("limitleasegate: amount required")
	   }
	   if strings.TrimSpace(req.Kind) == "" {
		  return nil, fmt.Errorf("limitleasegate: kind required")
	   }
	  // 幂等键命中，返回已有租约。
	   if req.IdemKey != "" {
		  if existing, err := e.store.GetLeaseByIdem(ctx, req.IdemKey); err == nil {
			   return &IssueResult{
				   Lease:   existing,
				  Allowed: true,
			   }, nil
		  } else if !errors.Is(err, ErrNotFound) {
			  return nil, err
		  }
	   }
	   // 调用 limitgate.Check（非原子读 policy+usage）。
	  checkOut, err := limitgate.Check(e.reg, e.use, limitgate.Req{
		  Kind:   policyKind(req.Kind),
		   UserID: req.UserID,
		   Asset:  req.Asset,
		   Amount: req.Amount,
		   Dest:        req.Dest,
		  Allowlisted: req.Allowlisted,
	   })
	   if err != nil {
		   return &IssueResult{
			  Allowed:      false,
			   RejectReason: err.Error(),
		  }, err
	  }
	   if !checkOut.Allow {
		  return &IssueResult{
			  Allowed:      false,
			  RejectReason: checkOut.Reason,
		   }, ErrCheckDenied
	  }
	  // 抓取当前策略文档用于固化 DailyCap、MaxSingle 等快照参数。
	  doc, err := e.reg.Current()
	   if err != nil {
		  return nil, fmt.Errorf("limitleasegate: read policy doc: %w", err)
	  }
	   uk := usageKind(req.Kind)
	  now := e.now()
	   // 构建 UsageSnapshot（固化签发瞬时状态）。
	  snap := UsageSnapshot{
		   UserID: req.UserID,
		  Asset:  req.Asset,
		   Kind:   req.Kind,
		   PolicyVer: checkOut.PolicyVer,
		  TodaySum: checkOut.Today,
		  Velocity:    checkOut.Velocity,
		   MaxSingle:   doc.MaxSingle,
		   MaxVelocity: doc.MaxVelocity,
		   SnappedAt: now,
	   }
	  dailyCap := doc.DailyWithdraw
	   if uk == usage.KindDeposit {
		   dailyCap = doc.DailyDeposit
	   }
	   snap.DailyCap = dailyCap
	  // 签发 LimitCheckLease。
	   lease := &LimitCheckLease{
		   ID: uuid.NewString(),
		  UserID: req.UserID,
		  Asset: req.Asset,
		   Kind:   req.Kind,
		   Amount: req.Amount,
		   Dest:   req.Dest,
		  PolicyVer:       checkOut.PolicyVer,
		   TodaySumAtIssue: checkOut.Today,
		   VelocityAtIssue:  checkOut.Velocity,
		   MaxSingleAtIssue: doc.MaxSingle,
		   DailyCapAtIssue:  dailyCap,
		   MaxVelocityAtIssue: doc.MaxVelocity,
		   Allowlisted:        req.Allowlisted,
		  State:    LeaseStateChecked,
		   IdemKey:  req.IdemKey,
		   IssuedAt: now,
		  ExpiresAt: now.Add(e.cfg.LeaseTTL),
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if err := e.store.InsertLease(ctx, lease); err != nil {
		  if errors.Is(err, ErrDuplicate) && req.IdemKey != "" {
			   existing,
				   getErr := e.store.GetLeaseByIdem(ctx, req.IdemKey)
			   if getErr == nil {
				  return &IssueResult{Lease: existing, Snap: snap, Allowed: true}, nil
			   }
		  }
		   return nil, fmt.Errorf("limitleasegate: insert lease: %w", err)
	   }
	  return &IssueResult{Lease: lease, Snap: snap, Allowed: true}, nil
  }

// ─── BindLease ────────────────────────────────────────────────────────────
// BindRequest 绑定占用提交请求。

type BindRequest struct {
	// LeaseID limit_check_lease_id（必填）。
	LeaseID string
	// UserID 用户 ID（校验主体一致性）。
	UserID string
	// Asset 资产代码（校验一致性）。
	Asset string
	// Kind 操作类型（校验一致性）。
	Kind string
	// CommitRef 占用提交单据 ID（出金/划转/结算批次 ID，必填）。
	CommitRef string
}

// BindLease 将占用提交绑定到 LimitCheckLease（checked → bound）。
// 校验：
// - 租约必须存在且为 checked 状态。
// - 租约未过 TTL。
// - UserID/Asset/Kind 与租约一致（ScopeMismatch）。
// - 无 open LimitDriftProof 挡写（ProofBlocked）。

  func (e *Engine) BindLease(ctx context.Context, req BindRequest) (*LimitCheckLease, error) {
	   if strings.TrimSpace(req.LeaseID) == "" {
		   return nil, ErrNoLease
	   }
	   lease, err := e.store.GetLeaseByID(ctx, req.LeaseID)
	  if err != nil {
		  return nil, err
	   }
	   now := e.now()
	  // 检查作用域一致性。
	  if lease.UserID != req.UserID || lease.Asset != req.Asset || lease.Kind != req.Kind {
		  return nil, ErrScopeMismatch
	   }
	   // 检查终态。
	  switch lease.State {
	   case LeaseStateConsumed:
		   return nil, ErrLeaseConsumed
	   case LeaseStateExpired:
		   return nil, ErrLeaseExpired
	  case LeaseStateRevoked:
		  return nil, ErrLeaseRevoked
	  }
	   // TTL 检查（即使状态未更新，实时判断）。
	  if lease.Expired(now) {
		  return nil, ErrLeaseExpired
	  }
	   // 检查是否已有 open 漂移证明挡写。
	  if _, proofErr := e.store.GetOpenProofByLease(ctx, req.LeaseID); proofErr == nil {
		   return nil, ErrProofBlocked
	   }
	   // 状态校验：只有 checked 才能绑定。
	  if lease.State != LeaseStateChecked {
		   return nil, fmt.Errorf("limitleasegate: lease state is %q, expected checked", lease.State)
	   }
	  t := now
	  lease.State = LeaseStateBound
	   lease.CommitRef = req.CommitRef
	  lease.BoundAt = &t
	   lease.UpdatedAt = now
	   if err := e.store.UpdateLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("limitleasegate: bind lease: %w", err)
	  }
	  return lease, nil
   }

// ─── CommitRecheck ────────────────────────────────────────────────────────
// CommitRecheckRequest 占用提交最终复验请求。

type CommitRecheckRequest struct {
	// LeaseID limit_check_lease_id（必填）。
	LeaseID string
	// UserID 用户 ID（校验主体一致性）。
	UserID string
	// Asset 资产代码（校验一致性）。
	Asset string
	// Kind 操作类型（校验一致性）。
	Kind string
	// CommitRef 占用提交单据 ID（bound 状态时校验一致性）。
	CommitRef string
	// Allowlisted 当前白名单状态。
	Allowlisted bool
}

// CommitRecheckResult 复验结果。
type CommitRecheckResult struct {
	// Lease 更新后的租约（非空）。
	Lease *LimitCheckLease
	// Allowed 复验是否通过（false 时租约置 revoked + 落 LimitDriftProof）。
	Allowed bool
	// RejectReason 拒绝原因。
	RejectReason string
	// DriftProof 漂移证明（Allowed=false 时非空）。
	DriftProof *LimitDriftProof
}

// CommitRecheck 占用提交最终核验：重新 Eval policy+usage，漂移则置 revoked 并落 LimitDriftProof。
// 漂移判断顺序：
// 1. PolicyVer 变化 → policy_flip（先级最高）。
// 2. TodaySum+Amount > DailyCap → usage_breach。
// 3. Velocity >= MaxVelocity → velocity_breach。
// 通过所有检查 → 租约置 consumed 终态。
// 不变量：
// - 无租约（LeaseID 为空）且 AllowCommitWithoutLease=false 时，强制 fail-closed。
// - 所有漂移路径均落 LimitDriftProof；漂移证明 ID 写入 lease.DriftProofID。

   func (e *Engine) CommitRecheck(ctx context.Context, req CommitRecheckRequest) (*CommitRecheckResult, error) {
	   if strings.TrimSpace(req.LeaseID) == "" {
		  if !e.cfg.AllowCommitWithoutLease {
			   return nil, ErrNoLease
		   }
		  return &CommitRecheckResult{Allowed: false, RejectReason: "no_lease"}, ErrNoLease
	  }
	   lease,
		   err := e.store.GetLeaseByID(ctx, req.LeaseID)
	  if err != nil {
		  return nil, err
	   }
	   now := e.now()
	   // 作用域一致性校验。
	  if lease.UserID != req.UserID || lease.Asset != req.Asset || lease.Kind != req.Kind {
		   return nil, ErrScopeMismatch
	   }
	   // 终态检查。
	  switch lease.State {
	  case LeaseStateConsumed:
		   return nil, ErrLeaseConsumed
	  case LeaseStateExpired:
		   return nil, ErrLeaseExpired
	  case LeaseStateRevoked:
		  return nil, ErrLeaseRevoked
	  }
	   // TTL 实时检查。
	   if lease.Expired(now) {
		   return nil, ErrLeaseExpired
	   }
	   // bound 状态校验 CommitRef 一致性。
	  if lease.State == LeaseStateBound && req.CommitRef != "" && lease.CommitRef != req.CommitRef {
		  return nil, ErrCommitRefMismatch
	   }
	  // ── 重新读取当前 PolicyVer ──────────────────────────────────────────
	   doc, err := e.reg.Current()
	  if err != nil {
		   return nil, fmt.Errorf("limitleasegate: read policy doc for recheck: %w", err)
	  }
	   // 1. PolicyVer 漂移检测（最高先级）。
	  if doc.Version != lease.PolicyVer {
		   proof, proofErr := e.openDriftProof(ctx, lease, DriftTypePolicyFlip,
			  lease.PolicyVer, doc.Version, lease.TodaySumAtIssue, "", now)
		   if proofErr != nil && !errors.Is(proofErr, ErrDuplicate) {
			  return nil, fmt.Errorf("limitleasegate: open policy drift proof: %w", proofErr)
		   }
		  lease,
			   err = e.revokeLeaseWithProof(ctx, lease, RevokeReasonPolicyFlip, proof, now)
		   if err != nil {
			  return nil, fmt.Errorf("limitleasegate: revoke lease (policy_flip): %w", err)
		   }
		  return &CommitRecheckResult{
			  Lease:   lease,
			  Allowed: false,
			  RejectReason: "policy_flip",
			  DriftProof:   proof,
		   }, ErrPolicyDrift
	   }
	   // 2. TodaySum 越阈检测。
	   uk := usageKind(req.Kind)
	  currentSum := e.use.TodaySum(uk, req.UserID, req.Asset)
	  dailyCap := doc.DailyWithdraw
	  if uk == usage.KindDeposit {
		   dailyCap = doc.DailyDeposit
	  }
	   if dailyCap != "" {
		   capAmt,
			  parseErr := parseAmount(dailyCap)
		  if parseErr == nil {
			   sumAmt, parseErr2 := parseAmount(currentSum.String())
			  reqAmt, parseErr3 := parseAmount(lease.Amount)
			   if parseErr2 == nil && parseErr3 == nil {
				   projected := sumAmt + reqAmt
				   if projected > capAmt {
					  proof,
						   proofErr := e.openDriftProof(ctx, lease, DriftTypeUsageBreach,
						   lease.PolicyVer, doc.Version,
						   lease.TodaySumAtIssue, currentSum.String(), now)
					   if proofErr != nil && !errors.Is(proofErr, ErrDuplicate) {
						   return nil, fmt.Errorf("limitleasegate: open usage breach proof: %w", proofErr)
					   }
					  lease, err = e.revokeLeaseWithProof(ctx, lease, RevokeReasonUsageBreach, proof, now)
					  if err != nil {
						   return nil, fmt.Errorf("limitleasegate: revoke lease (usage_breach): %w", err)
					  }
					   return &CommitRecheckResult{
						   Lease: lease,
						   Allowed:      false,
						  RejectReason: "usage_breach",
						  DriftProof:   proof,
					   }, ErrUsageDrift
				  }
			   }
		   }
	  }
	  // 3. Velocity 越阈检测。
	   win := doc.VelocityWin
	   if win <= 0 {
		  win = time.Minute
	   }
	   curVel := e.use.Velocity(uk, req.UserID, req.Asset, win)
	   if doc.MaxVelocity > 0 && curVel >= doc.MaxVelocity {
		  proof, proofErr := e.openDriftProof(ctx, lease, DriftTypeVelocityBreach,
			   lease.PolicyVer, doc.Version, lease.TodaySumAtIssue, "", now)
		  if proofErr != nil && !errors.Is(proofErr, ErrDuplicate) {
			   return nil, fmt.Errorf("limitleasegate: open velocity breach proof: %w", proofErr)
		   }
		  lease, err = e.revokeLeaseWithProof(ctx, lease, RevokeReasonVelocityBreach, proof, now)
		  if err != nil {
			  return nil, fmt.Errorf("limitleasegate: revoke lease (velocity_breach): %w", err)
		  }
		   return &CommitRecheckResult{
			   Lease: lease,
			  Allowed:      false,
			   RejectReason: "velocity_breach",
			  DriftProof: proof,
		   }, ErrVelocityDrift
	  }
	  // 所有检查通过 → 消费租约。
	   t := now
	  lease.State = LeaseStateConsumed
	  lease.ConsumedAt = &t
	   lease.UpdatedAt = now
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		  return nil, fmt.Errorf("limitleasegate: consume lease: %w", err)
	   }
	  return &CommitRecheckResult{Lease: lease,
		   Allowed: true}, nil
   }

// ─── 漂移证明内部辅助 ─────────────────────────────────────────────────────
// openDriftProof 开启漂移证明（内部辅助）。

  func (e *Engine) openDriftProof(
	   ctx context.Context,
	   lease *LimitCheckLease,
	   dt DriftType,
	   oldVer, newVer int,
	  oldSum, newSum string,
	   now time.Time,
  ) (*LimitDriftProof, error) {
	  proofHash := computeProofHash(lease.ID, dt, oldVer, newVer, oldSum, newSum)
	   idem := fmt.Sprintf("dp|%s|%s", lease.ID, dt)
	  // 幂等：已有同一租约+类型的证明则返回。
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		  return existing, nil
	   }
	  if existing, err := e.store.GetOpenProofByLease(ctx, lease.ID); err == nil {
		  return existing, nil
	  }
	   proof := &LimitDriftProof{
		   ID:      uuid.NewString(),
		  LeaseID: lease.ID,
		  UserID: lease.UserID,
		  Asset: lease.Asset,
		   Kind:         lease.Kind,
		  DriftType:    dt,
		  OldPolicyVer: oldVer,
		  NewPolicyVer: newVer,
		   OldTodaySum: oldSum,
		  NewTodaySum: newSum,
		  DailyCap:    lease.DailyCapAtIssue,
		   OldVelocity: lease.VelocityAtIssue,
		   MaxVelocity: lease.MaxVelocityAtIssue,
		  State:       ProofStateOpen,
		   ProofHash: proofHash,
		  IdemKey:   idem,
		  OpenedAt:  now,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.InsertProof(ctx, proof); err != nil {
		  if errors.Is(err, ErrDuplicate) {
			   if existing, getErr := e.store.GetProofByIdem(ctx, idem); getErr == nil {
				  return existing, nil
			  }
		   }
		   return nil, err
	   }
	  return proof, nil
  }

// revokeLeaseWithProof 置租约 revoked 并关联漂移证明（内部辅助）。
  func (e *Engine) revokeLeaseWithProof(
	   ctx context.Context,
	  lease *LimitCheckLease,
	  reason RevokeReason,
	  proof *LimitDriftProof,
	   now time.Time,
   ) (*LimitCheckLease, error) {
	  if lease.State.Closed() {
		  return lease, nil
	  }
	   t := now
	  lease.State = LeaseStateRevoked
	   lease.RevokedAt = &t
	   lease.RevokeReason = string(reason)
	   if proof != nil {
		  lease.DriftProofID = proof.ID
	  }
	   lease.UpdatedAt = now
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		  return lease, err
	  }
	  return lease, nil
   }

// ─── RevokeByPolicyVersion ────────────────────────────────────────────────
// RevokeByPolicyVersionRequest 策略代次翻转批量吊销请求。
type RevokeByPolicyVersionRequest struct {
	// NewPolicyVer 当前最新策略代次（低于此代次的活跃租约将被吊销）。
	NewPolicyVer int
	// Limit 每批处理数量（默认使用 cfg.RevokeBatchSize）。
	Limit int
}

// RevokeByPolicyVersionResult 批量吊销结果。
type RevokeByPolicyVersionResult struct {
	// Revoked 成功吊销的租约数量。
	Revoked int
	// Failed 处理失败的租约数量。
	Failed int
	// Errors 失败详情（租约 ID → 错误）。
	Errors map[string]string
}

// RevokeByPolicyVersion 批量吊销 PolicyVer 低于当前代次的活跃租约。
// 触发时机：policy.Registry.Publish 新代次后，由 PolicyVersionRevokeWorker 调用。
// 不变量：仅吊销 PolicyVer < NewPolicyVer 的租约；同代次租约不受影响。

  func (e *Engine) RevokeByPolicyVersion(ctx context.Context, req RevokeByPolicyVersionRequest) (*RevokeByPolicyVersionResult, error) {
	  if req.NewPolicyVer <= 0 {
		   return nil, fmt.Errorf("limitleasegate: new_policy_ver required")
	   }
	  limit := req.Limit
	  if limit <= 0 {
		  limit = e.cfg.RevokeBatchSize
	  }
	  leases, err := e.store.ListActiveLeasesByPolicyVer(ctx, req.NewPolicyVer, limit)
	  if err != nil {
		   return nil, fmt.Errorf("limitleasegate: list leases for revoke: %w", err)
	   }
	  result := &RevokeByPolicyVersionResult{
		   Errors: map[string]string{},
	  }
	   now := e.now()
	  for _, lease := range leases {
		   proof, proofErr := e.openDriftProof(ctx, lease, DriftTypePolicyFlip,
			   lease.PolicyVer, req.NewPolicyVer, lease.TodaySumAtIssue, "", now)
		  if proofErr != nil && !errors.Is(proofErr, ErrDuplicate) {
			  result.Failed++
			  result.Errors[lease.ID] = fmt.Sprintf("open proof: %v", proofErr)
			   continue
		  }
		   if _, revokeErr := e.revokeLeaseWithProof(ctx, lease, RevokeReasonPolicyFlip, proof, now); revokeErr != nil {
			  result.Failed++
			   result.Errors[lease.ID] = fmt.Sprintf("revoke: %v", revokeErr)
			  continue
		   }
		  result.Revoked++
	   }
	   return result, nil
   }

// ─── ExpireLease ──────────────────────────────────────────────────────────
// ExpireLease 将超 TTL 的活跃租约置 expired 终态。
// 由 LeaseExpiryWorker 在后台定期调用；Engine 主路径不直接调用。
  func (e *Engine) ExpireLease(ctx context.Context, leaseID string) error {
	  lease,
		   err := e.store.GetLeaseByID(ctx, leaseID)
	  if err != nil {
		  return err
	   }
	   if lease.State.Closed() {
		  return nil
	  }
	   now := e.now()
	   if !lease.Expired(now) {
		  return nil
	   }
	   lease.State = LeaseStateExpired
	   lease.UpdatedAt = now
	  return e.store.UpdateLease(ctx, lease)
   }

// ─── 证明生命周期 ─────────────────────────────────────────────────────────
// ProveProof 将 open 状态证明固化为 proven（ProofProveWorker 驱动）。
   func (e *Engine) ProveProof(ctx context.Context, proofID string) error {
	  proof, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		   return err
	  }
	   if proof.State != ProofStateOpen {
		  return fmt.Errorf("limitleasegate: proof %q state is %q, expected open", proofID, proof.State)
	  }
	  now := e.now()
	   t := now
	  proof.State = ProofStateProven
	   proof.ProvenAt = &t
	  proof.UpdatedAt = now
	   return e.store.UpdateProof(ctx,
		   proof)
  }

// ConfirmProof 管理员确认漂移证明（proven → closed）。

  func (e *Engine) ConfirmProof(ctx context.Context, proofID, closedBy, notes string) error {
	  proof, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		  return err
	  }
	   if proof.State != ProofStateProven {
		   return fmt.Errorf("limitleasegate: proof %q state is %q, expected proven", proofID, proof.State)
	   }
	   now := e.now()
	  t := now
	   proof.State = ProofStateClosed
	  proof.ClosedAt = &t
	   proof.ClosedBy = closedBy
	   proof.Notes = notes
	   proof.UpdatedAt = now
	  return e.store.UpdateProof(ctx, proof)
  }

// DismissProof 驳回漂移证明（open|proven → closed，无需 proven 中间态）。
  func (e *Engine) DismissProof(ctx context.Context, proofID, closedBy, notes string) error {
	  proof, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		   return err
	   }
	   if proof.State.Closed() {
		  return fmt.Errorf("limitleasegate: proof %q already closed", proofID)
	  }
	  now := e.now()
	   t := now
	  proof.State = ProofStateClosed
	   proof.ClosedAt = &t
	  proof.ClosedBy = closedBy
	   proof.Notes = notes
	   proof.UpdatedAt = now
	   return e.store.UpdateProof(ctx, proof)
   }

// ExportProof 导出证明审计摘要（JSON 友好结构）。
  func (e *Engine) ExportProof(ctx context.Context, proofID string) (*ProofExport, error) {
	   proof, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		  return nil, err
	   }
	   lease,
		   _ := e.store.GetLeaseByID(ctx, proof.LeaseID)
	  return &ProofExport{
		   Proof: proof,
		   Lease: lease,
	   }, nil
  }

// ProofExport 漂移证明导出结构（含关联租约）。
type ProofExport struct {
	Proof *LimitDriftProof
	Lease *LimitCheckLease
}

// ─── 辅助：浮点金额比较（内部使用，不依赖 money.Amount 加法） ──────────────
// parseAmount 将十进制字符串金额解析为 float64（仅用于大小比较，不用于财务精算）。
// 财务精算由 policy.Eval 的 money.Amount 负责；此处仅供 CommitRecheck 快速越阈判断。
  func parseAmount(s string) (float64, error) {
	  if s == "" {
		  return 0, nil
	   }
	   var f float64
	  _, err := fmt.Sscanf(strings.TrimSpace(s), "%f", &f)
	   return f, err
  }
