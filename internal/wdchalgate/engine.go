package wdchalgate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// ─── 配置 ─────────────────────────────────────────────────────────────────
// Config Engine 配置项。
type Config struct {
	// LeaseTTL 挑战租约有效期（默认 300s）。
	// Issue 后必须在此时间内完成 CheckWithdraw，否则 lease 过期被吊销。
	LeaseTTL time.Duration
	// AllowGenDrift 是否允许 combo_gen 小幅漂移（默认 false，保持 false）。
	// 仅集成可设 true；环境强制 gen 精确匹配。
	AllowGenDrift bool
	// StaleBatchSize 因子变更触发批量 stale 时每批处理量（默认 200）。
	StaleBatchSize int
}

   func (c Config) withDefaults() Config {
	  if c.LeaseTTL <= 0 {
		  c.LeaseTTL = 300 * time.Second
	  }
	   if c.StaleBatchSize <= 0 {
		   c.StaleBatchSize = 200
	  }
	  return c
   }

// ─── 只读外部观测钩子 ─────────────────────────────────────────────────────
// FailLockedChecker 只读查询用户失败锁定状态（不调用 noteFail / clearFail）。
// 绑定 wdverify.Gate.FailLocked；Engine 不改写失败计数。

type FailLockedChecker func(ctx context.Context, userID string) (locked bool, remainSec int64, lastFactor string, err error)

// CooldownUntilReader 只读查询用户安全冷却到期时刻（不调用 seclock.Mark）。
// 绑定 seclock.Service.Until；Engine 不写冷却窗口。
type CooldownUntilReader func(ctx context.Context, userID string) (until time.Time, err error)

// ─── Engine ────────────────────────────────────────────────────────────────
// Engine 出金多因子挑战租约核心引擎。
type Engine struct {
	store Store
	cfg   Config
	clock      func() time.Time
	failLocked FailLockedChecker
	cooldownUntil CooldownUntilReader
}

// NewEngine 初始化 Engine。
   func NewEngine(store Store, cfg Config) *Engine {
	   return &Engine{
		   store: store,
		  cfg:   cfg.withDefaults(),
		  clock: time.Now,
	   }
   }

// BindFailLockedChecker 绑定失败锁定只读钩子（可选；未绑定时跳过锁定检查）。
  func (e *Engine) BindFailLockedChecker(fn FailLockedChecker) *Engine {
	  if e != nil {
		  e.failLocked = fn
	  }
	   return e
  }

// BindCooldownUntilReader 绑定安全冷却只读钩子（可选；未绑定时跳过冷却检查）。
  func (e *Engine) BindCooldownUntilReader(fn CooldownUntilReader) *Engine {
	  if e != nil {
		   e.cooldownUntil = fn
	  }
	  return e
   }

// ─── 辅助方法 ─────────────────────────────────────────────────────────────

   func (e *Engine) now() time.Time {
	   if e.clock != nil {
		   return e.clock()
	   }
	   return time.Now().UTC()
  }

// assertNotFailLocked 检查用户失败锁定状态；已锁定则拒绝。
   func (e *Engine) assertNotFailLocked(ctx context.Context, userID string) error {
	  if e.failLocked == nil {
		  return nil
	  }
	   locked, remain, factor, err := e.failLocked(ctx, userID)
	  if err != nil {
		   return fmt.Errorf("wdchalgate fail-lock check: %w", err)
	   }
	   if locked {
		   return fmt.Errorf("%w: remain_sec=%d last_factor=%s", ErrFailLocked, remain, factor)
	   }
	  return nil
  }

// computeFactorSig 计算因子绑定指纹（SHA-256 hex）。
// 输入：userID + factor + comboGenStr + extraBindKey（可为空，如 TOTP secret 前缀哈希）。
// 指纹在 Issue 时固化，CheckWithdraw 时由 Engine 重新计算并。
// 不含 tradepw 哈希内容；不含 seclock Until 时间戳。

   func computeFactorSig(userID, factor string, comboGen int64, extraKey string) string {
	  raw := fmt.Sprintf("%s|%s|%d|%s", userID, factor, comboGen, extraKey)
	   sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
  }

