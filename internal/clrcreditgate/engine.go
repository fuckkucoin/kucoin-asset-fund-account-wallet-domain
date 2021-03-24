// CloseBatchGuard（关批守卫）、VoidLease（显式废弃租约）、ExportProofs（证明导出）。
// - Engine 通过 ClearingService 接口访问 clearing 包的 Release / ApplyReleased 能力；
// 不直接操作 clearing.Book 内部结构。
// - ReleaseWithLease：先调用 ClearingService.Release，成功后写 ReleaseCreditLease（issued）；
// 若 Lease 写失败则不回滚 clearing Release（Release 幂等，可重试）。
// - ApplyWithLease：
// 1. GetLease 校验 State/TTL/Fence；
// 2. GetOKProofByItem 检查是否已有 credit_ok 证明（防双贷，先行 fail-closed）；
// 3. CAS ConsumeLease（Store 层原子）；
// 4. 调用 ClearingService.Credit（portfolio.BookLedger）；
// 5. Credit 失败时调用 Store.MarkLeaseVoid + 落 ProofCreditFail；
// 6. Credit 成功时落 ProofCreditOK。
// - CloseBatchGuard：查询 BatchDrainState.UnconsumedLeases；> 0 时拒绝或切换 draining。
// - Engine 不持有任何业务锁；并发安全由 Store.ConsumeLease CAS 与 Store.SaveProof 幂等保证。
// 不变量：
// - 同一 ItemID 至多一条 credit_ok 证明；双 Apply 第二次在步骤 2 或步骤 3 被拦截。
// - Credit 失败后 applied 标记由 ClearingService.RollbackApplied 回滚（或同一调用链保证）。
// - Lease 进入终态（consumed/expired/void）后不可逆转。
// - ExportProofs 幂等：多次调用返回相同结果。

// Author: Draven
// Date: 2021.03.21

package clrcreditgate

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// ClearingService 接口
// ──────────────────────────────────────────────────────────────────────────────
// ClearingService clearing 包能力接口（只读契约，Engine 不直接访问 clearing.Book）。

type ClearingService interface {
	// Release 将 Item 状态从 ready 迁移到 released，返回更新后的 ItemView。
	// 幂等：已 released 时返回当前 Item 而不报错（调用方判断 State）。
	Release(ctx context.Context, itemID, actor string, epoch int64, token string) (ItemView, error)
	// Credit 向指定用户的资金账户贷记指定资产和金额。
	// 失败时返回非 nil error；调用方负责回滚 applied 标记。
	Credit(ctx context.Context, userID, asset, amount string) (map[string]string, error)
	// RollbackApplied 回滚 clearing.Book 中 itemID 的 applied 标记（Credit 失败后调用）。
	// 不存在时静默成功（幂等）。
	RollbackApplied(ctx context.Context, itemID string) error
	// GetItem 查询 Item 当前状态（供 CloseBatchGuard 读 released 计数）。
	GetItem(ctx context.Context, itemID string) (ItemView, error)
	// GetBatchItems 查询指定 batchID 的所有 Item（供 DrainState 计数刷新）。
	GetBatchItems(ctx context.Context,
		batchID string) ([]ItemView, error)
}

// ItemView clearing.Item 的只读视图（避免导入 clearing 包产生循环依赖）。
type ItemView struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
	UserID  string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Status string `json:"status"` // "pending"|"ready"|"released"|"excepted"|"closed"
	Epoch  int64  `json:"epoch"`
	Token  string `json:"token"`
}

// ──────────────────────────────────────────────────────────────────────────────
// EngineOption
// ──────────────────────────────────────────────────────────────────────────────
// EngineOption Engine 配置函数。
type EngineOption func(*Engine)

// WithEngineClock 注入时钟。

   func WithEngineClock(fn func() time.Time) EngineOption {
	   return func(e *Engine) {
		  if fn != nil {
			   e.clock = fn
		  }
	   }
}

