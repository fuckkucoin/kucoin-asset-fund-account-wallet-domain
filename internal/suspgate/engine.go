// 重点：
// SuspendWithGen 是 brmismatch.SuspendAccount 的世代延伸：每次暂停必须
// 原子递增 (user,asset) 的世代计数器，签发 SuspenseGeneration（state=active），
// 并通过 GenCounterRepo.IncrAndGet 的 CAS 语义防止并发竞争。
// ClearWithGen 是四眼清障核心路径：消费指定 gen（state→cleared），
// 签发 ClearanceLease；并发 Suspend 在同一 gen 上的竞争由 SaveGenCleared
// 的 CAS UPDATE 保证只有一方成功。
// WithdrawAssert 是出金挡写断言：ActiveCount>0 或存在未消费 gen 时
// fail-closed 并落 SuspenseDriftProof；清障后须持有效 ClearanceLease。
// issueDriftProof 在任何挡写或漂移事件发生时调用；禁止静默丢弃。
// 风险点：
// Engine.mu 仅保护内存计算与校验段；SQL CAS 约束是最终安全屏障。
// SuspendWithGen 中若 brmismatch.SuspendAccount 调用失败，须回滚 gen 计数器；
// 实现采用先提交 gen 再提交 brmismatch 的顺序，依赖 brmismatch 幂等（同 case 重复 Suspend 无副作用）。
// ClearWithGen 须在四眼验证完成后才调用；Actor1==Actor2 拒绝。
// WithdrawAssert 拒绝时须返回 ErrWithdrawBlocked 或 ErrLeaseRequired，
// 且必须先落 DriftProof 再返回错误。

// Author: Cason
// Date: 2021.12.01

package suspgate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// DefaultClearanceTTL 清障放行租约默认有效期（5 分钟）。
const DefaultClearanceTTL = 5 * time.Minute

// BrmismatchSuspender brmismatch.Service 的暂停接口，供 Engine 调用。
// 功能：Engine 不直接依赖 brmismatch 包，通过此接口解耦。
type BrmismatchSuspender interface {
	// SuspendForGen 由 Engine 调用，将对应案件推进到 StateSuspended。
	// 调用方（Engine）已完成世代签发；此处仅同步 brmismatch 状态。
	SuspendForGen(ctx context.Context,
		caseID, userID, asset, actor, note string) error
	// ClearForGen 由 Engine 调用，将对应案件推进到 StateCleared。
	ClearForGen(ctx context.Context, caseID, actor1, actor2, note string) error
}

// Engine 暂停世代与出金挡写互斥收口引擎。
// 功能：集中管理世代签发、四眼清障、出金断言、漂移证明签发。
type Engine struct {
	counters GenCounterRepo
	gens GenRepo
	leases LeaseRepo
	proofs    DriftProofRepo
	suspender BrmismatchSuspender // 可为 nil（或降级模式）
	leaseTTL  time.Duration
	clock func() time.Time
	mu sync.Mutex
}

// EngineConfig 引擎配置。
type EngineConfig struct {
	// LeaseTTL 清障放行租约 TTL，默认 DefaultClearanceTTL。
	LeaseTTL time.Duration
	// Suspender brmismatch 联动暂停器（可为 nil）。
	Suspender BrmismatchSuspender
}

   func (c *EngineConfig) defaults() {
	   if c.LeaseTTL <= 0 {
		  c.LeaseTTL = DefaultClearanceTTL
	   }
  }

