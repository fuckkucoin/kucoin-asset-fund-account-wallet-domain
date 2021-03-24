// @FileName: engine.go
// @Summary: 归集冷却标记与执行竞态收口引擎——核心执行路径。
// 重点：
// Engine 是本包的核心入口，封装 Mark / CheckExecAllowed / ClearCool /
// RecordBreach / ExportProof 五个关键操作。
// 所有状态变更须经过 FSM.Transition 校验，禁止直接赋值 State 字段。
// CheckExecAllowed 是资金安全核心：世代不符、冷却窗活跃、证明缺失三类拒绝
// 均必须落 CoolBreachCase，禁止静默拒绝。
// 风险点：
// Mark 与 CheckExecAllowed 之间存在竞态窗口。
// Engine.mu 互斥锁保护 Mark/ClearCool/CheckExecAllowed 的幂等性；
// SQL 实现依赖数据库唯一键约束作为额外保护层。
// ClearCool 必须先签发并落库 CoolProof，再将 CoolLease 流转到 cleared；
// 如果 ProofID 为空则拒绝 Clear，防止无证明放行。
// @Author: Ronan
// @Created: 2021.03.23

package coolmarkgate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Engine 冷却标记与执行竞态收口引擎。
// 功能：集中管理冷却世代签发、执行路径复验、违约落仓和冷却证明签发。

type Engine struct {
	leases CoolLeaseRepo
	cases  BreachCaseRepo
	proofs CoolProofRepo
	gens   CoolGenRepo
	clock func() time.Time
	// mu 保护 Mark/ClearCool/CheckExecAllowed 的原子性（内存实现场景）。
	mu sync.Mutex
}