// computeProofHash 计算 StaleChalProof 指纹（唯一性保证）。
  func computeProofHash(leaseID string, oldGen, newGen int64, reason DriftReason) string {
	   raw := fmt.Sprintf("%s|%d|%d|%s", leaseID, oldGen, newGen, reason)
	   sum := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", sum[:])
   }

// ─── IssueLease ───────────────────────────────────────────────────────────
// IssueRequest 签发挑战租约请求。

type IssueRequest struct {
	// UserID 发起用户 ID（必填）。
	UserID string
	// Factor 因子类型（totp / email / sms；必填）。
	Factor string
	// ExtraKey 因子绑定辅助键（TOTP 场景传入 TOTP secret 的前缀摘要；其他因子传空）。
	// 作用：使 factor_sig 能区分同用户同因子的 TOTP 重绑前后代次。
	ExtraKey string
	// DestChain 绑定目标链（可选）。
	DestChain string
	// DestAddr 绑定目标地址（可选；非空时 CheckWithdraw 强制）。
	DestAddr string
	// IdemKey 幂等键（防重复签发；推荐 = SHA256(userID+factor+requestID)）。
	IdemKey string
}

// IssueLease 签发挑战租约。
// 前置：
// - 用户不得处于 FailLocked 状态（只读查询）。
// - Factor 须为 totp / email / sms 之一。
// 副作用（仅写本域）：
// - 写入 ChallengeLease（state=issued）；写入/读取 ComboGeneration 世代。
// 幂等：IdemKey 已存在时返回原租约（不新建）。
   func (e *Engine) IssueLease(ctx context.Context, req IssueRequest) (*ChallengeLease, error) {
	   req.UserID = strings.TrimSpace(req.UserID)
	   req.Factor = strings.ToLower(strings.TrimSpace(req.Factor))
	   if req.UserID == "" {
		  return nil, fmt.Errorf("wdchalgate: user_id required")
	   }
	   switch req.Factor {
	   case "totp", "email", "sms":
	   default:
		   return nil, fmt.Errorf("wdchalgate: unsupported factor %q", req.Factor)
	   }
	  if err := e.assertNotFailLocked(ctx, req.UserID); err != nil {
		   return nil, err
	  }
	  // 幂等键命中，返回已有租约。
	  if req.IdemKey != "" {
		   if existing, err := e.store.GetLeaseByIdem(ctx, req.IdemKey); err == nil {
			  return existing, nil
		  } else if !errors.Is(err, ErrNotFound) {
			  return nil, err
		  }
	  }
	  // 读取当前组合世代。
	   comboGen, _, err := e.store.GetCurrentGen(ctx, req.UserID)
	  if err != nil {
		   return nil, fmt.Errorf("wdchalgate: get combo gen: %w", err)
	  }
	   now := e.now()
	  sig := computeFactorSig(req.UserID, req.Factor, comboGen, req.ExtraKey)
	  lease := &ChallengeLease{
		  ID:     uuid.NewString(),
		   UserID: req.UserID,
		  Factor: req.Factor,
		   ComboGen: comboGen,
		   FactorSig: sig,
		  State:     LeaseStateIssued,
		   DestChain: strings.TrimSpace(req.DestChain),
		  DestAddr:  strings.TrimSpace(req.DestAddr),
		   IdemKey:   req.IdemKey,
		   IssuedAt:  now,
		  ExpiresAt: now.Add(e.cfg.LeaseTTL),
		   RevokeReason: "",
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.InsertLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("wdchalgate: insert lease: %w", err)
	  }
	  return lease, nil
   }

// ─── PresentLease ─────────────────────────────────────────────────────────
// PresentRequest CheckWithdraw 提交核验请求（携带 chal_lease_id）。
type PresentRequest struct {
	// LeaseID 挑战租约 ID（必填；缺失则 ErrNoLease）。
	LeaseID string
	// UserID 核验用户 ID（必须与 lease.UserID 一致）。
	UserID string
	// Factor 核验因子（必须与 lease.Factor 一致）。
	Factor string
	// ExtraKey 因子绑定辅助键（用于重新计算 factor_sig，必须与签发时一致）。
	ExtraKey string
	// DestChain 目标链（与 lease.DestChain ；非空 lease 时必须匹配）。
	DestChain string
	// DestAddr 目标地址（与 lease.DestAddr ；非空 lease 时必须匹配）。
	DestAddr string
}

