package trexecgate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/treasury"
)

// InventoryBook 热库存观察接口，允许注入 mock（场景）或hotinv.Book（场景）。
type InventoryBook interface {
	Observe(ctx context.Context, st ledger.Store) ([]hotinv.Snapshot, error)
}

// Engine 库务计划执行租约引擎。
// 功能：集中管理计划执行的租约绑定、复核、放行和终态落库。

type Engine struct {
	leases LeaseRepo
	proofs DriftProofRepo
	gens GenRepo
	inv  InventoryBook
	cat *treasury.Catalog
	clock    func() time.Time
	leaseTTL time.Duration
	// bindMu 保护幂等键检查与创建的原子性（内存实现场景）。
	// SQL 实现依赖数据库唯一键约束，此锁仍可作为额外保护层。
	bindMu sync.Mutex
}

// 引入 sync 包。
// NewEngine 构造引擎实例（使用 *hotinv.Book 路径）。

   func NewEngine(leases LeaseRepo, proofs DriftProofRepo, gens GenRepo, inv *hotinv.Book, cat *treasury.Catalog) *Engine {
	   return NewEngineWithObs(leases, proofs, gens, inv, cat)
  }

// NewEngineWithObs 构造引擎实例，允许注入自定义 InventoryBook（场景）。
   func NewEngineWithObs(leases LeaseRepo, proofs DriftProofRepo, gens GenRepo, inv InventoryBook, cat *treasury.Catalog) *Engine {
	   if cat == nil {
		   cat = treasury.DefaultCatalog()
	   }
	  return &Engine{
		  leases: leases,
		   proofs: proofs,
		  gens: gens,
		  inv:   inv,
		   cat:   cat,
		   clock: func() time.Time { return time.Now().UTC() },
		  leaseTTL: LeaseDefaultTTL,
	  }
   }

// WithClock 注入时钟，仅场景使用。
  func (e *Engine) WithClock(c func() time.Time) *Engine {
	   if c != nil {
		   e.clock = c
	  }
	   return e
   }

  // WithTTL 覆盖租约存活时长，仅场景使用。

   func (e *Engine) WithTTL(ttl time.Duration) *Engine {
	  if ttl > 0 {
		   e.leaseTTL = ttl
	   }
	  return e
   }

// ─────────────────────────────────────────────────────────────────
// BindLease：Record 阶段绑定租约
// ─────────────────────────────────────────────────────────────────
// BindInput 绑定租约的输入参数。
type BindInput struct {
	// PlanID 归集计划唯一标识（用于构造幂等键）。
	PlanID string
	// ExecID treasury.Exec.ID；Record 后立即传入。
	ExecID string
	// Asset 资产符号（大写）。
	Asset string
	// Amount 归集金额字符串。
	Amount string
}

// BindResult 绑定结果。

type BindResult struct {
	Lease  *PlanExecLease
	Reused bool // true 表示复用了现有未过期租约（幂等场景）
}