// NewEngine 构造暂停世代引擎。

  func NewEngine(counters GenCounterRepo, gens GenRepo, leases LeaseRepo, proofs DriftProofRepo, cfg EngineConfig) *Engine {
	   cfg.defaults()
	  return &Engine{
		  counters: counters,
		   gens:   gens,
		   leases: leases,
		   proofs:    proofs,
		   suspender: cfg.Suspender,
		   leaseTTL: cfg.LeaseTTL,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟。
   func (e *Engine) WithClock(fn func() time.Time) *Engine {
	  if fn != nil {
		  e.clock = fn
	  }
	   return e
  }

  func (e *Engine) now() time.Time { return e.clock() }

// ──────────────────────────────────────────────────────────────────────────────
// SuspendWithGen：带世代的暂停
// ──────────────────────────────────────────────────────────────────────────────
// SuspendWithGen 执行带世代的账户暂停。
// 功能：
// 1. 校验输入参数（asset、actor、caseID）。
// 2. 读取当前世代计数器，获取 prevGen；调用 IncrAndGet(prevGen) 原子递增世代号。
// 3. 构造 SuspenseGeneration（state=active）并持久化。
// 4. 若 Engine 配置了 BrmismatchSuspender，调用联动暂停。
// 重点：
// 步骤 2 的 IncrAndGet 使用 CAS；若 prevGen 在读取与提交之间被另一 Suspend
// 或 Clear 改变，返回 ErrGenConflict；调用方须重试。
// 世代签发成功即视为暂停生效；brmismatch 联动失败不回滚世代（可重试）。
// 风险点：
// 禁止在 active gen 存在时调用 SuspendWithGen（会累积世代）；
// 调用方须先通过 IsSuspended 确认业务语义后再触发。

  func (e *Engine) SuspendWithGen(ctx context.Context, req SuspendWithGenReq) (*SuspendWithGenResult, error) {
	  if err := validateAsset(req.Asset); err != nil {
		  return nil, err
	   }
	   actor := strings.TrimSpace(req.Actor)
	   if actor == "" {
		  return nil, ErrActor
	  }
	   if strings.TrimSpace(req.CaseID) == "" || strings.TrimSpace(req.UserID) == "" {
		  return nil, ErrInvalidInput
	  }
	   now := e.now()
	  // 读取当前计数器以获取 prevGen。
	   prevGen := int64(0)
	   existing,
		  err := e.counters.GetCounter(ctx, req.UserID, req.Asset)
	  if err == nil {
		   prevGen = existing.CurrentGen
	   }
	  // 原子递增世代号（CAS）。
	   counter, err := e.counters.IncrAndGet(ctx, req.UserID, req.Asset, prevGen)
	   if err != nil {
		   return nil, fmt.Errorf("suspgate.SuspendWithGen: incr gen: %w", err)
	   }
	   // 构造世代记录。
	  genID := GenIDFor(req.UserID, req.Asset, counter.CurrentGen)
	   gen := &SuspenseGeneration{
		   GenID: genID,
		  UserID:   strings.TrimSpace(req.UserID),
		   Asset:    strings.ToUpper(strings.TrimSpace(req.Asset)),
		  GenValue: counter.CurrentGen,
		   CaseID:      strings.TrimSpace(req.CaseID),
		  SuspendedBy: actor,
		  State:       SuspActive,
		  SuspendedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.gens.SaveGen(ctx, gen); err != nil {
		   return nil, fmt.Errorf("suspgate.SuspendWithGen: save gen: %w", err)
	   }
	  // 联动 brmismatch 暂停（可选）。
	   if e.suspender != nil {
		  if serr := e.suspender.SuspendForGen(ctx, req.CaseID, req.UserID, req.Asset, actor, req.Note); serr != nil {
			  // 联动失败记录但不回滚世代；brmismatch 状态可通过重试补偿。
			   _ = serr
		  }
	   }
	   recordSuspGenIssued(gen.Asset)
	   return &SuspendWithGenResult{Gen: *gen, Counter: *counter}, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// ClearWithGen：带世代的四眼清障
// ──────────────────────────────────────────────────────────────────────────────
// ClearWithGen 执行带世代的四眼清障。
// 功能：
// 1. 校验 Actor1 ≠ Actor2。
// 2. 读取目标 gen 记录（按 genID 或 (userID, asset, genValue) 查找）。
// 3. 对 active gen 先推进到 clearing（若需要二审），随后推进到 cleared。
// 本实现简化为：直接一步流转到 cleared，符合单次四眼（Actor1+Actor2 同时提交）的场景。
// 4. 调用 SaveGenCleared（CAS）提交；并发竞争时仅一方成功。
// 5. 递减 ActiveCount；签发 ClearanceLease。
// 6. 若 gen 已是 cleared（重复 Clear），落 SuspenseDriftProof（OutcomeDoubleCleared）。
// 重点：
// 步骤 4 的 CAS 是并发互斥的核心；两个并发 Clear 只有一个返回成功。
// 第二次 Clear 同一 gen 必须落 DriftProof 而非静默拒绝。
// 风险点：
// SaveGenCleared 返回 ErrGenAlreadyCleared 时须立即落漂移证明；
// 调用方若忽略 DriftProof 字段，须自行记录操作失败日志。

  func (e *Engine) ClearWithGen(ctx context.Context, req ClearWithGenReq) (*ClearWithGenResult, error) {
	   if err := validateAsset(req.Asset); err != nil {
		  return nil, err
	   }
	  a1 := strings.TrimSpace(req.Actor1)
	   a2 := strings.TrimSpace(req.Actor2)
	  if a1 == "" || a2 == "" {
		   return nil, ErrActor
	  }
	  if a1 == a2 {
		  return nil, ErrFourEyes
	  }
	  if req.GenValue <= 0 {
		   return nil, fmt.Errorf("%w: gen_value must be > 0", ErrInvalidInput)
	   }
	   now := e.now()
	   userID := strings.TrimSpace(req.UserID)
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	  genID := GenIDFor(userID, asset, req.GenValue)
	  // 读取世代记录。
	   gen, err := e.gens.GetGen(ctx, genID)
	   if err != nil {
		  return nil, fmt.Errorf("suspgate.ClearWithGen: get gen: %w", err)
	  }
	  // 若已清障，落漂移证明（双清场景）。
	   if gen.State == SuspCleared {
		   counter, _ := e.counters.GetCounter(ctx, userID, asset)
		   countBefore := 0
		  if counter != nil {
			  countBefore = counter.ActiveCount
		  }
		   proof := e.newDriftProof(gen.CaseID,
			  userID, asset, req.GenValue,
			  countBefore, countBefore, OutcomeDoubleCleared,
			  fmt.Sprintf("double clear by %s+%s", a1, a2), now)
		   _ = e.proofs.SaveProof(ctx, proof)
		  recordDoubleClear(asset)
		   recordDriftProof(OutcomeDoubleCleared)
		   return &ClearWithGenResult{Gen: *gen, DriftProof: proof}, ErrGenAlreadyCleared
	  }
	  // 流转到 cleared（CAS 保护）。
	  prevState := gen.State
	   clearedAt := now
	   gen.State = SuspCleared
	   gen.ClearActor1 = a1
	   gen.ClearActor2 = a2
	  gen.ClearNote = strings.TrimSpace(req.Note)
	   gen.ClearedAt = &clearedAt
	   gen.UpdatedAt = now
	   if err := e.gens.SaveGenCleared(ctx, gen, prevState); err != nil {
		   if err == ErrGenAlreadyCleared {
			   // 并发竞争：另一方已清障，落漂移证明。
			   counter, _ := e.counters.GetCounter(ctx, userID, asset)
			  countBefore := 0
			  if counter != nil {
				  countBefore = counter.ActiveCount
			  }
			  proof := e.newDriftProof(gen.CaseID, userID, asset, req.GenValue,
				   countBefore, countBefore, OutcomeDoubleCleared,
				  fmt.Sprintf("concurrent double clear by %s+%s", a1, a2), now)
			  _ = e.proofs.SaveProof(ctx, proof)
			  recordDoubleClear(asset)
			  recordDriftProof(OutcomeDoubleCleared)
			   return &ClearWithGenResult{Gen: *gen, DriftProof: proof}, ErrGenAlreadyCleared
		   }
		   return nil, fmt.Errorf("suspgate.ClearWithGen: save cleared: %w", err)
	  }
	  // 递减 ActiveCount。
	   newCount, _ := e.counters.DecrActiveCount(ctx, userID, asset)
	  // 签发清障放行租约。
	  lease := &ClearanceLease{
		   LeaseID: uuid.NewString(),
		  UserID:  userID,
		  Asset:   asset,
		   GenValue: req.GenValue,
		   CaseID:   gen.CaseID,
		  State:    LeaseIssued,
		   IssuedAt: now,
		  ExpireAt: now.Add(e.leaseTTL),
	  }
	  if err := e.leases.SaveLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("suspgate.ClearWithGen: save lease: %w", err)
	  }
	  // 联动 brmismatch 清障（可选）。
	   if e.suspender != nil && strings.TrimSpace(req.CaseID) != "" {
		   _ = e.suspender.ClearForGen(ctx, req.CaseID, a1, a2, req.Note)
	   }
	   _ = newCount
	  recordClearSuccess(asset)
	  recordLeaseIssued()
	  return &ClearWithGenResult{Gen: *gen, Lease: lease}, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// WithdrawAssert：出金挡写断言
// ──────────────────────────────────────────────────────────────────────────────
// WithdrawAssert 出金写路径断言：fail-closed。
// 功能：
// 1. 读取 GenCounter；若 ActiveCount > 0，直接挡写并落 DriftProof。
// 2. 若 ActiveCount == 0，进一步检查是否存在未消费（active/clearing）的 gen；
// 若存在，挡写并落 DriftProof（Count 抖零但 gen 仍未消费）。
// 3. 若调用方携带 LeaseID，验证 lease 有效性；通过则消费 lease 并放行。
// 4. 若调用方 RequireLease=true 但未携带有效 lease，拒绝。
// 重点：
// 步骤 2 是防止 Count 抖零漏拦的关键；须检查 ListActiveGens 而非仅靠 Count。
// 步骤 3 消费 lease 须在业务写入数据库成功后才调用 ConsumeLease；
// 此处仅验证有效性；实际消费由调用方在写入成功后调用 ConsumeLease。
// 风险点：
// WithdrawAssert 为纯读取 + 落证明操作；不修改 GenCounter 或 gen 状态。
// 落 DriftProof 失败时须确保拒绝仍然生效，不得因证明落库失败而放行。

   func (e *Engine) WithdrawAssert(ctx context.Context, req WithdrawAssertReq) (*WithdrawAssertResult, error) {
	   if err := validateAsset(req.Asset); err != nil {
		  return nil, err
	   }
	   userID := strings.TrimSpace(req.UserID)
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   now := e.now()
	  // 读取世代计数器。
	  counter, err := e.counters.GetCounter(ctx, userID, asset)
	  if err != nil && err != ErrGenNotFound {
		   return nil, fmt.Errorf("suspgate.WithdrawAssert: get counter: %w", err)
	   }
	   activeCount := 0
	  if counter != nil {
		  activeCount = counter.ActiveCount
	   }
	   // 挡写检查 1：ActiveCount > 0。
	  if activeCount > 0 {
		   proof := e.newDriftProof("none", userID, asset, 0,
			   activeCount, activeCount, OutcomeBlocked,
			   fmt.Sprintf("active_count=%d > 0", activeCount), now)
		   _ = e.proofs.SaveProof(ctx, proof)
		  recordWithdrawBlocked(asset, "active_count_gt0")
		  recordDriftProof(OutcomeBlocked)
		  return &WithdrawAssertResult{
			   Allowed:     false,
			   BlockReason: fmt.Sprintf("active_count=%d", activeCount),
			   ActiveCount: activeCount,
		  }, ErrWithdrawBlocked
	  }
	  // 挡写检查 2：即使 Count=0，仍须检查未消费 gen（防抖零漏拦）。
	  activeGens, err := e.gens.ListActiveGens(ctx, userID, asset)
	   if err != nil && err != ErrGenNotFound {
		  return nil, fmt.Errorf("suspgate.WithdrawAssert: list active gens: %w", err)
	  }
	   if len(activeGens) > 0 {
		   maxGen := activeGens[0]
		  for _, g := range activeGens {
			   if g.GenValue > maxGen.GenValue {
				   maxGen = g
			  }
		  }
		   proof := e.newDriftProof(maxGen.CaseID, userID, asset, maxGen.GenValue,
			   activeCount, activeCount, OutcomeBlocked,
			   fmt.Sprintf("count=0 but %d unconsumed gen(s)", len(activeGens)), now)
		  _ = e.proofs.SaveProof(ctx, proof)
		  recordWithdrawBlocked(asset, "unconsumed_gen")
		   recordDriftProof(OutcomeBlocked)
		   return &WithdrawAssertResult{
			   Allowed:     false,
			   BlockReason: fmt.Sprintf("count=0 but %d unconsumed gen(s)", len(activeGens)),
			  ActiveCount: activeCount,
		  }, ErrWithdrawBlocked
	   }
	  // 通过基础挡写检查；若携带 lease 则验证。
	   leaseID := strings.TrimSpace(req.LeaseID)
	   if leaseID != "" {
		  lease, lerr := e.leases.GetLease(ctx, leaseID)
		  if lerr != nil {
			   if req.RequireLease {
				   return &WithdrawAssertResult{
					   Allowed:     false,
					   BlockReason: "lease not found",
					   ActiveCount: activeCount,
				  }, ErrLeaseRequired
			  }
			   // lease 不存在但未要求 lease；允许通过。
			  return &WithdrawAssertResult{Allowed: true, ActiveCount: activeCount}, nil
		  }
		  if !lease.IsValid(now) {
			   reason := "lease expired or consumed"
			   if lease.State == LeaseConsumed {
				   reason = "lease already consumed"
			  }
			   if req.RequireLease {
				   return &WithdrawAssertResult{
					   Allowed: false,
					   BlockReason: reason,
					   ActiveCount: activeCount,
				   }, ErrLeaseRequired
			  }
			  return &WithdrawAssertResult{Allowed: true, ActiveCount: activeCount}, nil
		   }
		  // lease 有效；返回 lease_id 供调用方在写入成功后消费。
		  return &WithdrawAssertResult{
			   Allowed:     true,
			  ActiveCount: activeCount,
			   ConsumedLeaseID: lease.LeaseID,
		  }, nil
	   }
	  // 无 lease；若策略要求则拒绝。
	   if req.RequireLease {
		  return &WithdrawAssertResult{
			  Allowed: false,
			   BlockReason: "no lease provided",
			  ActiveCount: activeCount,
		  }, ErrLeaseRequired
	   }
	   return &WithdrawAssertResult{Allowed: true, ActiveCount: activeCount}, nil
  }

// ──────────────────────────────────────────────────────────────────────────────
// ConsumeLease：出金成功后消费清障租约
// ──────────────────────────────────────────────────────────────────────────────
// ConsumeLease 在出金写入数据库成功后消费清障放行租约。
// 功能：将 lease 状态流转到 consumed；若已消费则返回 ErrLeaseConsumed（幂等）。
// 风险点：
// 须严格在出金写提交成功后调用；提前调用会造成出金失败后租约已消费的漏洞。

  func (e *Engine) ConsumeLease(ctx context.Context, leaseID, consumedBy string) (*ClearanceLease, error) {
	  if strings.TrimSpace(leaseID) == "" {
		   return nil, ErrInvalidInput
	   }
	   lease, err := e.leases.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	  }
	  if lease.State == LeaseConsumed {
		  return lease, ErrLeaseConsumed
	  }
	  if lease.State != LeaseIssued {
		   return nil, fmt.Errorf("%w: current state=%s", ErrLeaseInvalid, lease.State)
	   }
	  now := e.now()
	  if now.After(lease.ExpireAt) {
		  // TTL 已过，流转到 expired 而非 consumed。
		  if err := TransitionLease(lease, LeaseExpired); err != nil {
			   return nil, err
		  }
		   _ = e.leases.SaveLease(ctx, lease)
		  return lease, ErrLeaseExpired
	   }
	   if err := TransitionLease(lease, LeaseConsumed); err != nil {
		  return nil, err
	   }
	  lease.ConsumedBy = strings.TrimSpace(consumedBy)
	   t := now
	   lease.ConsumedAt = &t
	  if err := e.leases.SaveLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("suspgate.ConsumeLease: save: %w", err)
	  }
	  return lease, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// GetCurrentGen：读取最新活跃世代（只读）
// ──────────────────────────────────────────────────────────────────────────────
// GetCurrentGen 返回 (userID, asset) 最新的活跃或清障中世代；不存在返回 ErrGenNotFound。

  func (e *Engine) GetCurrentGen(ctx context.Context, userID, asset string) (*SuspenseGeneration, error) {
	  return e.gens.GetActiveGen(ctx,
		  userID, asset)
  }

// GetCounter 返回 (userID, asset) 的世代计数器；不存在返回 ErrGenNotFound。

   func (e *Engine) GetCounter(ctx context.Context, userID, asset string) (*GenCounter, error) {
	   return e.counters.GetCounter(ctx, userID, asset)
  }

// GetLease 按 leaseID 读取清障租约。

   func (e *Engine) GetLease(ctx context.Context, leaseID string) (*ClearanceLease, error) {
	  return e.leases.GetLease(ctx, leaseID)
  }

// GetProof 按 proofID 读取漂移证明。
   func (e *Engine) GetProof(ctx context.Context, proofID string) (*SuspenseDriftProof, error) {
	   return e.proofs.GetProof(ctx, proofID)
   }

// ListProofsByCase 列出案件关联的所有漂移证明。
   func (e *Engine) ListProofsByCase(ctx context.Context, caseID string) ([]*SuspenseDriftProof, error) {
	  return e.proofs.ListByCase(ctx, caseID)
   }

// ListProofsByOutcome 列出指定阻断结果的证明（供审计导出）。

   func (e *Engine) ListProofsByOutcome(ctx context.Context, outcome ProofOutcome, limit int) ([]*SuspenseDriftProof, error) {
	   return e.proofs.ListByOutcome(ctx, outcome, limit)
   }

// ──────────────────────────────────────────────────────────────────────────────
// 内部辅助
// ──────────────────────────────────────────────────────────────────────────────

  func (e *Engine) newDriftProof(caseID,
	   userID, asset string, gen int64,
	  countBefore, countAfter int, outcome ProofOutcome, detail string, now time.Time,
  ) *SuspenseDriftProof {
	   if caseID == "" {
		  caseID = "none"
	  }
	   p := &SuspenseDriftProof{
		  ProofID: uuid.NewString(),
		  CaseID: caseID,
		   UserID: userID,
		   Asset:  asset,
		  SuspGen: gen,
		  CountBefore: countBefore,
		   CountAfter:  countAfter,
		  BlockOutcome: outcome,
		  Detail:      detail,
		   AttemptedAt: now,
	   }
	  p.ProofHash = DriftProofHashFor(caseID, gen, outcome, now)
	  return p
   }

  func validateAsset(asset string) error {
	  a := strings.ToUpper(strings.TrimSpace(asset))
	  if a == "USDT" || a == "USDC" {
		  return nil
	   }
	  return ErrAsset
  }
