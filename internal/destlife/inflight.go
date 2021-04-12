package destlife

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// InflightStore 在途引用持久化接口。

type InflightStore interface {
	// InsertRef 写入新引用（movement_id 唯一约束；重复返回 ErrDuplicateRef）。
	InsertRef(ctx context.Context, ref InflightRef) error
	// ReleaseRef 标记引用已释放（released=true, released_at=now）。
	ReleaseRef(ctx context.Context, refID string, releasedAt time.Time) error
	// GetRef 按 ref_id 取引用。
	GetRef(ctx context.Context, refID string) (InflightRef, error)
	// GetRefByMovement 按 movement_id 取引用（用于幂等校验）。
	GetRefByMovement(ctx context.Context, movementID string) (InflightRef, bool, error)
	// ActiveCount 返回条目活跃引用数（released=false）。
	ActiveCount(ctx context.Context, entryLifeID string) (int, error)
	// ListByEntry 返回条目所有引用（含已释放）。
	ListByEntry(ctx context.Context,
		entryLifeID string, limit int) ([]InflightRef, error)
	// ActiveSummary 返回条目活跃引用摘要。
	ActiveSummary(ctx context.Context, entryLifeID string) (InflightCount, error)
}

// InflightLock 在途出金冻结锁。
type InflightLock struct {
	store InflightStore
	calendar *Calendar
	clock    func() time.Time
}

// NewInflightLock 构造在途锁。calendar 用于条目状态转换（Remove 触发 revoking）。

   func NewInflightLock(store InflightStore, cal *Calendar, clock func() time.Time) *InflightLock {
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &InflightLock{store: store, calendar: cal, clock: clock}
}

// AcquireRef 出金冻结时获取在途引用。
// 幂等：同一 movement_id 重复调用返回已有 ref（不报错）。
// 前置：entryLifeID 对应条目必须处于 Active 状态（调用方应先 Calendar.CheckCooldown）。
   func (l *InflightLock) AcquireRef(ctx context.Context,
	  entryLifeID, userID, chain, addr, movementID string) (InflightRef, error) {
	   movementID = strings.TrimSpace(movementID)
	   if movementID == "" {
		  return InflightRef{}, fmt.Errorf("destlife: movement_id required for inflight ref")
	  }
	   if entryLifeID == "" {
		  return InflightRef{}, fmt.Errorf("destlife: entry_life_id required for inflight ref")
	  }
	   // 幂等检查。
	   if l.store != nil {
		  if existing, ok, err := l.store.GetRefByMovement(ctx, movementID); err == nil && ok {
			  if existing.EntryLifeID != entryLifeID {
				  return InflightRef{}, fmt.Errorf("destlife: movement %s already holds ref on different entry", movementID)
			   }
			   return existing, nil
		   }
	   }
	   now := l.clock()
	  ref := InflightRef{
		  RefID:       uuid.NewString(),
		  EntryLifeID: entryLifeID,
		   UserID:  userID,
		  Chain:   chain,
		   Address: addr,
		   MovementID: movementID,
		   AcquiredAt: now,
		   Released: false,
	   }
	   if l.store != nil {
		  if err := l.store.InsertRef(ctx, ref); err != nil {
			   return InflightRef{}, fmt.Errorf("destlife: acquire inflight ref: %w", err)
		   }
	  }
	  return ref, nil
  }