// BindLease 将 SweepPlan 与热库存快照绑定为执行租约。
// 功能：
// 1. 按 IdemKey 查找活跃租约；未过期则直接复用（幂等）。
// 2. 无活跃租约则 Observe 当前热账快照并新建租约。
// 3. 更新 ExecGeneration，防止同 plan 双开 Exec。
// 风险点：
// 快照 Observe 失败必须拒绝绑定；不得用零快照绑定租约。
// 并发场景下 bindMu 保证幂等检查与创建的原子性。

   func (e *Engine) BindLease(ctx context.Context, st ledger.Store, in BindInput) (BindResult, error) {
	  if err := validateBindInput(in); err != nil {
		  return BindResult{}, err
	  }
	   idemKey := IdemKeyFor(in.Asset, in.PlanID)
	   // 持锁期间完成「检查是否存在 + 创建」的完整原子操作。
	   e.bindMu.Lock()
	  defer e.bindMu.Unlock()
	   now := e.clock()
	  // 幂等检查：同 plan 未过期租约直接复用。
	   existing, err := e.leases.GetLeaseByIdemKey(ctx, idemKey)
	   if err == nil {
		   if !existing.IsTerminal() && !existing.IsExpired(now) {
			  // 复用时更新 ExecID 映射（同 plan 可能产生新 ExecID）。
			   if existing.ExecID != in.ExecID && in.ExecID != "" {
				   existing.ExecID = in.ExecID
				   if serr := e.leases.SaveLease(ctx, existing); serr != nil {
					  return BindResult{}, serr
				   }
			  }
			   return BindResult{Lease: existing, Reused: true}, nil
		  }
	  }
	  // 生成世代：确保同 plan 不双开。
	   gen, genErr := e.gens.GetGen(ctx, idemKey)
	   if genErr == nil && gen.ActiveLeaseID != "" {
		  // 有记录中的活跃租约 ID，须验证其是否真正过期或终态。
		   activeLease, alerr := e.leases.GetLease(ctx, gen.ActiveLeaseID)
		   if alerr == nil && !activeLease.IsTerminal() && !activeLease.IsExpired(now) {
			  return BindResult{}, fmt.Errorf("%w: active lease %s still valid", ErrDuplicateExec, gen.ActiveLeaseID)
		  }
	  }
	   // 取当前热库存快照；st 为 nil 时（ mock）须由 inv 自行处理。
	  snap, snapErr := e.observeAsset(ctx, st, in.Asset)
	   if snapErr != nil {
		   return BindResult{}, snapErr
	   }
	  // 取 Band 信息。
	   band, bandErr := e.cat.Get(in.Asset)
	   if bandErr != nil {
		  return BindResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, bandErr)
	  }
	   leaseID := uuid.NewString()
	  digest := NewSnapshotDigest(snap, band, now)
	   lease := &PlanExecLease{
		   LeaseID: leaseID,
		  IdemKey: idemKey,
		   ExecID:  in.ExecID,
		  Asset: strings.ToUpper(strings.TrimSpace(in.Asset)),
		   Amount:         in.Amount,
		   SnapshotDigest: digest,
		  State:     LeaseBound,
		  TTL:       e.leaseTTL,
		  BoundAt:   now,
		   ExpiresAt: now.Add(e.leaseTTL),
	  }
	   if err := e.leases.SaveLease(ctx, lease); err != nil {
		   return BindResult{}, err
	  }
	   // 更新世代记录。
	   newGen := &ExecGeneration{
		   GenID: idemKey,
		  IdemKey: idemKey,
		   ActiveLeaseID: leaseID,
		  UpdatedAt: now,
	   }
	   if gen != nil {
		   newGen.Generation = gen.Generation + 1
	   } else {
		   newGen.Generation = 1
	   }
	   _ = e.gens.SaveGen(ctx, newGen)
	  return BindResult{Lease: lease, Reused: false}, nil
  }

// ─────────────────────────────────────────────────────────────────
// Recheck：MarkSent 前的库存复核
// ─────────────────────────────────────────────────────────────────
// RecheckResult 复核结果。

type RecheckResult struct {
	Lease *PlanExecLease
	Proof *TreasDriftProof // 仅漂移拒绝时非空
	Passed bool
}

// Recheck 在 MarkSent 前重新 Observe 热库存，与租约快照核对漂移。
// 功能：
// 1. 校验租约存在、未过期、状态合法（bound 或 rechecked）。
// 2. 重新 Observe 当前热账快照。
// 3. 调用 CheckDrift 判断是否漂移超限。
// 4. 漂移超限则落 TreasDriftProof，将租约置 blocked，返回拒绝。
// 5. 通过则将租约置 rechecked。
// 风险点：
// Recheck 失败必须落 TreasDriftProof；不得仅在内存拒绝。

   func (e *Engine) Recheck(ctx context.Context, st ledger.Store, leaseID string) (RecheckResult, error) {
	  lease, err := e.leases.GetLease(ctx, leaseID)
	  if err != nil {
		  return RecheckResult{}, err
	  }
	  now := e.clock()
	   if lease.IsExpired(now) {
		  if terr := Transition(lease, LeaseExpired); terr == nil {
			  _ = e.leases.SaveLease(ctx, lease)
		   }
		  return RecheckResult{Lease: lease,
			  Passed: false}, ErrLeaseExpired
	  }
	  if lease.IsTerminal() {
		  return RecheckResult{Lease: lease, Passed: false}, fmt.Errorf("%w: state=%s", ErrIllegalTransition, lease.State)
	  }
	  // 重新取快照；必须重新 Observe，不得复用 BindLease 时的快照。
	   snap, snapErr := e.observeAsset(ctx, st, lease.Asset)
	  if snapErr != nil {
		   return RecheckResult{}, snapErr
	   }
	  band,
		  bandErr := e.cat.Get(lease.Asset)
	   if bandErr != nil {
		  return RecheckResult{}, fmt.Errorf("%w: band for %s: %v", ErrInvalidInput, lease.Asset, bandErr)
	   }
	  recheckDigest := NewSnapshotDigest(snap,
		  band, now)
	   dr, driftErr := CheckDrift(lease.SnapshotDigest, recheckDigest, band)
	  if driftErr != nil {
		  return RecheckResult{}, driftErr
	   }
	   // 无论是否超限，都保留证明（pass 证明供审计）。
	  proof := BuildDriftProof(leaseID, lease.ExecID, lease.Asset, dr, now)
	  _ = e.proofs.SaveProof(ctx, proof)
	  if dr.Exceeded {
		   // 漂移超限：封堵租约。
		  t := now
		  lease.BlockedAt = &t
		   lease.BlockReason = dr.Reason
		  if terr := Transition(lease, LeaseBlocked); terr != nil {
			   return RecheckResult{Proof: proof},
				   terr
		  }
		  _ = e.leases.SaveLease(ctx, lease)
		  return RecheckResult{Lease: lease, Proof: proof, Passed: false}, ErrDriftExceeded
	   }
	  // 通过复核。
	  t := now
	   lease.RecheckAt = &t
	   if terr := Transition(lease, LeaseRechecked); terr != nil {
		   return RecheckResult{Proof: proof}, terr
	   }
	   _ = e.leases.SaveLease(ctx, lease)
	   return RecheckResult{Lease: lease, Proof: proof, Passed: true}, nil
   }

