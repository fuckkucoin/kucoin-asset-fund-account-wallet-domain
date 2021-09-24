package polhopgate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/acctpol"
)

// ─── 配置 ─────────────────────────────────────────────────────────────────
// Config Engine 配置项。
type Config struct {
	// TokenTTL hop 令牌有效期（默认 30s）。
	TokenTTL time.Duration
	// LeaseTTL 策略发布互斥租约有效期（默认 60s）。
	LeaseTTL time.Duration
	// DualDivergeThreshold DualRead 分歧升级阈值（连续分歧次数，默认 3）。
	DualDivergeThreshold int
	// DualDivergeWindow 分歧统计时间窗口（默认 5 分钟）。
	DualDivergeWindow time.Duration
}

  func (c Config) withDefaults() Config {
	   if c.TokenTTL <= 0 {
		  c.TokenTTL = 30 * time.Second
	   }
	   if c.LeaseTTL <= 0 {
		   c.LeaseTTL = 60 * time.Second
	   }
	   if c.DualDivergeThreshold <= 0 {
		  c.DualDivergeThreshold = 3
	   }
	  if c.DualDivergeWindow <= 0 {
		  c.DualDivergeWindow = 5 * time.Minute
	  }
	   return c
   }

// ─── Engine ────────────────────────────────────────────────────────────────
// Engine 策略 hop 世代闸核心。

type Engine struct {
	store    Store
	polStore acctpol.HopStore // 只读：读取策略代次与 DualReadLog
	cfg   Config
	clock func() time.Time
}

// NewEngine 装配 Engine。
   func NewEngine(store Store, polStore acctpol.HopStore, cfg Config) *Engine {
	   return &Engine{
		  store: store,
		  polStore: polStore,
		  cfg:      cfg.withDefaults(),
		   clock:    time.Now,
	   }
   }