// ReleaseRef 出金完成/取消时释放在途引用。
// 释放后若条目处于 revoking 且引用计数归零，自动触发 FinalizeRevoked。
  func (l *InflightLock) ReleaseRef(ctx context.Context, refID, actor string, bookGen int64) error {
	  if refID == "" {
		  return fmt.Errorf("destlife: ref_id required")
	   }
	   now := l.clock()
	   if l.store != nil {
		   if err := l.store.ReleaseRef(ctx, refID, now); err != nil {
			  return fmt.Errorf("destlife: release inflight ref: %w", err)
		  }
	  }
	   // 检查是否需要自动完成 revoking→revoked 转换。
	   if l.store != nil && l.calendar != nil {
		   ref,
			  err := l.store.GetRef(ctx, refID)
		  if err != nil {
			  return nil
		   }
		   cnt, err := l.store.ActiveCount(ctx, ref.EntryLifeID)
		  if err != nil || cnt > 0 {
			  return nil
		   }
		   // 引用计数归零，尝试完成撤销。
		  e, err := l.calendar.GetLifeByID(ctx, ref.EntryLifeID)
		   if err != nil {
			  return nil
		   }
		   if e.State == StateRevoking {
			  _, _ = l.calendar.FinalizeRevoked(ctx, ref.EntryLifeID, actor, bookGen)
		  }
	  }
	  return nil
   }

// BlockRemoveIfInflight 检查引用计数；> 0 则拒绝 Remove，返回 ErrInflightBlock。
// 调用方（destallow.Book.RemoveExact）在调用前应先调用此方法。
  func (l *InflightLock) BlockRemoveIfInflight(ctx context.Context, entryLifeID string) error {
	   if l == nil || l.store == nil {
		  return nil
	  }
	  cnt, err := l.store.ActiveCount(ctx, entryLifeID)
	   if err != nil {
		   return fmt.Errorf("destlife: check inflight count: %w", err)
	   }
	   if cnt > 0 {
		   return fmt.Errorf("%w (active_refs=%d)", ErrInflightBlock, cnt)
	   }
	  return nil
   }

// ActiveCount 返回条目活跃引用数（公开供 API 调用）。
  func (l *InflightLock) ActiveCount(ctx context.Context, entryLifeID string) (int, error) {
	   if l == nil || l.store == nil {
		   return 0, nil
	   }
	  return l.store.ActiveCount(ctx,
		  entryLifeID)
  }

// Summary 返回条目引用摘要。
  func (l *InflightLock) Summary(ctx context.Context, entryLifeID string) (InflightCount, error) {
	  if l == nil || l.store == nil {
		  return InflightCount{EntryLifeID: entryLifeID}, nil
	  }
	  return l.store.ActiveSummary(ctx, entryLifeID)
  }

// ListByEntry 返回条目所有引用（含已释放）。

   func (l *InflightLock) ListByEntry(ctx context.Context, entryLifeID string, limit int) ([]InflightRef, error) {
	   if l == nil || l.store == nil {
		  return nil, nil
	  }
	  return l.store.ListByEntry(ctx, entryLifeID, limit)
   }

// ForceRevokingOnRemove 在 Remove 请求到来时：
// - 若无活跃引用：返回 nil（允许直接物理删除）。
// - 若有活跃引用：调用 Calendar.BeginRevoking 进入待撤销队列，返回 ErrInflightBlock。
// 调用方收到 ErrInflightBlock 不得接着执行物理删除。
  func (l *InflightLock) ForceRevokingOnRemove(ctx context.Context, entryLifeID, actor, note string, bookGen int64) error {
	  if l == nil {
		   return nil
	   }
	  cnt := 0
	   var err error
	   if l.store != nil {
		   cnt, err = l.store.ActiveCount(ctx, entryLifeID)
		   if err != nil {
			  return fmt.Errorf("destlife: check inflight before remove: %w", err)
		   }
	   }
	  if cnt == 0 {
		   return nil
	   }
	   if l.calendar == nil {
		   return ErrInflightBlock
	   }
	  if _, err := l.calendar.BeginRevoking(ctx, entryLifeID, actor, note, bookGen); err != nil {
		   return err
	  }
	   return ErrInflightBlock
  }

// ── InflightEngine（世代租约 / 漂移 / 吊销） ──