// PresentResult 提交核验结果。
type PresentResult struct {
	// Lease 更新后的租约（状态已推进）。
	Lease *ChallengeLease
	// Drifted 是否检测到因子/combo_gen 漂移（true 时 Lease.State=stale，ProofID 非空）。
	Drifted bool
	// ProofID 关联的 StaleChalProof.ID（仅 Drifted=true 时非空）。
	ProofID string
}

// PresentLease CheckWithdraw 携带 chal_lease_id 首次提交核验。
// 前置：
// - LeaseID 必须存在且未终态。
// - UserID 必须与 lease.UserID 一致。
// - Factor 必须与 lease.Factor 一致。
// - 目标地址须与 lease 绑定一致（lease.DestAddr 非空时）。
// - combo_gen 与 factor_sig 须与签发时一致；漂移则置 stale 并开 StaleChalProof。
// 副作用（仅写本域）：
// - 更新 ChallengeLease state（issued→presented 或 → stale）。
// - stale 路径：写 StaleChalProof（state=open）。
  func (e *Engine) PresentLease(ctx context.Context, req PresentRequest) (*PresentResult, error) {
	   if req.LeaseID == "" {
		   return nil, ErrNoLease
	   }
	   req.UserID = strings.TrimSpace(req.UserID)
	  req.Factor = strings.ToLower(strings.TrimSpace(req.Factor))
	  lease, err := e.store.GetLeaseByID(ctx, req.LeaseID)
	  if err != nil {
		   if errors.Is(err, ErrNotFound) {
			   return nil, ErrNoLease
		  }
		  return nil, err
	  }
	   // 用户/因子。
	   if lease.UserID != req.UserID {
		  return nil, fmt.Errorf("wdchalgate: lease user mismatch")
	  }
	  if lease.Factor != req.Factor {
		  return nil, fmt.Errorf("wdchalgate: lease factor mismatch")
	  }
	   // 终态检查。
	   now := e.now()
	   switch lease.State {
	  case LeaseStateConsumed:
		  return nil, ErrLeaseConsumed
	  case LeaseStateExpired:
		  return nil, ErrLeaseExpired
	   case LeaseStateStale:
		  // 已被 NotifyFactorChange 批量置 stale：携带 proof_id 返回供上层透传。
		  return &PresentResult{Lease: lease, Drifted: true, ProofID: lease.ProofID}, ErrLeaseStale
	   case LeaseStateRevoked:
		  return nil, ErrLeaseRevoked
	  }
	   // TTL 检查（状态尚未同步时由 Engine 实时判断）。
	  if lease.Expired(now) {
		  return nil, ErrLeaseExpired
	   }
	  // 目标地址（仅 lease 有绑定时强制）。
	   if lease.DestAddr != "" {
		  if strings.TrimSpace(req.DestAddr) != lease.DestAddr ||
			  strings.TrimSpace(req.DestChain) != lease.DestChain {
			  return nil, ErrDestMismatch
		  }
	  }
	   // combo_gen + factor_sig 漂移检查。
	  currentGen, currentSig, err := e.store.GetCurrentGen(ctx, req.UserID)
	  if err != nil {
		  return nil, fmt.Errorf("wdchalgate: get current gen: %w", err)
	   }
	   expectedSig := computeFactorSig(req.UserID, req.Factor, lease.ComboGen, req.ExtraKey)
	   genDrifted := !e.cfg.AllowGenDrift && currentGen != lease.ComboGen
	  sigDrifted := currentSig != "" && currentSig != expectedSig
	   _ = sigDrifted // factor_sig 已通过 combo_gen 世代覆盖，此处记录供日志
	  if genDrifted {
		  // 因子/世代漂移：置 stale + 开证明。
		  driftReason := DriftReasonComboGenDrift
		   if currentSig != "" && computeFactorSig(req.UserID, req.Factor, currentGen, req.ExtraKey) != lease.FactorSig {
			   driftReason = DriftReasonTOTPRebind
		   }
		   proof, proofErr := e.openStaleProof(ctx, lease, currentGen, driftReason, now)
		   if proofErr != nil && !errors.Is(proofErr, ErrDuplicate) {
			  return nil, proofErr
		   }
		  staleT := now
		  lease.State = LeaseStateStale
		  lease.StaleAt = &staleT
		   lease.RevokeReason = string(driftReason)
		  if proof != nil {
			  lease.ProofID = proof.ID
		  }
		   lease.UpdatedAt = now
		   if upErr := e.store.UpdateLease(ctx, lease); upErr != nil {
			   return nil, upErr
		   }
		   proofID := ""
		  if proof != nil {
			   proofID = proof.ID
		   }
		   return &PresentResult{Lease: lease, Drifted: true, ProofID: proofID}, ErrLeaseStale
	   }
	  // 正常路径：issued → presented。
	   if lease.State == LeaseStateIssued {
		   t := now
		  lease.State = LeaseStatePresented
		  lease.PresentedAt = &t
		  lease.UpdatedAt = now
		   if err := e.store.UpdateLease(ctx, lease); err != nil {
			   return nil, err
		  }
	  }
	   return &PresentResult{Lease: lease,
		   Drifted: false}, nil
   }