// WithClock 替换时钟（供注入）。

  func (e *Engine) WithClock(fn func() time.Time) *Engine {
	   e.clock = fn
	  return e
   }

   // ─── IssueToken ────────────────────────────────────────────────────────────
   // IssueRequest 令牌签发请求。

   type IssueRequest struct {
	  UserID  string
	  Asset   string
	  BizKind string
	  IdemKey string // 幂等键（防重复签发）
   }

   // IssueReply 令牌签发结果。
  type IssueReply struct {
	   TokenID string `json:"token_id"`
	  PolicyGen int64      `json:"policy_gen"`
	   PolicyID  string     `json:"policy_id"`
	   ExpiresAt time.Time  `json:"expires_at"`
	  State     TokenState `json:"state"`
   }

  // IssueToken Gate.Check 通过后签发 hop 令牌。
   // 流程：
  // 1. 检查该 BizKind 是否存在活跃 DualEscalate（挡写中则返回 ErrDualEscalateBlock）。
   // 2. 读取当前生效策略代次（个人钉版 > 全局 active）。
  // 3. 策略规则校验（AllowsHop / DenyBizKinds）。
  // 4. 幂等检查（IdemKey 已存在则返回已有令牌）。
  // 5. 插入新 HopToken（state=issued，绑定当前 Gen）。

  func (e *Engine) IssueToken(ctx context.Context, req IssueRequest) (*IssueReply, error) {
	   if req.UserID == "" || req.BizKind == "" {
		  return nil, fmt.Errorf("polhopgate: user_id and biz_kind required")
	   }
	   // 检查 DualEscalate 挡写
	  if esc, err := e.store.GetActiveEscalate(ctx, req.UserID, req.Asset, req.BizKind); err == nil && esc != nil {
		   return nil, fmt.Errorf("%w: escalate_id=%s biz=%s", ErrDualEscalateBlock, esc.ID, req.BizKind)
	   }
	   // 全局级 DualEscalate（userID=""）
	  if esc, err := e.store.GetActiveEscalate(ctx, "", req.Asset, req.BizKind); err == nil && esc != nil {
		  return nil, fmt.Errorf("%w: global escalate_id=%s biz=%s", ErrDualEscalateBlock, esc.ID, req.BizKind)
	  }
	  // 读取生效策略代次（个人钉版 > 全局 active）
	  ver, err := e.effectiveVersion(ctx, req.UserID, req.Asset)
	   if err != nil {
		  return nil, fmt.Errorf("polhopgate: issue token: get effective version: %w", err)
	  }
	   // 策略规则校验
	   bk := acctpol.HopBizKind(req.BizKind)
	   if !ver.AllowsHop(bk) {
		   return nil, fmt.Errorf("%w: user=%s biz=%s policy_gen=%d", ErrHopDenied, req.UserID, req.BizKind, ver.Gen)
	  }
	  // 幂等查重
	  if req.IdemKey != "" {
		   existing,
			  iErr := e.store.GetTokenByIdem(ctx, req.IdemKey)
		   if iErr == nil && existing != nil {
			  return e.tokenToReply(existing), nil
		  }
	  }
	   // 签发新令牌
	   now := e.clock()
	   tok := &HopToken{
		  ID:     uuid.NewString(),
		  UserID: req.UserID,
		   Asset:     req.Asset,
		  BizKind:   req.BizKind,
		   PolicyGen: ver.Gen,
		  PolicyID:  ver.ID,
		  State:     TokenStateIssued,
		   IdemKey: req.IdemKey,
		  IssuedAt:  now,
		   ExpiresAt: now.Add(e.cfg.TokenTTL),
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   tok.TokenHash = computeTokenHash(tok)
	  if err := e.store.InsertToken(ctx, tok); err != nil {
		   if errors.Is(err, ErrDuplicate) {
			  // 并发竞态：另一方已插入
			   if req.IdemKey != "" {
				   existing, _ := e.store.GetTokenByIdem(ctx, req.IdemKey)
				   if existing != nil {
					  return e.tokenToReply(existing), nil
				  }
			  }
		  }
		   return nil, fmt.Errorf("polhopgate: insert token: %w", err)
	   }
	   return e.tokenToReply(tok), nil
  }

// ─── BindHop ──────────────────────────────────────────────────────────────
// BindRequest hop 执行绑定请求（实际扣款/冻账前调用）。

type BindRequest struct {
	TokenID string // 待绑定令牌
	HopRef string // 业务单据 ID（划转 ID、出金 ID 等）
	UserID string // 发起用户（用于追加校验）
}

// BindReply hop 绑定结果。

type BindReply struct {
	TokenID   string `json:"token_id"`
	PolicyGen int64  `json:"policy_gen"`
	State TokenState `json:"state"`
}

// BindHop hop 执行复验：
// 1. 读取令牌；校验状态（非 issued → 拒绝）。
// 2. 校验未过期。
// 3. 读取当前生效策略世代；若 Gen 不符 → token→blocked + ErrGenMismatch。
// 4. 若 Gen 一致且未过期 → token→consumed，返回成功。
// 此方法是资金安全的关键路径：ErrGenMismatch 时调用方必须不改账。

   func (e *Engine) BindHop(ctx context.Context, req BindRequest) (*BindReply, error) {
	  tok, err := e.store.GetTokenByID(ctx, req.TokenID)
	   if err != nil {
		  return nil, fmt.Errorf("polhopgate: bind hop: get token: %w", err)
	  }
	   if req.UserID != "" && tok.UserID != req.UserID {
		   return nil, fmt.Errorf("polhopgate: bind hop: token user mismatch: want %s got %s", req.UserID, tok.UserID)
	  }
	  now := e.clock()
	  // 终态令牌直接返回对应错误
	  switch tok.State {
	   case TokenStateConsumed:
		  return nil, ErrTokenConsumed
	   case TokenStateExpired:
		   return nil, ErrTokenExpired
	  case TokenStateBlocked:
		  return nil, ErrTokenBlocked
	  }
	  // 过期校验
	  if now.After(tok.ExpiresAt) {
		   tok.State = TokenStateExpired
		   tok.UpdatedAt = now
		  _ = e.store.UpdateToken(ctx, tok)
		  return nil, ErrTokenExpired
	  }
	  // 策略世代复验（核心：防越权 hop）
	  ver, verErr := e.effectiveVersion(ctx, tok.UserID, tok.Asset)
	  if verErr != nil {
		  // 无法读取代次时保守挡写
		   reason := fmt.Sprintf("cannot read policy: %v", verErr)
		   return nil,
			  e.blockToken(ctx, tok, reason, now)
	   }
	   if ver.Gen != tok.PolicyGen {
		  // 世代不符：令牌挡写 + 开漂移案例
		   reason := fmt.Sprintf("policy gen mismatch: token=%d current=%d", tok.PolicyGen, ver.Gen)
		  blockErr := e.blockToken(ctx, tok, reason, now)
		  // 开漂移案例（竞态窗口越权）
		  _ = e.openDriftCase(ctx, tok, ver.Gen, DriftTriggerUnauthorized, reason, now)
		  return nil, blockErr
	  }
	  // 世代一致：消费令牌
	  tok.State = TokenStateConsumed
	  tok.ConsumedAt = &now
	   tok.HopRef = req.HopRef
	  tok.UpdatedAt = now
	   if err := e.store.UpdateToken(ctx, tok); err != nil {
		   return nil, fmt.Errorf("polhopgate: consume token: %w", err)
	   }
	  return &BindReply{TokenID: tok.ID, PolicyGen: tok.PolicyGen, State: tok.State}, nil
  }

// ─── CheckPublishWindow ────────────────────────────────────────────────────
// AcquireLeaseRequest 获取策略发布互斥租约。
type AcquireLeaseRequest struct {
	UserID string // 空=全局租约
	Asset  string
	Holder string // 操作人 ID
}

// AcquireLeaseReply 租约结果。

type AcquireLeaseReply struct {
	LeaseID string `json:"lease_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AcquireLease 获取策略发布互斥租约。
// 如果已有活跃租约（非持有者）返回 ErrLeaseLocked。
// Publish/Pin 操作前必须先 AcquireLease，完成后调用 ReleaseLease。

  func (e *Engine) AcquireLease(ctx context.Context, req AcquireLeaseRequest) (*AcquireLeaseReply, error) {
	  now := e.clock()
	   // 检查是否已有活跃租约
	   existing, err := e.store.GetActiveLease(ctx, req.UserID, req.Asset)
	   if err == nil && existing != nil {
		  if existing.Holder != req.Holder {
			  return nil, fmt.Errorf("%w: holder=%s expires=%s", ErrLeaseLocked, existing.Holder, existing.ExpiresAt.Format(time.RFC3339))
		   }
		   // 同持有人刷新过期时间（幂等）
		  existing.ExpiresAt = now.Add(e.cfg.LeaseTTL)
		  existing.UpdatedAt = now
		   if uErr := e.store.UpdateLease(ctx, existing); uErr != nil {
			   return nil, fmt.Errorf("polhopgate: refresh lease: %w", uErr)
		   }
		   return &AcquireLeaseReply{LeaseID: existing.ID,
			   ExpiresAt: existing.ExpiresAt}, nil
	   }
	  lease := &PolLease{
		   ID:     uuid.NewString(),
		   UserID: req.UserID,
		   Asset:  req.Asset,
		   Holder: req.Holder,
		  State:  LeaseStateAcquired,
		   AcquiredAt: now,
		  ExpiresAt: now.Add(e.cfg.LeaseTTL),
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if err := e.store.InsertLease(ctx,
		   lease); err != nil {
		  return nil, fmt.Errorf("polhopgate: insert lease: %w", err)
	   }
	  return &AcquireLeaseReply{LeaseID: lease.ID, ExpiresAt: lease.ExpiresAt}, nil
   }

// ReleaseLease 释放策略发布互斥租约（Publish/Pin 完成后调用）。
  func (e *Engine) ReleaseLease(ctx context.Context, leaseID string, completedGen int64) error {
	  lease, err := e.store.GetLeaseByID(ctx, leaseID)
	   if err != nil {
		   return fmt.Errorf("polhopgate: release lease: %w", err)
	   }
	  if lease.State != LeaseStateAcquired {
		   return nil // 幂等：已释放则跳过
	  }
	   now := e.clock()
	   lease.State = LeaseStateReleased
	   lease.PolicyGen = completedGen
	   lease.ReleasedAt = &now
	  lease.UpdatedAt = now
	   return e.store.UpdateLease(ctx, lease)
   }

// ─── RecordDualRead ────────────────────────────────────────────────────────
// DualReadRequest 双读观测请求。
type DualReadRequest struct {
	UserID string
	Asset string
	BizKind    string
	PrimaryGen int64
	ReplicaGen int64
	TraceID    string
}

// RecordDualRead 记录双读观测，分歧超阈时升级为 DualEscalate + PolDriftCase。
// 关键：分歧检测是在进程内判断 PrimaryGen != ReplicaGen；
// 不依赖 cfgcenter rollout_id 键空间（r7 隔离）。
  func (e *Engine) RecordDualRead(ctx context.Context, req DualReadRequest) error {
	   now := e.clock()
	  result := acctpol.DualReadMatch
	  delta := req.PrimaryGen - req.ReplicaGen
	  if delta != 0 {
		  result = acctpol.DualReadDiverge
	  }
	  log := &acctpol.HopDualReadLog{
		  ID: uuid.NewString(),
		  UserID:     req.UserID,
		   Asset:      req.Asset,
		   BizKind:    acctpol.HopBizKind(req.BizKind),
		   PrimaryGen: req.PrimaryGen,
		  ReplicaGen: req.ReplicaGen,
		   DivergeDelta: delta,
		   Result:       result,
		  ObservedAt: now,
		  TraceID:   req.TraceID,
		   CreatedAt: now,
	  }
	   if err := e.polStore.AppendDualReadLog(ctx, log); err != nil {
		   return fmt.Errorf("polhopgate: append dual read log: %w", err)
	  }
	  if result != acctpol.DualReadDiverge {
		   return nil
	   }
	  // 统计时间窗口内分歧次数
	   since := now.Add(-e.cfg.DualDivergeWindow)
	  count, err := e.polStore.CountDivergeLogs(ctx, acctpol.HopBizKind(req.BizKind), since)
	   if err != nil {
		   return fmt.Errorf("polhopgate: count diverge logs: %w", err)
	   }
	  if count < e.cfg.DualDivergeThreshold {
		   return nil // 未达阈值，仅记日志
	  }
	  // 检查是否已有活跃升级挡写
	  if _, err := e.store.GetActiveEscalate(ctx, req.UserID, req.Asset, req.BizKind); err == nil {
		  return nil // 已有升级，幂等
	   }
	   // 升级：创建 PolDriftCase + DualEscalate
	  idemKey := fmt.Sprintf("dualread:%s:%s:%s:%d", req.UserID, req.Asset, req.BizKind, req.PrimaryGen)
	  existingCase, caseErr := e.store.GetDriftCaseByIdem(ctx, idemKey)
	   if caseErr == nil && existingCase != nil {
		  return nil // 已有案例，幂等
	   }
	   driftCase := &PolDriftCase{
		  ID:     uuid.NewString(),
		   UserID: req.UserID,
		  Asset: req.Asset,
		  BizKind: req.BizKind,
		   Trigger: DriftTriggerDualRead,
		   TriggerGen:   req.PrimaryGen,
		   ReplicaGen:   req.ReplicaGen,
		   DivergeDelta: delta,
		   State:        DriftCaseOpen,
		  IdemKey:      idemKey,
		   BlockedAt: now,
		  ScanNote:  fmt.Sprintf("dual_read diverge count=%d threshold=%d delta=%d", count, e.cfg.DualDivergeThreshold, delta),
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.InsertDriftCase(ctx, driftCase); err != nil && !errors.Is(err, ErrDuplicate) {
		   return fmt.Errorf("polhopgate: insert drift case: %w", err)
	  }
	  escalate := &DualEscalate{
		  ID:      uuid.NewString(),
		   UserID:  req.UserID,
		   Asset:   req.Asset,
		  BizKind: req.BizKind,
		  ThresholdCount: count,
		   MaxDivergeDelta: absInt64(delta),
		   CaseID:          driftCase.ID,
		   State:           EscalateStateActive,
		  EscalatedAt:     now,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := e.store.InsertEscalate(ctx, escalate); err != nil && !errors.Is(err, ErrDuplicate) {
		   return fmt.Errorf("polhopgate: insert escalate: %w", err)
	   }
	  return nil
  }

// ─── ConfirmDriftCase ─────────────────────────────────────────────────────
// ConfirmDriftCaseRequest 确认漂移案例请求。
type ConfirmDriftCaseRequest struct {
	CaseID      string
	ConfirmedBy string
	CloseReason string
}

// ConfirmDriftCase 人工确认漂移案例（必须先有证明，才能关闭）。
// 关闭案例后，对应的 DualEscalate 挡写解除。

   func (e *Engine) ConfirmDriftCase(ctx context.Context, req ConfirmDriftCaseRequest) error {
	   dc, err := e.store.GetDriftCaseByID(ctx, req.CaseID)
	   if err != nil {
		  return fmt.Errorf("polhopgate: confirm drift case: %w", err)
	  }
	   // 必须有证明才能关闭（dismissed 路径由 DismissDriftCase 处理）
	  if dc.State == DriftCaseOpen {
		  return fmt.Errorf("%w: case %s must be proven before close", ErrProofMissing, req.CaseID)
	   }
	  if dc.State == DriftCaseClosed {
		   return nil // 幂等
	  }
	  if err := dc.Transition(DriftCaseClosed); err != nil {
		   return err
	   }
	   now := e.clock()
	   dc.State = DriftCaseClosed
	  dc.ConfirmedBy = req.ConfirmedBy
	  dc.ConfirmedAt = &now
	  dc.CloseReason = req.CloseReason
	  dc.UpdatedAt = now
	   if err := e.store.UpdateDriftCase(ctx,
		  dc); err != nil {
		  return fmt.Errorf("polhopgate: update drift case: %w", err)
	  }
	  // 解除 DualEscalate 挡写
	   if esc, escErr := e.store.GetActiveEscalate(ctx, dc.UserID, dc.Asset, dc.BizKind); escErr == nil && esc != nil {
		   esc.State = EscalateStateResolved
		  esc.ResolvedAt = &now
		   esc.UpdatedAt = now
		  _ = e.store.UpdateEscalate(ctx, esc)
	   }
	   return nil
  }

// DismissDriftCase 驳回漂移案例（无需证明，直接关闭）。
   func (e *Engine) DismissDriftCase(ctx context.Context, caseID, dismissedBy, reason string) error {
	  dc, err := e.store.GetDriftCaseByID(ctx, caseID)
	  if err != nil {
		  return fmt.Errorf("polhopgate: dismiss drift case: %w", err)
	  }
	  if dc.State == DriftCaseClosed {
		  return nil
	  }
	   if err := dc.Transition(DriftCaseClosed); err != nil {
		  return err
	   }
	  now := e.clock()
	   dc.State = DriftCaseClosed
	   dc.ConfirmedBy = dismissedBy
	  dc.ConfirmedAt = &now
	   dc.CloseReason = "dismissed: " + reason
	  dc.UpdatedAt = now
	   if err := e.store.UpdateDriftCase(ctx, dc); err != nil {
		  return fmt.Errorf("polhopgate: dismiss drift case update: %w", err)
	  }
	   // 解除 DualEscalate
	  if esc, escErr := e.store.GetActiveEscalate(ctx, dc.UserID, dc.Asset, dc.BizKind); escErr == nil && esc != nil {
		   esc.State = EscalateStateResolved
		   esc.ResolvedAt = &now
		  esc.UpdatedAt = now
		  _ = e.store.UpdateEscalate(ctx, esc)
	  }
	  return nil
   }

// ─── ForceReEvaluate ──────────────────────────────────────────────────────
// ForceReEvaluateRequest 强制令牌集体复验请求（Publish 后批量挡写旧令牌）。

type ForceReEvaluateRequest struct {
	UserID string // 空=全用户
	Asset string
	BizKind     string
	CurrentGen  int64 // 当前策略世代（只挡写 PolicyGen < CurrentGen 的令牌）
	BlockReason string
}

// ForceReEvaluateReply 批量复验结果。
type ForceReEvaluateReply struct {
	BlockedCount int `json:"blocked_count"`
	SkippedCount int `json:"skipped_count"`
}

// ForceReEvaluate Publish 后批量将旧 Gen 的 issued 令牌标记为 blocked。
// 安全语义：不改账；仅挡写令牌状态，防止已发令牌在新 Gen 后接着被消费。

   func (e *Engine) ForceReEvaluate(ctx context.Context, req ForceReEvaluateRequest) (*ForceReEvaluateReply, error) {
	  limit := 500
	   tokens, err := e.store.ListTokensByUser(ctx, req.UserID, req.BizKind, limit)
	   if err != nil {
		   return nil, fmt.Errorf("polhopgate: force re-evaluate list tokens: %w", err)
	  }
	   now := e.clock()
	   reply := &ForceReEvaluateReply{}
	   for _, tok := range tokens {
		   if tok.State != TokenStateIssued {
			   reply.SkippedCount++
			   continue
		  }
		  if tok.PolicyGen >= req.CurrentGen {
			  reply.SkippedCount++
			   continue
		  }
		  reason := req.BlockReason
		   if reason == "" {
			  reason = fmt.Sprintf("force re-evaluate: new gen=%d token gen=%d", req.CurrentGen, tok.PolicyGen)
		   }
		   tok.State = TokenStateBlocked
		   tok.BlockReason = reason
		  tok.UpdatedAt = now
		   if uErr := e.store.UpdateToken(ctx, tok); uErr == nil {
			   reply.BlockedCount++
		   }
	  }
	   return reply, nil
   }

// ─── GetCase / GetToken ────────────────────────────────────────────────────
// GetDriftCase 查询漂移案例。

  func (e *Engine) GetDriftCase(ctx context.Context, caseID string) (*PolDriftCase, error) {
	  return e.store.GetDriftCaseByID(ctx, caseID)
  }

// GetToken 查询 hop 令牌。

  func (e *Engine) GetToken(ctx context.Context, tokenID string) (*HopToken, error) {
	   return e.store.GetTokenByID(ctx, tokenID)
   }

// GetEscalate 查询 DualEscalate 状态。
   func (e *Engine) GetEscalate(ctx context.Context, userID, asset, bizKind string) (*DualEscalate, error) {
	  return e.store.GetActiveEscalate(ctx, userID, asset, bizKind)
  }

// ─── 内部工具 ─────────────────────────────────────────────────────────────
// effectiveVersion 读取对用户生效的策略代次（个人钉版 > 全局 active）。
  func (e *Engine) effectiveVersion(ctx context.Context, userID, asset string) (*acctpol.HopPolicyVersion, error) {
	   if userID != "" {
		   pinned,
			  err := e.polStore.GetPinnedVersionByUser(ctx, userID, asset)
		  if err == nil && pinned != nil {
			  return pinned, nil
		   }
	   }
	  return e.polStore.GetActiveGlobalVersion(ctx, asset)
   }

// blockToken 将令牌标记为 blocked。

   func (e *Engine) blockToken(ctx context.Context, tok *HopToken, reason string, now time.Time) error {
	   tok.State = TokenStateBlocked
	   tok.BlockReason = reason
	   tok.UpdatedAt = now
	  _ = e.store.UpdateToken(ctx, tok)
	   return fmt.Errorf("%w: token=%s reason=%s", ErrGenMismatch, tok.ID, reason)
   }

// openDriftCase 开启漂移案例（越权触发路径）。
  func (e *Engine) openDriftCase(ctx context.Context, tok *HopToken, currentGen int64, trigger DriftTrigger, note string, now time.Time) error {
	  idemKey := fmt.Sprintf("unauthorized:%s:%s:%s:%d", tok.UserID, tok.BizKind, tok.ID, tok.PolicyGen)
	  _, err := e.store.GetDriftCaseByIdem(ctx, idemKey)
	   if err == nil {
		  return nil // 已有案例，幂等
	  }
	  dc := &PolDriftCase{
		   ID:     uuid.NewString(),
		  UserID: tok.UserID,
		   Asset: tok.Asset,
		   BizKind:    tok.BizKind,
		   Trigger:    trigger,
		  TriggerGen: currentGen,
		  ReplicaGen: tok.PolicyGen,
		  DivergeDelta: currentGen - tok.PolicyGen,
		  HopTokenID:   tok.ID,
		   State:        DriftCaseOpen,
		   IdemKey: idemKey,
		   BlockedAt: now,
		  ScanNote:  note,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   return e.store.InsertDriftCase(ctx, dc)
  }

// tokenToReply 将 HopToken 转换为 IssueReply。

  func (e *Engine) tokenToReply(tok *HopToken) *IssueReply {
	   return &IssueReply{
		   TokenID: tok.ID,
		  PolicyGen: tok.PolicyGen,
		  PolicyID:  tok.PolicyID,
		  ExpiresAt: tok.ExpiresAt,
		  State: tok.State,
	   }
   }

// computeTokenHash 计算令牌内容指纹。

   func computeTokenHash(tok *HopToken) string {
	  raw := fmt.Sprintf("%s:%s:%d:%d", tok.UserID, tok.BizKind, tok.PolicyGen, tok.IssuedAt.UnixNano())
	  h := sha256.Sum256([]byte(raw))
	   return fmt.Sprintf("%x", h)
  }

   func absInt64(v int64) int64 {
	  if v < 0 {
		  return -v
	   }
	  return v
   }