const (
	// DefaultLeaseTTL 在途世代租约默认有效期。
	DefaultLeaseTTL = 4 * time.Hour
	// DefaultRefLeaseTTL 泄漏检测租约超时基准（安全边际）。
	DefaultRefLeaseTTL = 8 * time.Hour
)

// InflightEngine 在途引用核心引擎。
// 调用方须保证：
// 1. entryID 在 destallow/白名单已存在（由 destallow 包负责校验，本包不写白名单）。
// 2. 出金广播路径调用 VerifyBroadcast 后才允许提交链上交易。
// 3. ReleaseRef 在出金完成（成功或失败）时必须调用，避免引用泄漏。
type InflightEngine struct {
	leases  GenLeaseStore
	revokes RevokeGenStore
	drifts  DriftCaseStore
	proofs  ProofStore
	clock   func() time.Time
	leaseTTL time.Duration
}

// EngineConfig 引擎配置。

type EngineConfig struct {
	LeaseTTL time.Duration
}

   func (c *EngineConfig) ttl() time.Duration {
	   if c == nil || c.LeaseTTL <= 0 {
		   return DefaultLeaseTTL
	   }
	   return c.LeaseTTL
  }

// NewInflightEngine 构造在途引用引擎。

  func NewInflightEngine(
	  leases GenLeaseStore,
	  revokes RevokeGenStore,
	  drifts DriftCaseStore,
	   proofs ProofStore,
	   cfg EngineConfig,
   ) *InflightEngine {
	  return &InflightEngine{
		  leases: leases,
		  revokes:  revokes,
		   drifts:   drifts,
		  proofs:   proofs,
		  clock:    func() time.Time { return time.Now().UTC() },
		  leaseTTL: cfg.ttl(),
	   }
  }

// WithClock 注入时钟。

  func (e *InflightEngine) WithClock(c func() time.Time) *InflightEngine {
	   if e != nil && c != nil {
		   e.clock = c
	   }
	  return e
  }

   // ─────────────────────────────────────────────────────────────────────────────
  // AcquireRef — 签发在途世代租约
   // ─────────────────────────────────────────────────────────────────────────────
  // AcquireRef 为 movement_id 签发在途世代租约。
  // 语义：
   // - 同一 (entry_id, movement_id) 重复调用幂等，返回已有租约。
   // - 跨 entry 相同 movement_id 已有活跃引用时显式返回 ErrRefConflict。
  // - 条目已 revoked 拒绝签发（返回 ErrEntryRevoked）。
  // - 条目处于 revoking 仍可签发，但广播路径随后会拒绝（BoundRevokeGen 不一致）。
   func (e *InflightEngine) AcquireRef(ctx context.Context, req AcquireRefReq) (AcquireResult, error) {
	   if req.EntryID == "" {
		  return AcquireResult{}, ErrEntryIDEmpty
	  }
	  if req.MovementID == "" {
		   return AcquireResult{}, ErrMovementIDEmpty
	   }
	   rg,
		   err := e.revokes.GetOrInit(ctx, req.EntryID, req.UserID, req.Chain, req.Address)
	   if err != nil {
		  return AcquireResult{}, fmt.Errorf("%w: %v", ErrStore, err)
	  }
	  if rg.State == EntryRevoked {
		  return AcquireResult{}, ErrEntryRevoked
	   }
	  now := e.clock()
	   lease := InflightGenLease{
		  LeaseID: uuid.NewString(),
		   EntryID: req.EntryID,
		   UserID:     req.UserID,
		  Chain:      req.Chain,
		  Address:    req.Address,
		   MovementID: req.MovementID,
		  InflightGen:    now.UnixNano(),
		   BoundRevokeGen: rg.Gen,
		   State:    RefAcquired,
		   IssuedAt: now,
		   ExpiresAt: now.Add(e.leaseTTL),
		  Actor:     req.Actor,
	   }
	  inserted, idempotent, err := e.leases.Insert(ctx, lease)
	   if err != nil {
		  return AcquireResult{}, fmt.Errorf("%w: %v", ErrStore, err)
	   }
	  if !idempotent {
		  if _, updErr := e.revokes.UpdateActiveRefs(ctx, req.EntryID, +1); updErr != nil {
			  return AcquireResult{}, fmt.Errorf("%w: update active refs: %v", ErrStore, updErr)
		  }
	  }
	   return AcquireResult{Lease: inserted, Idempotent: idempotent}, nil
   }