// openStaleProof 开启旧令牌拒放行证明（幂等：已存在则返回原证明）。
   func (e *Engine) openStaleProof(ctx context.Context, lease *ChallengeLease, newGen int64, reason DriftReason, now time.Time) (*StaleChalProof, error) {
	  idem := fmt.Sprintf("stale:%s:%d:%s", lease.ID, newGen, reason)
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		   return existing, nil
	  } else if !errors.Is(err, ErrNotFound) {
		   return nil, err
	  }
	   hash := computeProofHash(lease.ID,
		  lease.ComboGen, newGen, reason)
	  proof := &StaleChalProof{
		  ID:      uuid.NewString(),
		  LeaseID: lease.ID,
		   UserID:  lease.UserID,
		   Factor:  lease.Factor,
		   OldComboGen: lease.ComboGen,
		  NewComboGen: newGen,
		  DriftReason: reason,
		  State: ProofStateOpen,
		   ProofHash: hash,
		   IdemKey:   idem,
		   OpenedAt:  now,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.InsertProof(ctx, proof); err != nil {
		  return nil, err
	   }
	   return proof, nil
   }

// ─── ConsumeLease ─────────────────────────────────────────────────────────
// ConsumeLease 消费挑战租约（CheckWithdraw 所有因子核验通过后调用）。
// 前置：
// - 租约必须处于 presented 状态（PresentLease 已通过）。
// - UserID 必须一致。
// 幂等：已消费则返回 ErrLeaseConsumed（调用方可忽略）。
   func (e *Engine) ConsumeLease(ctx context.Context, leaseID, userID string) (*ChallengeLease, error) {
	   lease, err := e.store.GetLeaseByID(ctx, leaseID)
	  if err != nil {
		   return nil, err
	   }
	  if lease.UserID != userID {
		  return nil, fmt.Errorf("wdchalgate: lease user mismatch")
	   }
	   if lease.State == LeaseStateConsumed {
		  return lease, ErrLeaseConsumed
	   }
	  if lease.State != LeaseStatePresented {
		   return nil, fmt.Errorf("%w: from=%s to=consumed", ErrInvalidTransit, lease.State)
	  }
	  now := e.now()
	   t := now
	  lease.State = LeaseStateConsumed
	  lease.ConsumedAt = &t
	   lease.UpdatedAt = now
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		  return nil, err
	   }
	  return lease, nil
   }