// WithLeaseConfig 自定义租约配置。

  func WithLeaseConfig(cfg LeaseConfig) EngineOption {
	  return func(e *Engine) {
		  cfg.Fill()
		  e.leaseCfg = cfg
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// Engine
// ──────────────────────────────────────────────────────────────────────────────
// Engine clrcreditgate 核心引擎。
type Engine struct {
	store Store
	clearing ClearingService
	clock    func() time.Time
	leaseCfg LeaseConfig
	// inFlight 在途 ApplyWithLease 并发计数（原子），供 CloseBatchGuard 读取。
	inFlight int64
}

// NewEngine 构造 Engine。
  func NewEngine(store Store, svc ClearingService, opts ...EngineOption) *Engine {
	  if store == nil {
		  store = NewMemStore()
	   }
	  cfg := LeaseConfig{}
	   cfg.Fill()
	   e := &Engine{
		  store:    store,
		  clearing: svc,
		  clock:    func() time.Time { return time.Now().UTC() },
		   leaseCfg: cfg,
	   }
	  for _, opt := range opts {
		  opt(e)
	  }
	   return e
  }

// ──────────────────────────────────────────────────────────────────────────────
// ReleaseWithLease
// ──────────────────────────────────────────────────────────────────────────────
// ReleaseWithLease 将 clearing Item 推进到 released 状态，并签发 ReleaseCreditLease。
// 流程：
// 1. 校验 actor；
// 2. 调用 ClearingService.Release；
// 3. 若 Item 已有 issued Lease（重复调用），返回现有 Lease（幂等）；
// 4. 签发新 Lease（issued），写入 Store。
// 同一 ItemID 至多一个 issued Lease；重复调用时若 Lease 仍为 issued 则直接返回，不重签。

   func (e *Engine) ReleaseWithLease(ctx context.Context, itemID, actor string, epoch int64, token string) (*ReleaseWithLeaseResult, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	  }
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		   return nil, ErrActor
	   }
	   itemID = strings.TrimSpace(itemID)
	  // 调用 clearing Release
	   var item ItemView
	   if e.clearing != nil {
		  var err error
		   item, err = e.clearing.Release(ctx, itemID, actor, epoch, token)
		  if err != nil {
			  return nil, err
		   }
	   } else {
		   // clearing 未注入时（单测模式），直接构造 released 视图
		   item = ItemView{ID: itemID, Status: "released", Epoch: epoch, Token: token}
	  }
	  // 幂等：检查是否已有 issued Lease
	   if existing, err := e.store.GetLeaseByItem(ctx, itemID); err == nil && existing.State == LeaseIssued {
		  return &ReleaseWithLeaseResult{
			   ItemID: itemID,
			  BatchID: item.BatchID,
			   Lease: existing,
		   }, nil
	   }
	  now := e.clock()
	   leaseID := uuid.NewString()
	  token2 := newToken()
	  lease := &ReleaseCreditLease{
		   LeaseID: leaseID,
		  ItemID: itemID,
		   BatchID: item.BatchID,
		  UserID:  item.UserID,
		  Asset:  item.Asset,
		   Amount: item.Amount,
		  IssuedAt:  now,
		   ExpiresAt: now.Add(e.leaseCfg.DefaultTTL),
		  State: LeaseIssued,
		   Actor:     actor,
		   Fence:     Fence{Epoch: 1, Token: token2},
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.SaveLease(ctx, lease); err != nil {
		   // ErrLeaseAlreadyIssued 时再次尝试读取（并发签发竞争）
		  if errors.Is(err, ErrLeaseAlreadyIssued) {
			  if existing, err2 := e.store.GetLeaseByItem(ctx, itemID); err2 == nil {
				   return &ReleaseWithLeaseResult{ItemID: itemID,
					   BatchID: item.BatchID, Lease: existing}, nil
			  }
		  }
		  return nil, err
	  }
	  return &ReleaseWithLeaseResult{
		   ItemID: itemID,
		   BatchID: item.BatchID,
		  Lease: lease,
	  }, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// ApplyWithLease
// ──────────────────────────────────────────────────────────────────────────────
// ApplyWithLease 携带 Lease CAS 消费后执行贷记，落 ClearingCreditProof。
// fail-closed 语义：
// - Lease 不存在、已消费、已过期、已 void、Fence 不匹配 → 拒绝，落 ProofDupApply 或 ProofExpiredLease。
// - 已存在 credit_ok 证明（先行防双贷检查）→ 拒绝，落 ProofDupApply。
// - Credit 调用失败 → 落 ProofCreditFail，MarkLeaseVoid，RollbackApplied。
// 成功路径：ConsumeLease（CAS）→ Credit → ProofCreditOK。

  func (e *Engine) ApplyWithLease(ctx context.Context, itemID, leaseID, actor string, leaseFence Fence) (*ApplyWithLeaseResult, error) {
	  if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	   }
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		  return nil, ErrActor
	  }
	   itemID = strings.TrimSpace(itemID)
	   leaseID = strings.TrimSpace(leaseID)
	  // 在途计数 +1
	   atomic.AddInt64(&e.inFlight, 1)
	   defer atomic.AddInt64(&e.inFlight, -1)
	  now := e.clock()
	   // 1. 读 Lease
	   lease, err := e.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	  }
	  // 2. Lease.ItemID 必须与请求 ItemID 一致（防跨条目消费）
	   if lease.ItemID != itemID {
		   return nil, ErrLeaseItemMismatch
	  }
	   // 3. 状态 / TTL 检查（fail-closed）
	   switch lease.State {
	   case LeaseConsumed:
		   proof := e.emitProof(ctx, lease, actor, ProofDupApply, "lease already consumed", now)
		   return &ApplyWithLeaseResult{ItemID: itemID, LeaseID: leaseID, Proof: proof}, ErrLeaseConsumed
	  case LeaseExpired:
		   proof := e.emitProof(ctx, lease, actor, ProofExpiredLease, "lease expired (state)", now)
		   return &ApplyWithLeaseResult{ItemID: itemID,
			  LeaseID: leaseID, Proof: proof}, ErrLeaseExpired
	   case LeaseVoid:
		  proof := e.emitProof(ctx, lease, actor, ProofExpiredLease, "lease voided", now)
		  return &ApplyWithLeaseResult{ItemID: itemID, LeaseID: leaseID, Proof: proof}, ErrLeaseVoid
	   }
	  if lease.Expired(now) {
		   // issued 但 TTL 已过：先标记 expired，再落 Proof
		  _ = e.store.MarkLeaseExpired(ctx, leaseID, now)
		  proof := e.emitProof(ctx, lease, actor, ProofExpiredLease, "lease ttl exceeded", now)
		  return &ApplyWithLeaseResult{ItemID: itemID, LeaseID: leaseID, Proof: proof}, ErrLeaseExpired
	  }
	   // 4. 先行防双贷：检查是否已有 credit_ok 证明
	   if _, err2 := e.store.GetOKProofByItem(ctx, itemID); err2 == nil {
		   proof := e.emitProof(ctx, lease, actor, ProofDupApply, "credit_ok proof already exists", now)
		   return &ApplyWithLeaseResult{ItemID: itemID, LeaseID: leaseID, Proof: proof}, ErrDupApply
	  }
	   // 5. CAS 消费 Lease（Store 层原子）
	   proofID := uuid.NewString()
	  if err := e.store.ConsumeLease(ctx, leaseID, leaseFence, proofID, now); err != nil {
		   switch {
		   case errors.Is(err, ErrLeaseConsumed):
			  proof := e.emitProof(ctx, lease, actor, ProofDupApply, "cas consume: already consumed", now)
			   return &ApplyWithLeaseResult{ItemID: itemID, LeaseID: leaseID, Proof: proof}, ErrDupApply
		   case errors.Is(err, ErrLeaseExpired):
			  proof := e.emitProof(ctx, lease, actor, ProofExpiredLease, "cas consume: expired", now)
			   return &ApplyWithLeaseResult{ItemID: itemID, LeaseID: leaseID, Proof: proof}, ErrLeaseExpired
		   case errors.Is(err, ErrLeaseFenceStale):
			   proof := e.emitProof(ctx, lease, actor, ProofDupApply, "cas consume: fence stale", now)
			  return &ApplyWithLeaseResult{ItemID: itemID,
				   LeaseID: leaseID, Proof: proof}, ErrLeaseFenceStale
		  }
		   return nil, err
	  }
	  // 6. 调用 ClearingService.Credit
	   var posAfter map[string]string
	   if e.clearing != nil {
		  posAfter, err = e.clearing.Credit(ctx, lease.UserID, lease.Asset, lease.Amount)
		  if err != nil {
			   // Credit 失败：MarkLeaseVoid + RollbackApplied + 落 ProofCreditFail
			  _ = e.store.MarkLeaseVoid(ctx, leaseID, "credit_fail", now)
			  if e.clearing != nil {
				  _ = e.clearing.RollbackApplied(ctx, itemID)
			   }
			   failProof := &ClearingCreditProof{
				   ProofID: proofID,
				   LeaseID: leaseID,
				  ItemID: itemID,
				  BatchID: lease.BatchID,
				  UserID:  lease.UserID,
				  Asset: lease.Asset,
				  Amount: lease.Amount,
				   Kind:   ProofCreditFail,
				  Detail: err.Error(),
				   Actor:  actor,
				  At:     now,
				  EpochAtProof: leaseFence.Epoch,
				  CreatedAt: now,
			  }
			  _ = e.store.SaveProof(ctx, failProof)
			  return &ApplyWithLeaseResult{ItemID: itemID, LeaseID: leaseID, Proof: failProof}, err
		  }
	   }
	   // 7. 落 ProofCreditOK
	   okProof := &ClearingCreditProof{
		   ProofID: proofID,
		  LeaseID: leaseID,
		   ItemID:  itemID,
		  BatchID: lease.BatchID,
		   UserID: lease.UserID,
		   Asset:  lease.Asset,
		   Amount: lease.Amount,
		  Kind:  ProofCreditOK,
		  Actor: actor,
		  At:           now,
		  EpochAtProof: leaseFence.Epoch,
		  CreatedAt: now,
	   }
	  if err := e.store.SaveProof(ctx, okProof); err != nil && !errors.Is(err, ErrAlreadyProven) {
		   // SaveProof 失败不回滚账本（证明落地失败不影响已完成的贷记；
		  // 运营可通过 Lease.ConsumedByProof 追溯）
		  return &ApplyWithLeaseResult{ItemID: itemID,
			  LeaseID: leaseID, Proof: okProof, PositionAfter: posAfter}, nil
	  }
	  return &ApplyWithLeaseResult{ItemID: itemID, LeaseID: leaseID, Proof: okProof, PositionAfter: posAfter}, nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// CloseBatchGuard