// AcquireRefReq AcquireRef 请求参数。
type AcquireRefReq struct {
	EntryID string
	UserID string
	Chain      string
	Address    string
	MovementID string
	Actor      string
}

// ─────────────────────────────────────────────────────────────────────────────
// BindRef — 绑定广播世代
// ─────────────────────────────────────────────────────────────────────────────
// BindRef 将 acquired 引用绑定到出金广播，记录当前吊销世代快照。
// 广播路径在调用链上提交之前须先调用 BindRef，获取 BoundRevokeGen 作为世代锁定。
// 随后调用 VerifyBroadcast 校验 BoundRevokeGen 与当前吊销世代一致性。

   func (e *InflightEngine) BindRef(ctx context.Context, leaseID string) (InflightGenLease, error) {
	  lease, err := e.leases.GetByID(ctx, leaseID)
	  if err != nil {
		   return InflightGenLease{}, err
	   }
	   if lease.State == RefReleased {
		   return InflightGenLease{}, ErrRefAlreadyReleased
	  }
	   if lease.State == RefBound {
		   return lease, nil
	  }
	  rg, err := e.revokes.Get(ctx, lease.EntryID)
	  if err != nil {
		   return InflightGenLease{}, fmt.Errorf("%w: %v", ErrStore, err)
	   }
	   now := e.clock()
	   updates := InflightGenLease{
		  BoundRevokeGen: rg.Gen,
		  BoundAt:        &now,
	   }
	  if err := e.leases.Transition(ctx, leaseID, RefAcquired, RefBound, updates); err != nil {
		   return InflightGenLease{}, err
	   }
	  lease.State = RefBound
	   lease.BoundRevokeGen = rg.Gen
	  lease.BoundAt = &now
	  return lease, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// ReleaseRef — 释放在途引用
// ─────────────────────────────────────────────────────────────────────────────
// ReleaseRef 释放在途引用（出金完成或回滚时调用）。
// 释放后引用计数递减；若条目处于 revoking 且引用计数归零，触发 CompleteRevoke。
  func (e *InflightEngine) ReleaseRef(ctx context.Context, leaseID, actor string) error {
	  lease, err := e.leases.GetByID(ctx, leaseID)
	  if err != nil {
		   return err
	   }
	   if lease.State == RefReleased {
		   return nil
	  }
	   now := e.clock()
	   updates := InflightGenLease{ReleasedAt: &now, Note: "released by " + actor}
	  fromState := lease.State
	   if err := e.leases.Transition(ctx, leaseID, fromState, RefReleased, updates); err != nil {
		  return err
	  }
	   rg, err := e.revokes.UpdateActiveRefs(ctx, lease.EntryID, -1)
	  if err != nil {
		  return fmt.Errorf("%w: update active refs: %v", ErrStore, err)
	  }
	   if rg.State == EntryRevoking && rg.ActiveRefs == 0 {
		  if _, cErr := e.revokes.SetRevoked(ctx, lease.EntryID, actor); cErr != nil {
			   return fmt.Errorf("%w: complete revoke: %v", ErrStore, cErr)
		  }
	   }
	   return nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// BeginRevoking — 发起吊销（延迟版，存在活跃引用）
// ─────────────────────────────────────────────────────────────────────────────
// BeginRevoking 存在活跃引用时发起延迟吊销，抬升吊销世代。
// 语义：
// - 若无活跃引用（count==0），直接进入 ForceRevoke（active→revoked）。
// - 若有活跃引用，进入 revoking，抬升 Gen，广播路径收到 BoundRevokeGen 不一致后拒绝。
// - 返回 RevokeResult.Deferred=true 表示吊销已进入延迟等待引用释放。
// - 若条目从未初始化，使用提供的 UserID/Chain/Address 初始化再吊销（ForceRevoke）。
  func (e *InflightEngine) BeginRevoking(ctx context.Context, req RevokeReq) (RevokeResult, error) {
	  if req.EntryID == "" {
		  return RevokeResult{}, ErrEntryIDEmpty
	  }
	  rg, err := e.revokes.GetOrInit(ctx, req.EntryID, req.UserID, req.Chain, req.Address)
	  if err != nil {
		   return RevokeResult{}, fmt.Errorf("%w: %v", ErrStore, err)
	  }
	  if rg.State == EntryRevoked {
		   return RevokeResult{EntryID: req.EntryID, Gen: rg.Gen, State: EntryRevoked}, nil
	  }
	   count, err := e.leases.CountActiveByEntry(ctx, req.EntryID)
	   if err != nil {
		  return RevokeResult{}, fmt.Errorf("%w: count active: %v", ErrStore, err)
	   }
	   if count == 0 {
		   updated, bumpErr := e.revokes.BumpGen(ctx, req.EntryID, rg.Gen, EntryRevoking, req.Actor)
		   if bumpErr != nil {
			   return RevokeResult{}, fmt.Errorf("%w: bump gen: %v", ErrStore, bumpErr)
		   }
		   if _, rErr := e.revokes.SetRevoked(ctx, req.EntryID, req.Actor); rErr != nil {
			  return RevokeResult{}, fmt.Errorf("%w: set revoked: %v", ErrStore, rErr)
		  }
		   return RevokeResult{EntryID: req.EntryID, Gen: updated.Gen, State: EntryRevoked, Deferred: false}, nil
	   }
	   updated, err := e.revokes.BumpGen(ctx, req.EntryID, rg.Gen, EntryRevoking, req.Actor)
	  if err != nil {
		  return RevokeResult{}, fmt.Errorf("%w: bump gen: %v", ErrStore, err)
	   }
	  return RevokeResult{
		   EntryID: req.EntryID,
		   Gen:        updated.Gen,
		  State:      EntryRevoking,
		  ActiveRefs: count,
		   Deferred:   true,
	  }, nil
  }

// RevokeReq 吊销请求参数。
type RevokeReq struct {
	EntryID string
	UserID  string
	Chain   string
	Address string
	Actor   string
}

// ─────────────────────────────────────────────────────────────────────────────
// VerifyBroadcast — 广播路径世代验证
// ─────────────────────────────────────────────────────────────────────────────
// VerifyBroadcast 广播路径须调用此方法验证在途世代有效性。
// 验证逻辑：
// 1. 检查 lease 是否存在且未释放（ErrStaleGen 若 Released）。
// 2. 检查 entry 是否存在未关闭漂移案例（ErrDriftBlocked）。
// 3. 检查 entry.State 非 revoked（产出 DriftCase 并返回 ErrEntryRevoked）。
// 4. 检查 lease.BoundRevokeGen == current RevokeGen（不一致则返回 ErrStaleGen 并产出 DriftCase）。
// 5. 所有校验通过后写入 InflightProof（证明缺失不得放行）。
  func (e *InflightEngine) VerifyBroadcast(ctx context.Context, req BroadcastVerifyReq) (BroadcastVerifyResult, error) {
	  if req.LeaseID == "" || req.MovementID == "" {
		   return BroadcastVerifyResult{},
			   ErrMovementIDEmpty
	  }
	   lease, err := e.leases.GetByID(ctx, req.LeaseID)
	  if err != nil {
		  return BroadcastVerifyResult{}, err
	  }
	   if lease.State == RefReleased {
		  _ = e.openDriftCase(ctx,
			   lease, 0, DriftBroadcastStaleGen,
			  fmt.Sprintf("lease released; movement=%s", req.MovementID))
		   return BroadcastVerifyResult{
			  OK: false,
			   LeaseID:      req.LeaseID,
			  EntryID:      lease.EntryID,
			  InflightGen:  lease.InflightGen,
			   RejectReason: "lease released",
		  }, ErrStaleGen
	  }
	  if _, dcErr := e.drifts.GetOpenByEntry(ctx, lease.EntryID); dcErr == nil {
		  return BroadcastVerifyResult{
			   OK: false,
			  LeaseID: req.LeaseID,
			  EntryID: lease.EntryID,
			  InflightGen:  lease.InflightGen,
			   RejectReason: "open drift case blocks broadcast",
		  }, ErrDriftBlocked
	  }
	  rg, err := e.revokes.Get(ctx, lease.EntryID)
	   if err != nil {
		  return BroadcastVerifyResult{}, fmt.Errorf("%w: %v", ErrStore, err)
	  }
	  if rg.State == EntryRevoked {
		   _ = e.openDriftCase(ctx, lease, rg.Gen, DriftBroadcastAfterRevoke,
			   fmt.Sprintf("entry revoked; movement=%s", req.MovementID))
		   return BroadcastVerifyResult{
			   OK:      false,
			  LeaseID: req.LeaseID,
			   EntryID:     lease.EntryID,
			  InflightGen: lease.InflightGen,
			  BoundRevokeGen:   lease.BoundRevokeGen,
			   CurrentRevokeGen: rg.Gen,
			   EntryState:       rg.State,
			   RejectReason: "entry revoked",
		   }, ErrEntryRevoked
	   }
	   if rg.Gen != lease.BoundRevokeGen {
		  _ = e.openDriftCase(ctx, lease, rg.Gen, DriftBroadcastStaleGen,
			   fmt.Sprintf("revoke_gen mismatch: bound=%d current=%d movement=%s",
				  lease.BoundRevokeGen, rg.Gen, req.MovementID))
		  return BroadcastVerifyResult{
			   OK:      false,
			  LeaseID: req.LeaseID,
			  EntryID:        lease.EntryID,
			  InflightGen:    lease.InflightGen,
			   BoundRevokeGen: lease.BoundRevokeGen,
			   CurrentRevokeGen: rg.Gen,
			   EntryState:   rg.State,
			  RejectReason: fmt.Sprintf("stale revoke_gen: bound=%d current=%d", lease.BoundRevokeGen, rg.Gen),
		   }, ErrStaleGen
	  }
	   proof := InflightProof{
		   ProofID:    uuid.NewString(),
		  LeaseID:    req.LeaseID,
		   EntryID:    lease.EntryID,
		  MovementID: req.MovementID,
		   InflightGen: lease.InflightGen,
		   BoundRevokeGen: lease.BoundRevokeGen,
		   BroadcastTxID: req.BroadcastTxID,
		   Chain:         lease.Chain,
		   Address:       lease.Address,
		   CreatedAt:     e.clock(),
	  }
	  if _, _, pErr := e.proofs.Insert(ctx, proof); pErr != nil {
		   return BroadcastVerifyResult{}, fmt.Errorf("%w: insert proof: %v", ErrStore, pErr)
	  }
	  return BroadcastVerifyResult{
		  OK: true,
		   LeaseID:     req.LeaseID,
		  EntryID:     lease.EntryID,
		  InflightGen: lease.InflightGen,
		   BoundRevokeGen:   lease.BoundRevokeGen,
		  CurrentRevokeGen: rg.Gen,
		   EntryState:       rg.State,
	   }, nil
   }

// BroadcastVerifyReq 广播验证请求参数。
type BroadcastVerifyReq struct {
	LeaseID string
	MovementID    string
	BroadcastTxID string
}

// openDriftCase 内部辅助：产出漂移案例。不阻断调用链（错误静默记录）。
   func (e *InflightEngine) openDriftCase(ctx context.Context, lease InflightGenLease, currentGen int64, kind DriftKind, note string) error {
	   dc := InflightDriftCase{
		   CaseID: uuid.NewString(),
		   EntryID: lease.EntryID,
		  UserID:  lease.UserID,
		  Chain: lease.Chain,
		  Address:    lease.Address,
		   MovementID: lease.MovementID,
		   LeaseID:    lease.LeaseID,
		   InflightGen: lease.InflightGen,
		  BoundRevokeGen:   lease.BoundRevokeGen,
		   CurrentRevokeGen: currentGen,
		  EntryState:       EntryRevoked,
		   Kind: kind,
		   State:    DriftCaseOpen,
		   OpenedAt: e.clock(),
		   Note:     note,
	   }
	  _, err := e.drifts.Open(ctx, dc)
	   return err
   }

// ─────────────────────────────────────────────────────────────────────────────
// 只读查询方法
// ─────────────────────────────────────────────────────────────────────────────
// GetRef 按 lease_id 只读查询在途引用。

   func (e *InflightEngine) GetRef(ctx context.Context, leaseID string) (InflightGenLease, error) {
	  return e.leases.GetByID(ctx, leaseID)
  }

// GetRefByMovement 按 (entry_id, movement_id) 只读查询在途引用。

  func (e *InflightEngine) GetRefByMovement(ctx context.Context, entryID, movementID string) (InflightGenLease, error) {
	  return e.leases.GetByMovement(ctx, entryID, movementID)
  }

// ListActiveRefs 列出 entry_id 下所有活跃引用。

   func (e *InflightEngine) ListActiveRefs(ctx context.Context, entryID string) ([]InflightGenLease, error) {
	   return e.leases.ListActiveByEntry(ctx, entryID)
   }

// GetRevokeGeneration 只读查询条目吊销世代。
  func (e *InflightEngine) GetRevokeGeneration(ctx context.Context, entryID string) (RevokeGeneration, error) {
	   return e.revokes.Get(ctx, entryID)
   }

// GetProof 按 (lease_id, movement_id) 只读查询在途证明。
  func (e *InflightEngine) GetProof(ctx context.Context, leaseID, movementID string) (InflightProof, error) {
	  return e.proofs.GetByLease(ctx, leaseID, movementID)
  }

// GetOpenDriftCase 按 entry_id 只读查询未关闭漂移案例。
   func (e *InflightEngine) GetOpenDriftCase(ctx context.Context, entryID string) (InflightDriftCase, error) {
	  return e.drifts.GetOpenByEntry(ctx, entryID)
  }

// CloseDriftCase 四眼收口漂移案例。
   func (e *InflightEngine) CloseDriftCase(ctx context.Context, caseID, closedBy, resolution string) (InflightDriftCase, error) {
	  return e.drifts.Close(ctx, caseID, closedBy, resolution)
   }

// ListOpenDriftCases 列出所有未关闭漂移案例（供巡检 worker）。
  func (e *InflightEngine) ListOpenDriftCases(ctx context.Context, limit int) ([]InflightDriftCase, error) {
	  return e.drifts.ListOpen(ctx, limit)
   }

// ListRevokingEntries 列出所有处于 revoking 状态的条目（供完成吊销 worker）。
  func (e *InflightEngine) ListRevokingEntries(ctx context.Context, limit int) ([]RevokeGeneration, error) {
	   return e.revokes.ListRevoking(ctx, limit)
   }

// ListExpiredLeases 列出超时未释放的泄漏引用（供泄漏扫描 worker）。

  func (e *InflightEngine) ListExpiredLeases(ctx context.Context, limit int) ([]InflightGenLease, error) {
	  return e.leases.ListExpired(ctx, e.clock(), limit)
   }