// ─────────────────────────────────────────────────────────────────
// MarkSentGate：MarkSent 闸
// ─────────────────────────────────────────────────────────────────
// MarkSentGateInput MarkSent 闸入参。
type MarkSentGateInput struct {
	LeaseID string
	ExecID string
}

// MarkSentGate 放行 MarkSent；严格校验租约有效性后更新状态。
// 功能：
// 1. 按 LeaseID 取租约。
// 2. 校验 ExecID 与租约绑定一致。
// 3. 校验租约未过期、状态为 rechecked（必须已经过 Recheck）。
// 4. 将租约置 sent，更新 SentAt。
// 风险点：
// 未经 Recheck 的租约（state=bound）不得放行；防止绕过复核直接 MarkSent。
// ExecID 不符须返回 ErrLeaseIDMismatch，禁止静默放行。

  func (e *Engine) MarkSentGate(ctx context.Context, in MarkSentGateInput) (*PlanExecLease, error) {
	  if strings.TrimSpace(in.LeaseID) == "" || strings.TrimSpace(in.ExecID) == "" {
		  return nil, fmt.Errorf("%w: lease_id and exec_id required", ErrInvalidInput)
	   }
	  lease,
		  err := e.leases.GetLease(ctx, in.LeaseID)
	   if err != nil {
		  return nil, err
	  }
	   now := e.clock()
	  if lease.IsExpired(now) {
		  if terr := Transition(lease, LeaseExpired); terr == nil {
			   _ = e.leases.SaveLease(ctx, lease)
		  }
		  return lease, ErrLeaseExpired
	  }
	   if lease.ExecID != strings.TrimSpace(in.ExecID) {
		  return lease, fmt.Errorf("%w: lease bound to %s, got %s", ErrLeaseIDMismatch, lease.ExecID, in.ExecID)
	   }
	  if lease.State != LeaseRechecked {
		   return lease, fmt.Errorf("%w: expected rechecked, got %s", ErrIllegalTransition, lease.State)
	  }
	  t := now
	  lease.SentAt = &t
	  if terr := Transition(lease, LeaseSent); terr != nil {
		  return lease, terr
	   }
	   if serr := e.leases.SaveLease(ctx, lease); serr != nil {
		   return lease, serr
	  }
	  return lease, nil
  }

// ─────────────────────────────────────────────────────────────────
// AckLease / FailLease：终态绑定
// ─────────────────────────────────────────────────────────────────
// AckLease 链上回执确认后将租约置 acked 终态。
// 功能：Acked 是最终成功态；必须携带同一 LeaseID。
// 风险点：过期或非 sent 状态不得 Acked。

  func (e *Engine) AckLease(ctx context.Context, leaseID, execID string) (*PlanExecLease, error) {
	   return e.terminal(ctx, leaseID, execID, LeaseAcked)
   }

// FailLease 执行失败时将租约置 blocked 终态（用于非漂移场景的失败封堵）。

   func (e *Engine) FailLease(ctx context.Context, leaseID, execID, reason string) (*PlanExecLease, error) {
	  lease, err := e.terminal(ctx, leaseID, execID, LeaseBlocked)
	   if err != nil {
		   return lease, err
	  }
	  if reason != "" {
		   now := e.clock()
		  t := now
		  lease.BlockedAt = &t
		  lease.BlockReason = reason
		   _ = e.leases.SaveLease(ctx, lease)
	   }
	   return lease, nil
  }