// ──────────────────────────────────────────────────────────────────────────────
// CloseBatchGuard 检查指定批次是否允许关闭。
// 逻辑：
// 1. GetOrCreateBatchDrain 获取排空状态；
// 2. 刷新 UnconsumedLeases 计数（CountUnconsumedLeases）；
// 3. 刷新 InFlightApplies（读 inFlight 原子计数，近似值）；
// 4. 若 UnconsumedLeases > 0 或 InFlightApplies > 0：
// - 若 Status=pending，切换到 draining；
// - 若 Status=draining 且超时，强制 VoidUncosmedLeases（drain 泵由 Worker 处理）；
// - 返回 Allowed=false，ErrDrainBlocked。
// 5. 否则：切换 Status=closed，返回 Allowed=true。

   func (e *Engine) CloseBatchGuard(ctx context.Context, batchID, actor string) (*CloseBatchGuardResult, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	  }
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		  return nil, ErrActor
	   }
	  batchID = strings.TrimSpace(batchID)
	  now := e.clock()
	  drain, err := e.store.GetOrCreateBatchDrain(ctx, batchID, now)
	   if err != nil {
		  return nil, err
	   }
	   if drain.Status == DrainClosed {
		  return &CloseBatchGuardResult{
			  BatchID: batchID,
			   Allowed: true,
			   DrainState: drain,
		   }, nil
	   }
	  // 刷新未消费 Lease 计数
	   unconsumed, err := e.store.CountUnconsumedLeases(ctx, batchID, now)
	   if err != nil {
		  return nil, err
	  }
	  inFlight := int(atomic.LoadInt64(&e.inFlight))
	  drain.UnconsumedLeases = unconsumed
	  drain.InFlightApplies = inFlight
	   drain.SnapshotAt = now
	   if drain.Drainable() {
		  // 允许关批：切换 closed
		  patch := &BatchDrainState{
			  UnconsumedLeases: 0,
			   InFlightApplies: 0,
			  SnapshotAt: now,
			   UpdatedAt:  now,
		   }
		  _ = e.store.UpdateBatchDrainStatus(ctx, batchID, drain.Status, DrainClosed, patch)
		   drain.Status = DrainClosed
		   return &CloseBatchGuardResult{
			   BatchID: batchID,
			   Allowed: true,
			  DrainState: drain,
		   }, nil
	   }
	   // 阻断：切换或维持 draining
	   if drain.Status == DrainPending {
		  patch := &BatchDrainState{
			  UnconsumedLeases: unconsumed,
			  InFlightApplies:  inFlight,
			  DrainStartedAt:   now,
			  DrainTimeoutAt: now.Add(e.leaseCfg.DrainTimeout),
			  SnapshotAt:     now,
			  UpdatedAt:      now,
		  }
		  _ = e.store.UpdateBatchDrainStatus(ctx, batchID, DrainPending, DrainDraining, patch)
		  drain.Status = DrainDraining
		  drain.DrainStartedAt = now
		  drain.DrainTimeoutAt = now.Add(e.leaseCfg.DrainTimeout)
	   }
	  reason := "unconsumed leases or in-flight applies"
	   return &CloseBatchGuardResult{
		   BatchID: batchID,
		  Allowed: false,
		   DrainState: drain,
		  Reason: reason,
	  }, ErrDrainBlocked
  }