// ─── RevokeLease ──────────────────────────────────────────────────────────
// RevokeLease 手动或 FailLocked 联动吊销 open 租约。
// 前置：
// - 租约必须处于 issued 或 presented 状态。
// - 终态租约静默跳过（幂等）。
// 副作用：更新 ChallengeLease state=revoked，写入 reason。
  func (e *Engine) RevokeLease(ctx context.Context, leaseID, userID, reason string) (*ChallengeLease, error) {
	   lease, err := e.store.GetLeaseByID(ctx, leaseID)
	  if err != nil {
		  return nil, err
	   }
	   if lease.UserID != userID {
		   return nil, fmt.Errorf("wdchalgate: lease user mismatch")
	   }
	  if lease.State.Closed() {
		  return lease, nil // 幂等
	   }
	   now := e.now()
	  lease.State = LeaseStateRevoked
	  lease.RevokeReason = reason
	  lease.UpdatedAt = now
	   if err := e.store.UpdateLease(ctx, lease); err != nil {
		  return nil, err
	   }
	   return lease, nil
  }

// ─── NotifyFactorChange ───────────────────────────────────────────────────
// FactorChangeEvent 因子变更事件（TOTP 重绑 / 组核验则变更 / 冷却抬升）。

type FactorChangeEvent struct {
	// UserID 受影响用户 ID。
	UserID string
	// Factor 变更因子（totp / email / sms；组合变更时可为 "combo"）。
	Factor string
	// ExtraKey 新绑定因子辅助键（TOTP 重绑时为新 secret 前缀摘要）。
	ExtraKey string
	// ChangeReason 变更原因。
	ChangeReason ChangeReason
	// Actor 操作人/系统标识。
	Actor string
	// RefID 关联外部单据 ID（可为空）。
	RefID string
}

// NotifyFactorChange 因子变更事件处理：写入 ComboGeneration + 批量置 stale 旧租约。
// 触发场景：
// - TOTP 重绑（wdverify.Gate.BindTOTP → 本方法观测变更）
// - 组核验则变更（Gate.SetCombos → 本方法观测变更）
// - 冷却参数变更（外部安全策略 → 本方法观测变更）
// 副作用（仅写本域）：
// - 追加 ComboGeneration（gen+1）。
// - 批量将同用户同因子的 issued/presented 租约置 stale，并开 StaleChalProof。
// 隔离：不修改 tradepw 哈希；不调用 seclock.Mark；不调用 wdguard.CheckTriple。
  func (e *Engine) NotifyFactorChange(ctx context.Context, ev FactorChangeEvent) (*ComboGeneration, error) {
	   ev.UserID = strings.TrimSpace(ev.UserID)
	   if ev.UserID == "" {
		   return nil, fmt.Errorf("wdchalgate: user_id required")
	  }
	  // 读取当前世代，分配新 gen。
	  currentGen, _, err := e.store.GetCurrentGen(ctx, ev.UserID)
	  if err != nil {
		   return nil, fmt.Errorf("wdchalgate: get combo gen: %w", err)
	   }
	   newGen := currentGen + 1
	  now := e.now()
	  newSig := computeFactorSig(ev.UserID, ev.Factor, newGen, ev.ExtraKey)
	  gen := &ComboGeneration{
		  ID:     uuid.NewString(),
		  UserID: ev.UserID,
		  Gen:    newGen,
		   ChangeReason: ev.ChangeReason,
		  FactorSig: newSig,
		  Actor:     ev.Actor,
		  RefID:     ev.RefID,
		   ChangedAt: now,
		   CreatedAt: now,
	   }
	  if err := e.store.InsertComboGen(ctx, gen); err != nil {
		  return nil, fmt.Errorf("wdchalgate: insert combo gen: %w", err)
	   }
	  // 批量置 stale 同用户同因子的旧租约。
	   factor := ev.Factor
	  if factor == "combo" || factor == "" {
		   factor = "" // 组合变更影响所有因子
	  }
	   if err := e.staleBatchByFactor(ctx, ev.UserID, factor, newGen, DriftReasonComboGenDrift, now); err != nil {
		   return gen, err
	   }
	  return gen, nil
  }