// terminal 公共终态跃迁逻辑。

   func (e *Engine) terminal(ctx context.Context, leaseID, execID string, to LeaseState) (*PlanExecLease, error) {
	   if strings.TrimSpace(leaseID) == "" {
		   return nil, fmt.Errorf("%w: lease_id required", ErrInvalidInput)
	  }
	   lease, err := e.leases.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	  }
	   now := e.clock()
	   if lease.IsExpired(now) && !lease.IsTerminal() {
		   if terr := Transition(lease, LeaseExpired); terr == nil {
			   _ = e.leases.SaveLease(ctx, lease)
		   }
		  return lease, ErrLeaseExpired
	  }
	   if strings.TrimSpace(execID) != "" && lease.ExecID != strings.TrimSpace(execID) {
		   return lease, fmt.Errorf("%w: lease bound to %s, got %s", ErrLeaseIDMismatch, lease.ExecID, execID)
	   }
	  if lease.IsTerminal() {
		  // 幂等：已在目标终态则直接返回。
		   if lease.State == to {
			  return lease, nil
		  }
		  // 已过期终态须返回 ErrLeaseExpired，方便调用方区分是过期还是其他失败。
		  if lease.State == LeaseExpired {
			   return lease, ErrLeaseExpired
		  }
		   return lease, fmt.Errorf("%w: already in terminal state %s", ErrIllegalTransition, lease.State)
	  }
	  switch to {
	   case LeaseAcked:
		  t := now
		  lease.AckedAt = &t
	  case LeaseBlocked:
		   t := now
		  lease.BlockedAt = &t
	   }
	   if terr := Transition(lease, to); terr != nil {
		  return lease, terr
	  }
	  if serr := e.leases.SaveLease(ctx, lease); serr != nil {
		  return lease, serr
	  }
	   return lease, nil
  }

// ─────────────────────────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────────────────────────
// GetLease 按 LeaseID 查询租约（供他包只读使用）。
  func (e *Engine) GetLease(ctx context.Context, leaseID string) (*PlanExecLease, error) {
	   return e.leases.GetLease(ctx,
		  leaseID)
   }

// GetProofsByLease 查询租约关联的所有漂移证明（供他包只读使用）。
  func (e *Engine) GetProofsByLease(ctx context.Context, leaseID string) ([]*TreasDriftProof, error) {
	   return e.proofs.ListProofsByLease(ctx, leaseID)
   }

// ExpireStaleLeases 将过期的非终态租约标记为 expired。
// 功能：由 worker 定期调用；批量清理过期租约，防止租约表无限膨胀。
  func (e *Engine) ExpireStaleLeases(ctx context.Context) (int, error) {
	   now := e.clock()
	   stale, err := e.leases.ListExpiredLeases(ctx, now, 100)
	  if err != nil {
		  return 0, err
	  }
	   count := 0
	   for _, l := range stale {
		   if l.IsTerminal() {
			   continue
		   }
		  if terr := Transition(l, LeaseExpired); terr != nil {
			  continue
		  }
		  if serr := e.leases.SaveLease(ctx, l); serr == nil {
			  count++
		  }
	  }
	   return count, nil
   }

// observeAsset 取指定资产的当前热库存快照，封装接口调用与资产过滤。

  func (e *Engine) observeAsset(ctx context.Context, st ledger.Store, asset string) (hotinv.Snapshot, error) {
	   if e.inv == nil {
		   return hotinv.Snapshot{}, ErrSnapshotUnavailable
	   }
	  snaps, err := e.inv.Observe(ctx, st)
	   if err != nil {
		  return hotinv.Snapshot{}, fmt.Errorf("%w: %v", ErrSnapshotUnavailable, err)
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   for _, s := range snaps {
		   if strings.ToUpper(strings.TrimSpace(s.Asset)) == asset {
			   return s, nil
		   }
	  }
	   return hotinv.Snapshot{}, fmt.Errorf("%w: asset %s not in snapshot", ErrSnapshotUnavailable, asset)
   }

  func validateBindInput(in BindInput) error {
	  if strings.TrimSpace(in.PlanID) == "" {
		  return fmt.Errorf("%w: plan_id required", ErrInvalidInput)
	   }
	  if strings.TrimSpace(in.Asset) == "" {
		   return fmt.Errorf("%w: asset required", ErrInvalidInput)
	   }
	  return nil
   }