// ──────────────────────────────────────────────────────────────────────────────
// VoidLease
// ──────────────────────────────────────────────────────────────────────────────
// VoidLease 显式废弃指定 Lease（ops 操作或 drain 超时强制废弃）。
// State 须为 issued；已进入终态时返回 ErrInvalidTransition。
   func (e *Engine) VoidLease(ctx context.Context, leaseID, actor, reason string) error {
	   if e == nil || e.store == nil {
		  return ErrUnavailable
	  }
	  actor = strings.TrimSpace(actor)
	  if actor == "" {
		   return ErrActor
	  }
	   now := e.clock()
	   return e.store.MarkLeaseVoid(ctx, leaseID, reason, now)
   }

// ──────────────────────────────────────────────────────────────────────────────
// ExportProofs
// ──────────────────────────────────────────────────────────────────────────────
// ExportProofs 按 BatchID 导出所有交收证明（幂等，多次调用返回相同结果）。
  func (e *Engine) ExportProofs(ctx context.Context, batchID string) ([]*ClearingCreditProof, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	  return e.store.ListProofs(ctx, ProofListFilter{BatchID: batchID})
   }

// ExportLeases 按 BatchID 导出所有 Lease（供 ops 审计）。

   func (e *Engine) ExportLeases(ctx context.Context, batchID string) ([]*ReleaseCreditLease, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	  }
	  return e.store.ListLeases(ctx,
		   LeaseListFilter{BatchID: batchID})
   }

// ──────────────────────────────────────────────────────────────────────────────
// 内部辅助
// ──────────────────────────────────────────────────────────────────────────────
// emitProof 构造并落地一条证明；落地失败时静默（证明为审计工具，不阻断主路径）。
   func (e *Engine) emitProof(ctx context.Context, lease *ReleaseCreditLease, actor string, kind ProofKind, detail string, now time.Time) *ClearingCreditProof {
	  p := &ClearingCreditProof{
		  ProofID: uuid.NewString(),
		   LeaseID: lease.LeaseID,
		   ItemID:  lease.ItemID,
		  BatchID: lease.BatchID,
		   UserID: lease.UserID,
		   Asset: lease.Asset,
		   Amount: lease.Amount,
		  Kind: kind,
		   Detail: detail,
		   Actor:  actor,
		  At:           now,
		  EpochAtProof: lease.Fence.Epoch,
		   CreatedAt: now,
	   }
	   _ = e.store.SaveProof(ctx, p)
	  return p
  }

// newToken 生成随机围栏 token（16 字节 hex）。

   func newToken() string {
	  return uuid.NewString()
  }