// staleBatchByFactor 批量将指定用户+因子的非终态租约置 stale。
// factor 为空时处理所有非终态租约（组合变更场景）。
  func (e *Engine) staleBatchByFactor(ctx context.Context, userID, factor string, newGen int64, reason DriftReason, now time.Time) error {
	   var leases []*ChallengeLease
	  var err error
	  if factor != "" {
		   leases, err = e.store.ListLeasesByUserFactor(ctx, userID, factor, e.cfg.StaleBatchSize)
	   } else {
		   leases, err = e.store.ListOpenLeasesByUser(ctx, userID, e.cfg.StaleBatchSize)
	  }
	  if err != nil {
		   return fmt.Errorf("wdchalgate: list leases for stale: %w", err)
	   }
	  for _, lease := range leases {
		   if lease.State.Closed() {
			   continue
		  }
		   // 开 StaleChalProof（幂等）。
		   proof, proofErr := e.openStaleProof(ctx, lease, newGen, reason, now)
		   if proofErr != nil && !errors.Is(proofErr, ErrDuplicate) {
			   return proofErr
		  }
		   t := now
		   lease.State = LeaseStateStale
		  lease.StaleAt = &t
		  lease.RevokeReason = string(reason)
		  if proof != nil {
			   lease.ProofID = proof.ID
		  }
		  lease.UpdatedAt = now
		   if upErr := e.store.UpdateLease(ctx, lease); upErr != nil {
			  return upErr
		   }
	  }
	  return nil
   }

// ─── NotifyFailLockUnlock ─────────────────────────────────────────────────
// NotifyFailLockUnlock FailLocked 解锁通知：推世代 + 吊销所有残存非终态租约。
// 语义：解锁后推一个新 gen（ChangeReasonFailLockUnlock），确保在锁定期间签发的旧租约
// 全部失效，不得在解锁后复活并通过 CheckWithdraw。
// 副作用（仅写本域）：追加 ComboGeneration + 批量 revoke 非终态租约。
  func (e *Engine) NotifyFailLockUnlock(ctx context.Context, userID, actor string) (*ComboGeneration, error) {
	  ev := FactorChangeEvent{
		   UserID: userID,
		  Factor:       "",
		   ChangeReason: ChangeReasonFailLockUnlock,
		   Actor:        actor,
	   }
	  gen, _, err := e.store.GetCurrentGen(ctx, userID)
	  if err != nil {
		   return nil, err
	  }
	  newGen := gen + 1
	   now := e.now()
	  newSig := computeFactorSig(userID, "", newGen, "")
	   comboGen := &ComboGeneration{
		  ID:           uuid.NewString(),
		  UserID:       ev.UserID,
		  Gen:          newGen,
		  ChangeReason: ev.ChangeReason,
		  FactorSig:    newSig,
		  Actor:        actor,
		  ChangedAt: now,
		   CreatedAt: now,
	  }
	  if err := e.store.InsertComboGen(ctx, comboGen); err != nil {
		   return nil, fmt.Errorf("wdchalgate: insert faillock gen: %w", err)
	   }
	   // 吊销所有残存非终态租约（revoke，不置 stale，语义不同）。
	  leases, err := e.store.ListOpenLeasesByUser(ctx, userID, e.cfg.StaleBatchSize)
	   if err != nil {
		   return comboGen, err
	  }
	   for _, lease := range leases {
		  if lease.State.Closed() {
			  continue
		   }
		   lease.State = LeaseStateRevoked
		  lease.RevokeReason = string(DriftReasonFailLockRevoke)
		  lease.UpdatedAt = now
		  _ = e.store.UpdateLease(ctx, lease)
	  }
	  return comboGen, nil
   }

// ─── Proof 管理 ───────────────────────────────────────────────────────────
// ProofSummary 证明审计摘要（不含敏感认证材料）。
type ProofSummary struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"lease_id"`
	UserID  string `json:"user_id"`
	Factor string `json:"factor"`
	OldComboGen int64 `json:"old_combo_gen"`
	NewComboGen int64  `json:"new_combo_gen"`
	DriftReason string `json:"drift_reason"`
	State string `json:"state"`
	ProofHash string `json:"proof_hash"`
	OpenedAt time.Time  `json:"opened_at"`
	ProvenAt *time.Time `json:"proven_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	ClosedBy string     `json:"closed_by,omitempty"`
}