// NewEngine 构造引擎实例。
  func NewEngine(
	  leases CoolLeaseRepo,
	   cases BreachCaseRepo,
	   proofs CoolProofRepo,
	   gens CoolGenRepo,
  ) *Engine {
	   return &Engine{
		   leases: leases,
		  cases: cases,
		  proofs: proofs,
		  gens:   gens,
		  clock:  func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟；仅场景使用。
  func (e *Engine) WithClock(c func() time.Time) *Engine {
	   if c != nil {
		  e.clock = c
	   }
	   return e
  }

  // ─────────────────────────────────────────────────────────────────
  // Mark：触发冷却 + 抬升世代
  // ─────────────────────────────────────────────────────────────────
   // Mark 为指定资产签发冷却租约并抬升世代号。
   // 功能：
   // 1. 读取当前 CoolGeneration；若不存在则从世代 0 开始初始化。
  // 2. 若该资产已有活跃冷却租约（State != cleared），检查冷却窗是否仍活跃：
   // - 若仍活跃则复用现有租约（不重复 Mark），Reused=true 返回。
   // - 若已超时但未 Clear，则强制视为可覆盖。
   // 3. 创建新 CoolLease，State=marked，CoolUntil=now+Window。
  // 4. 递增 Generation 并更新 CoolGeneration.ActiveLeaseID。
   // 5. 持久化 CoolGeneration 与 CoolLease。
  // 风险点：须在 Engine.mu 锁内执行，防止并发 Mark 对同资产双写世代。
  func (e *Engine) Mark(ctx context.Context, in MarkInput) (*MarkResult, error) {
	   if strings.TrimSpace(in.Asset) == "" {
		  return nil, fmt.Errorf("%w: asset required", ErrInvalidInput)
	   }
	   asset := strings.ToUpper(strings.TrimSpace(in.Asset))
	   window := in.Window
	   if window <= 0 {
		  window = CoolDefaultWindow
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  now := e.clock()
	  // 读取或初始化世代。
	  gen,
		   err := e.gens.GetGen(ctx, asset)
	   if err != nil {
		  if !errors.Is(err, ErrCoolLeaseNotFound) {
			  return nil, fmt.Errorf("coolmarkgate.Mark: load gen: %w", err)
		  }
		   gen = &CoolGeneration{
			   GenID: GenIDFor(asset),
			   Asset: asset,
			   Generation: 0,
			  UpdatedAt:  now,
		   }
	  }
	  // 检查是否已有活跃租约。
	   existing, err := e.leases.GetActiveLeaseByAsset(ctx, asset)
	   if err == nil && existing.IsActive(now) {
		   // 冷却窗仍活跃：复用现有租约，不再抬升世代。
		  recordMarkMetric(asset, true)
		  return &MarkResult{
			  Lease:      existing,
			   Generation: existing.Generation,
			  Reused:     true,
		  }, nil
	   }
	   // 新世代。
	  gen.Generation++
	   leaseID := uuid.NewString()
	   coolUntil := now.Add(window)
	  lease := &CoolLease{
		  LeaseID: leaseID,
		  Asset:   asset,
		  Generation: gen.Generation,
		  State: CoolMarked,
		  MarkedAt:  now,
		   CoolUntil: coolUntil,
		  Reason: strings.TrimSpace(in.Reason),
	  }
	  gen.ActiveLeaseID = leaseID
	   gen.UpdatedAt = now
	   if err := e.leases.SaveLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("coolmarkgate.Mark: save lease: %w", err)
	  }
	  if err := e.gens.SaveGen(ctx, gen); err != nil {
		  return nil, fmt.Errorf("coolmarkgate.Mark: save gen: %w", err)
	  }
	  recordMarkMetric(asset, false)
	  return &MarkResult{
		   Lease: lease,
		  Generation: gen.Generation,
		   Reused:     false,
	  }, nil
   }

// ─────────────────────────────────────────────────────────────────
// CheckExecAllowed：Exec 路径复验
// ─────────────────────────────────────────────────────────────────
// CheckExecAllowed 验证归集执行是否允许放行。
// 功能：
// 1. 读取当前 CoolGeneration；若不存在，表示该资产从未 Mark，直接放行（Generation=0，Allowed=true）。
// 2. 校验调用方携带的 CoolGen 是否与当前世代一致；不一致则拒绝并落 CoolBreachCase。
// 3. 检查当前活跃 CoolLease 的冷却窗是否已结束：
// - 若仍活跃则拒绝并落 CoolBreachCase（Decision=blocked）。
// 4. 验证 CoolProof 是否存在且世代匹配：
// - 若证明缺失则拒绝并落 CoolBreachCase。
// 5. 全部通过则返回 Allowed=true 与 ProofID。
// 风险点：
// 拒绝路径必须落 CoolBreachCase，禁止静默拒绝。
// Allowed=true 时必须同时返回 ProofID；空 ProofID 等同拒绝。
   func (e *Engine) CheckExecAllowed(ctx context.Context, in ExecVerifyInput) (*ExecVerifyResult, error) {
	  if strings.TrimSpace(in.Asset) == "" {
		   return nil, fmt.Errorf("%w: asset required", ErrInvalidInput)
	   }
	  asset := strings.ToUpper(strings.TrimSpace(in.Asset))
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  now := e.clock()
	   // 读取世代；若不存在则资产未被 Mark，直接放行。
	   gen, err := e.gens.GetGen(ctx, asset)
	  if err != nil {
		   if errors.Is(err, ErrCoolLeaseNotFound) {
			   return &ExecVerifyResult{Allowed: true, Generation: 0}, nil
		  }
		   return nil, fmt.Errorf("coolmarkgate.CheckExecAllowed: load gen: %w", err)
	   }
	   // 世代号校验。
	  if in.CoolGen != gen.Generation {
		   bc, bErr := e.recordBreach(ctx, asset, gen.Generation, gen.ActiveLeaseID, in.ExecID,
			  BreachBlocked, now,
			  fmt.Sprintf("cool_gen mismatch: got %d want %d", in.CoolGen, gen.Generation))
		   if bErr != nil {
			   return nil, bErr
		  }
		   recordExecBlockedMetric(asset, "gen_mismatch")
		   recordBreachMetric(asset, BreachBlocked)
		  return &ExecVerifyResult{
			   Allowed:    false,
			  Generation: gen.Generation,
			  BreachCaseID: bc.CaseID,
			  Reason:       fmt.Sprintf("cool_gen mismatch: got %d want %d", in.CoolGen, gen.Generation),
		  }, ErrGenMismatch
	  }
	  // 检查冷却窗是否仍活跃。
	   if gen.ActiveLeaseID != "" {
		  lease, lErr := e.leases.GetLease(ctx, gen.ActiveLeaseID)
		  if lErr == nil && lease.IsActive(now) {
			  bc, bErr := e.recordBreach(ctx, asset, gen.Generation, gen.ActiveLeaseID, in.ExecID,
				  BreachBlocked, now,
				  fmt.Sprintf("cool window active until %s", lease.CoolUntil.UTC().Format(time.RFC3339)))
			  if bErr != nil {
				  return nil, bErr
			   }
			   recordExecBlockedMetric(asset, "window_active")
			   recordBreachMetric(asset, BreachBlocked)
			  return &ExecVerifyResult{
				   Allowed:    false,
				  Generation: gen.Generation,
				   BreachCaseID: bc.CaseID,
				  Reason:       fmt.Sprintf("cool window active until %s", lease.CoolUntil.UTC().Format(time.RFC3339)),
			   }, ErrCoolWindowActive
		  }
	  }
	  // 验证冷却证明。
	   proof, pErr := e.proofs.GetProofByAssetGen(ctx, asset, gen.Generation)
	   if pErr != nil {
		   bc, bErr := e.recordBreach(ctx, asset, gen.Generation, gen.ActiveLeaseID, in.ExecID,
			   BreachBlocked, now,
			  "cool proof missing for current generation")
		   if bErr != nil {
			  return nil, bErr
		  }
		   recordExecBlockedMetric(asset, "proof_missing")
		   recordBreachMetric(asset, BreachBlocked)
		  return &ExecVerifyResult{
			  Allowed:    false,
			   Generation: gen.Generation,
			   BreachCaseID: bc.CaseID,
			   Reason: "cool proof missing for current generation",
		   }, ErrNoProof
	  }
	  recordExecAllowedMetric(asset)
	   return &ExecVerifyResult{
		  Allowed: true,
		   ProofID: proof.ProofID,
		  Generation: gen.Generation,
	  }, nil
   }

// ─────────────────────────────────────────────────────────────────
// ClearCool：冷却窗结束后签发证明并 cleared
// ─────────────────────────────────────────────────────────────────
// ClearCool 冷却窗结束后签发 CoolProof 并将 CoolLease 流转到 cleared。
// 功能：
// 1. 按 LeaseID 加载 CoolLease，校验状态与 CoolUntil。
// 2. CoolUntil 未到则拒绝（冷却窗仍活跃，不可提前 Clear）。
// 3. 构造 CoolProof 并落库。
// 4. 将 CoolLease 状态流转到 cleared，写入 ProofID 与 ClearedAt。
// 5. 更新 CoolGeneration.ActiveLeaseID 为空（无活跃冷却）。
// 风险点：
// ClearCool 由 worker 周期调用；幂等——已 cleared 的租约直接返回现有证明。
// 签发 CoolProof 与更新 CoolLease 之间若发生宕机，证明已存在但租约未 cleared；
// 重试时须先查证明是否已存在（GetProofByLease），避免重复插入。
  func (e *Engine) ClearCool(ctx context.Context, leaseID string) (*CoolProof, error) {
	   if strings.TrimSpace(leaseID) == "" {
		   return nil, fmt.Errorf("%w: leaseID required", ErrInvalidInput)
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  now := e.clock()
	   lease, err := e.leases.GetLease(ctx, leaseID)
	  if err != nil {
		   return nil, err
	  }
	  // 已 cleared：返回现有证明。
	  if lease.State == CoolCleared {
		  proof,
			  pErr := e.proofs.GetProofByLease(ctx, leaseID)
		   if pErr != nil {
			  return nil, fmt.Errorf("coolmarkgate.ClearCool: lease cleared but proof missing: %w", pErr)
		  }
		  return proof, nil
	   }
	   // 冷却窗仍活跃：拒绝提前 Clear。
	  if now.Before(lease.CoolUntil) {
		   return nil, fmt.Errorf("%w: cool until %s, now %s",
			  ErrCoolWindowActive, lease.CoolUntil.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	   }
	   // 检查证明是否已存在（幂等恢复路径）。
	   existProof, pErr := e.proofs.GetProofByLease(ctx, leaseID)
	   if pErr == nil {
		  // 证明已存在；仅更新租约状态。
		   t := now
		  lease.ClearedAt = &t
		   lease.ProofID = existProof.ProofID
		   if err := Transition(lease, CoolCleared); err != nil {
			  return nil, err
		  }
		   if err := e.leases.SaveLease(ctx, lease); err != nil {
			  return nil, fmt.Errorf("coolmarkgate.ClearCool: save lease: %w", err)
		   }
		   e.clearGenActiveLeaseID(ctx, lease.Asset, leaseID, now)
		   return existProof, nil
	   }
	   // 签发新证明。
	   proof := &CoolProof{
		   ProofID: uuid.NewString(),
		  LeaseID: leaseID,
		   Asset:   lease.Asset,
		  Generation: lease.Generation,
		   CoolUntil:  lease.CoolUntil,
		  SignedAt:   now,
	   }
	  proof.Digest = proof.ComputeDigest()
	   if err := e.proofs.SaveProof(ctx, proof); err != nil {
		   return nil, fmt.Errorf("coolmarkgate.ClearCool: save proof: %w", err)
	   }
	   // 流转 CoolLease 到 cleared。
	  t := now
	   lease.ClearedAt = &t
	  lease.ProofID = proof.ProofID
	  if err := Transition(lease, CoolCleared); err != nil {
		  return nil, err
	   }
	  if err := e.leases.SaveLease(ctx, lease); err != nil {
		  return nil, fmt.Errorf("coolmarkgate.ClearCool: save lease cleared: %w", err)
	   }
	  e.clearGenActiveLeaseID(ctx, lease.Asset, leaseID, now)
	   recordProofIssuedMetric(lease.Asset)
	   return proof, nil
  }

// clearGenActiveLeaseID 清除 CoolGeneration 的 ActiveLeaseID（若仍指向本租约）。
   func (e *Engine) clearGenActiveLeaseID(ctx context.Context, asset, leaseID string, now time.Time) {
	   gen, err := e.gens.GetGen(ctx, asset)
	  if err != nil {
		  return
	   }
	  if gen.ActiveLeaseID != leaseID {
		   return
	  }
	  gen.ActiveLeaseID = ""
	   gen.UpdatedAt = now
	   _ = e.gens.SaveGen(ctx, gen)
   }

// ─────────────────────────────────────────────────────────────────
// RecordSentCheck：MarkSent 入口校验（独立于 CheckExecAllowed 的最后防线）
// ─────────────────────────────────────────────────────────────────
// RecordSentCheck 在 MarkSent 最终提交前二次校验冷却状态。
// 功能：与 CheckExecAllowed 逻辑一致，但作为 MarkSent 入口独立调用层；
// 两层校验确保即使 CheckExecAllowed 被绕过，MarkSent 仍不放行。
// 风险点：
// 本函数不替代 CheckExecAllowed；两者须在不同调用点分别执行。
// 返回 ErrCoolWindowActive 时必须由调用方中止 MarkSent 流程并落 BreachCase。

  func (e *Engine) RecordSentCheck(ctx context.Context, in ExecVerifyInput) (*ExecVerifyResult, error) {
	   return e.CheckExecAllowed(ctx, in)
   }

// ─────────────────────────────────────────────────────────────────
// 内部辅助
// ─────────────────────────────────────────────────────────────────
// recordBreach 构造并落库 CoolBreachCase；调用方须持 Engine.mu 锁。

   func (e *Engine) recordBreach(
	  ctx context.Context,
	   asset string,
	  generation int64,
	   leaseID, execID string,
	  decision BreachDecision,
	   now time.Time,
	   reason string,
  ) (*CoolBreachCase, error) {
	  bc := &CoolBreachCase{
		  CaseID: uuid.NewString(),
		   Asset:  asset,
		  Generation: generation,
		   LeaseID:     leaseID,
		  ExecID:      execID,
		   AttemptedAt: now,
		  Decision:    decision,
		  Reason:      reason,
		   Resolved:    false,
	   }
	   if err := e.cases.SaveCase(ctx, bc); err != nil {
		   return nil, fmt.Errorf("coolmarkgate.recordBreach: save case: %w", err)
	  }
	   return bc, nil
   }

// ─────────────────────────────────────────────────────────────────
// 读取 API（只读，供他包引用）
// ─────────────────────────────────────────────────────────────────
// GetCoolGen 返回指定资产的当前世代号（只读）；供他包引用。
// 若资产从未 Mark，返回 0。
  func (e *Engine) GetCoolGen(ctx context.Context, asset string) (int64, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   gen, err := e.gens.GetGen(ctx, asset)
	   if err != nil {
		   if errors.Is(err, ErrCoolLeaseNotFound) {
			   return 0, nil
		   }
		  return 0, err
	   }
	   return gen.Generation, nil
  }

// GetActiveLease 返回指定资产的活跃冷却租约（只读）；若无活跃租约返回 ErrCoolLeaseNotFound。
  func (e *Engine) GetActiveLease(ctx context.Context, asset string) (*CoolLease, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  return e.leases.GetActiveLeaseByAsset(ctx, asset)
  }

// GetProof 返回指定 ProofID 的 CoolProof（只读）。

  func (e *Engine) GetProof(ctx context.Context, proofID string) (*CoolProof, error) {
	   return e.proofs.GetProof(ctx, proofID)
   }

// ListProofsByAsset 返回指定资产的 CoolProof 列表（最多 limit 条）。
   func (e *Engine) ListProofsByAsset(ctx context.Context, asset string, limit int) ([]*CoolProof, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   return e.proofs.ListByAsset(ctx, asset, limit)
   }

// ListBreachCasesByAsset 返回指定资产的 CoolBreachCase 列表（最多 limit 条）。
   func (e *Engine) ListBreachCasesByAsset(ctx context.Context, asset string, limit int) ([]*CoolBreachCase, error) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   return e.cases.ListByAsset(ctx, asset, limit)
   }

// ExpireStaleLeases 将 CoolUntil 已过期但未 cleared 的租约触发 ClearCool。
// 功能：由 CoolWindowPatrolWorker 周期调用；幂等，已 cleared 的租约跳过。
   func (e *Engine) ExpireStaleLeases(ctx context.Context, batchSize int) (int, error) {
	   now := e.clock()
	   leases, err := e.leases.ListExpiredUncleared(ctx, now, batchSize)
	  if err != nil {
		  return 0, err
	  }
	   cleared := 0
	   for _, l := range leases {
		   if _, cErr := e.ClearCool(ctx, l.LeaseID); cErr == nil {
			   cleared++
		   }
	   }
	   return cleared, nil
  }