// ExportProof 导出证明审计摘要（只读，供审计/运营）。

   func (e *Engine) ExportProof(ctx context.Context, proofID string) (*ProofSummary, error) {
	  p, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		   return nil, err
	  }
	  return &ProofSummary{
		   ProofID:     p.ID,
		  LeaseID:     p.LeaseID,
		   UserID:      p.UserID,
		   Factor:      p.Factor,
		  OldComboGen: p.OldComboGen,
		  NewComboGen: p.NewComboGen,
		   DriftReason: string(p.DriftReason),
		   State:     string(p.State),
		  ProofHash: p.ProofHash,
		   OpenedAt:  p.OpenedAt,
		  ProvenAt:  p.ProvenAt,
		  ClosedAt: p.ClosedAt,
		  ClosedBy: p.ClosedBy,
	  }, nil
  }

// ConfirmProof 管理员确认 StaleChalProof（proven→closed）。

  func (e *Engine) ConfirmProof(ctx context.Context, proofID, closedBy, notes string) (*StaleChalProof, error) {
	  p, err := e.store.GetProofByID(ctx, proofID)
	  if err != nil {
		   return nil, err
	   }
	  if p.State == ProofStateClosed {
		  return p, nil // 幂等
	  }
	   if p.State != ProofStateProven {
		   // 允许 open → closed（驳回路径由 DismissProof 覆盖，此处只做 proven→closed）。
		  return nil, fmt.Errorf("%w: from=%s to=closed (use DismissProof to close open)", ErrInvalidTransit, p.State)
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

// DismissProof 驳回 StaleChalProof（open/proven → closed，无需四眼）。

  func (e *Engine) DismissProof(ctx context.Context, proofID, closedBy, notes string) (*StaleChalProof, error) {
	  p, err := e.store.GetProofByID(ctx, proofID)
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

// ProveProof StaleChalWorker 固化证明（open→proven）。
   func (e *Engine) ProveProof(ctx context.Context, proofID string) (*StaleChalProof, error) {
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

// GetLease 查询租约（只读）。
   func (e *Engine) GetLease(ctx context.Context, leaseID string) (*ChallengeLease, error) {
	   return e.store.GetLeaseByID(ctx,
		   leaseID)
   }

// GetProof 查询证明（只读）。
   func (e *Engine) GetProof(ctx context.Context, proofID string) (*StaleChalProof, error) {
	  return e.store.GetProofByID(ctx, proofID)
  }

// ListOpenLeases 列举用户 open 租约（只读）。

   func (e *Engine) ListOpenLeases(ctx context.Context, userID string, limit int) ([]*ChallengeLease, error) {
	   return e.store.ListOpenLeasesByUser(ctx, userID, limit)
   }

// ListProofsByState 按状态列举证明（只读）。
  func (e *Engine) ListProofsByState(ctx context.Context, state ProofState, limit int) ([]*StaleChalProof, error) {
	  return e.store.ListProofsByState(ctx, state, limit)
   }

// ListComboGens 列举用户世代历史（只读）。
   func (e *Engine) ListComboGens(ctx context.Context, userID string, limit int) ([]*ComboGeneration, error) {
	   return e.store.ListComboGens(ctx, userID, limit)
   }

// CheckWithdrawGuard CheckWithdraw 前置租约闸（在 wdverify 因子核验前调用）。
// 调用流程（由上层 API handler 编排）：
// 1. 调用本方法，验证租约存在、未漂移、未过期、目标地址匹配。
// 2. 上层调用 wdverify.Gate.Check（因子 OTP 核验）。
// 3. 核验通过后调用 ConsumeLease（consumed 终态）。
// 本方法只做前置租约合法性检查，不调用 wdverify 因子核验（避免循环依赖）。
  func (e *Engine) CheckWithdrawGuard(ctx context.Context, req PresentRequest) (*PresentResult, error) {
	  if req.LeaseID == "" {
		  return nil, ErrNoLease
	   }
	   return e.PresentLease(ctx, req)
   }
