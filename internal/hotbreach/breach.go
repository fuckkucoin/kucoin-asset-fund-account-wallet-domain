package hotbreach

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────
// BreachDetector
// ─────────────────────────────────────────────
// BreachDetector 读热库存快照，判断是否超 cap 击穿。

type BreachDetector struct {
	store *Store
	hot   *hotinv.Book
	ledSt ledger.Store
	clock func() time.Time
}

// NewBreachDetector 构造击穿检测器。

   func NewBreachDetector(store *Store, hot *hotinv.Book, ledSt ledger.Store) *BreachDetector {
	   return &BreachDetector{
		   store: store,
		   hot:   hot,
		  ledSt: ledSt,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。

   func (d *BreachDetector) WithClock(c func() time.Time) *BreachDetector {
	   if d != nil && c != nil {
		   d.clock = c
	   }
	   return d
   }

   // DetectResult 单次检测结果。
   type DetectResult struct {
	   // Asset 被检测资产。
	  Asset string `json:"asset"`
	   // Breached 是否超 cap。
	  Breached bool `json:"breached"`
	  // BreachID 本次触发的 BreachCase ID（新建）；已有活跃击穿时为现有 ID。
	  BreachID string `json:"breach_id,omitempty"`
	   // Snapshot 当时热库存快照。
	  Snapshot *hotinv.Snapshot `json:"snapshot,omitempty"`
   }

  // Detect 对指定 asset 检测是否击穿。
  // leaseID 为关联租约 ID（可为空，空时只做检测不创建关联）。
   // concurrentParty 为并发触发方标识（出金 txn_id / 归集 sweep_id）。

  func (d *BreachDetector) Detect(ctx context.Context, asset, leaseID, concurrentParty string) (DetectResult, error) {
	   if d == nil {
		   return DetectResult{}, fmt.Errorf("hotbreach: breach detector not initialized")
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		  return DetectResult{}, fmt.Errorf("%w: asset required", ErrInvalidInput)
	   }
	  // 先检查是否已有活跃击穿
	   has, existingID, err := d.store.HasActiveBreachForAsset(ctx, asset)
	   if err != nil {
		  return DetectResult{}, err
	  }
	  if has {
		   return DetectResult{Asset: asset, Breached: true, BreachID: existingID}, nil
	   }
	  // 读热库存
	  if d.hot == nil || d.ledSt == nil {
		  return DetectResult{Asset: asset, Breached: false}, nil
	  }
	   snaps, err := d.hot.Observe(ctx, d.ledSt)
	   if err != nil {
		  return DetectResult{}, fmt.Errorf("hotbreach: observe hot inventory: %w", err)
	   }
	  for _, sn := range snaps {
		   if strings.ToUpper(sn.Asset) != asset {
			   continue
		  }
		  result := DetectResult{Asset: asset, Snapshot: &sn}
		   if !sn.Over {
			  return result, nil
		  }
		  // 超 cap → 触发击穿
		  result.Breached = true
		  now := d.clock()
		   cpHash := hashParty(concurrentParty)
		  bc := BreachCase{
			  ID:          uuid.NewString(),
			  Asset:       asset,
			  LeaseID:     leaseID,
			   PreCheckHot: "",
			  BreachHot: sn.HotAvail,
			  CapValue: sn.HotMax,
			   Excess:   sn.Excess,
			   ConcurrentParty:     concurrentParty,
			  ConcurrentPartyHash: cpHash,
			   State: BreachOpen,
			   DetectedAt: now,
		   }
		   // 读租约快照
		   if leaseID != "" {
			  if l, lerr := d.store.GetLease(ctx, leaseID); lerr == nil {
				   bc.PreCheckHot = l.SnapshotHot
			   }
		  }
		   if serr := d.store.SaveBreach(ctx, bc); serr != nil {
			  return DetectResult{}, fmt.Errorf("hotbreach: save breach: %w", serr)
		   }
		   result.BreachID = bc.ID
		   // 生成证明
		   if leaseID != "" {
			  if lease, lerr := d.store.GetLease(ctx, leaseID); lerr == nil {
				   if proof, perr := buildProof(bc, lease, sn, now); perr == nil {
					  _ = d.store.SaveProof(ctx, proof)
				  }
			  }
		   }
		  // 激活 DrainFence HardBlock
		  d.activateHardBlock(ctx, asset, bc.ID, now)
		   return result, nil
	  }
	   return DetectResult{Asset: asset,
		  Breached: false}, nil
   }

// activateHardBlock 将 asset 的活跃 DrainFence 置为 HardBlock。

  func (d *BreachDetector) activateHardBlock(ctx context.Context, asset, breachID string, now time.Time) {
	  df, ok, err := d.store.ActiveDrainFenceForAsset(ctx, asset)
	   if err != nil || !ok {
		  return
	   }
	   df.HardBlock = true
	   df.BreachID = breachID
	   _ = d.store.UpdateDrainFence(ctx, df)
	   _ = now
  }

// ─────────────────────────────────────────────
// DrainGuard：挡写互斥守卫
// ─────────────────────────────────────────────
// DrainGuard 对 Sweep/出金预扣调用点实施互斥检查。
// 调用方（出金服务、归集服务）在实际预扣前调用 Guard：
// - 无租约模式（leaseID 为空）：检查 asset 是否有 HardBlock 击穿。
// - 有租约模式（leaseID 非空）：额外检查租约合法性。

type DrainGuard struct {
	store *Store
	hot   *hotinv.Book
	ledSt ledger.Store
	clock func() time.Time
}

// NewDrainGuard 构造挡写守卫。

  func NewDrainGuard(store *Store, hot *hotinv.Book, ledSt ledger.Store) *DrainGuard {
	  return &DrainGuard{
		  store: store,
		   hot:   hot,
		  ledSt: ledSt,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

  func (g *DrainGuard) WithClock(c func() time.Time) *DrainGuard {
	   if g != nil && c != nil {
		  g.clock = c
	  }
	   return g
   }

   // GuardReq 挡写检查请求。
  type GuardReq struct {
	  // Asset 资产代码。
	   Asset string `json:"asset"`
	  // LeaseID 预检租约 ID（可为空，空表示无租约出金）。
	   LeaseID string `json:"lease_id,omitempty"`
	   // Party 本次出金/归集标识（用于击穿触发时记录并发方）。
	   Party string `json:"party"`
   }

  // Guard 执行互斥检查。返回 nil 表示允许，否则为拒绝原因。

   func (g *DrainGuard) Guard(ctx context.Context, req GuardReq) error {
	   if g == nil || g.store == nil {
		  return fmt.Errorf("hotbreach: drain guard not initialized")
	   }
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   if asset == "" {
		  return fmt.Errorf("%w: asset required", ErrInvalidInput)
	  }
	  // 1. 检查击穿挡写
	  has, breachID, err := g.store.HasActiveBreachForAsset(ctx, asset)
	   if err != nil {
		  return fmt.Errorf("hotbreach: guard check breach: %w", err)
	   }
	   if has {
		   return fmt.Errorf("%w: breach %s", ErrBreachGated, breachID)
	  }
	   // 2. 检查 DrainFence 活跃状态
	   df, hasFence, err := g.store.ActiveDrainFenceForAsset(ctx, asset)
	  if err != nil {
		   return fmt.Errorf("hotbreach: guard check drain fence: %w", err)
	   }
	   if hasFence {
		  if df.HardBlock {
			  return fmt.Errorf("%w: hard block fence %s", ErrFenceConflict, df.ID)
		   }
		   // 软挡写：检查租约合法性
		  if req.LeaseID == "" {
			  // 无租约但有活跃 fence → 拒绝（防止并发绕过）
			   return fmt.Errorf("%w: concurrent debit without lease while fence %s active", ErrNoLease, df.ID)
		   }
		   if req.LeaseID != df.LeaseID {
			   // 租约 ID 与 fence 绑定不一致 → 可能是两个并发 lease
			   return fmt.Errorf("%w: lease %s does not match fence lease %s", ErrFenceConflict, req.LeaseID, df.LeaseID)
		   }
	  }
	  // 3. 租约存在时验证其有效性（不消费，只读检查）
	  if req.LeaseID != "" {
		  lease,
			  lerr := g.store.GetLease(ctx, req.LeaseID)
		   if lerr != nil {
			  return lerr
		   }
		   now := g.clock()
		   if !lease.Usable(now) {
			  switch lease.State {
			  case LeaseExpired:
				  return ErrLeaseExpired
			  case LeaseConsumed:
				   return ErrLeaseConsumed
			  case LeaseRevoked:
				   return ErrLeaseRevoked
			   default:
				  return fmt.Errorf("%w: lease %s in state %s", ErrInvalidInput, req.LeaseID, lease.State)
			   }
		  }
		  if strings.ToUpper(lease.Asset) != asset {
			   return fmt.Errorf("%w: lease asset %s != guard asset %s", ErrInvalidInput, lease.Asset, asset)
		  }
	   }
	  return nil
  }

// ─────────────────────────────────────────────
// BreachPatrol：击穿巡检 Worker
// ─────────────────────────────────────────────
// PatrolReport 一轮巡检汇报。
type PatrolReport struct {
	Scanned int `json:"scanned"`
	Gated int `json:"gated"`
	Probed int `json:"probed"`
	Errs int `json:"errors"`
}

// BreachPatrol 周期巡检活跃击穿，推进状态。
type BreachPatrol struct {
	store *Store
	hot   *hotinv.Book
	ledSt ledger.Store
	clock func() time.Time
}

// NewBreachPatrol 构造巡检器。
  func NewBreachPatrol(store *Store, hot *hotinv.Book, ledSt ledger.Store) *BreachPatrol {
	  return &BreachPatrol{
		  store: store,
		  hot:   hot,
		   ledSt: ledSt,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟。

  func (p *BreachPatrol) WithClock(c func() time.Time) *BreachPatrol {
	   if p != nil && c != nil {
		   p.clock = c
	  }
	   return p
   }

   // Patrol 一轮巡检：
   // - open → gated（发现后等待一个周期再激活挡写）。
   // - 探测热库存是否仍超 cap（仍超则维持 gated；已恢复则等人工解除）。

   func (p *BreachPatrol) Patrol(ctx context.Context) (PatrolReport, error) {
	  report := PatrolReport{}
	  if p == nil || p.store == nil {
		  return report, nil
	   }
	  cases, err := p.store.ListActiveBreaches(ctx, 200)
	   if err != nil {
		  return report, fmt.Errorf("hotbreach: patrol list: %w", err)
	  }
	  report.Scanned = len(cases)
	  now := p.clock()
	  for _, bc := range cases {
		  switch bc.State {
		  case BreachOpen:
			  // open → gated
			   t := now
			   bc.State = BreachGated
			   bc.GatedAt = &t
			  if uerr := p.store.UpdateBreach(ctx, bc); uerr != nil {
				   report.Errs++
				  continue
			   }
			   report.Gated++
		   case BreachGated:
			   // 探测热库存是否仍超（信息记录）
			   report.Probed++
		   }
	  }
	  return report, nil
  }

// ─────────────────────────────────────────────
// BreachClearing：四眼解除
// ─────────────────────────────────────────────
// BreachClearing 四眼解除击穿挡写。
type BreachClearing struct {
	store *Store
	clock func() time.Time
}

// NewBreachClearing 构造四眼解除器。
   func NewBreachClearing(store *Store) *BreachClearing {
	   return &BreachClearing{
		  store: store,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

  func (c *BreachClearing) WithClock(cl func() time.Time) *BreachClearing {
	   if c != nil && cl != nil {
		   c.clock = cl
	  }
	   return c
  }

   // ClearBreach 四眼解除击穿挡写（gated → cleared）。
   // 约束：
   // - ApproverA != ApproverB（两人不同）。
  // - BreachCase 必须处于 gated 状态。
  // - 解除后释放 DrainFence HardBlock。

  func (cl *BreachClearing) ClearBreach(ctx context.Context, req ClearBreachReq) (BreachCase, error) {
	  if cl == nil || cl.store == nil {
		  return BreachCase{}, fmt.Errorf("hotbreach: clearing not initialized")
	   }
	  if strings.TrimSpace(req.BreachID) == "" {
		   return BreachCase{}, fmt.Errorf("%w: breach_id required", ErrInvalidInput)
	  }
	  approverA := strings.TrimSpace(req.ApproverA)
	   approverB := strings.TrimSpace(req.ApproverB)
	  if approverA == "" || approverB == "" {
		  return BreachCase{}, fmt.Errorf("%w: two approvers required", ErrClearNotApproved)
	  }
	  if approverA == approverB {
		   return BreachCase{}, fmt.Errorf("%w: approver_a and approver_b must differ", ErrClearNotApproved)
	   }
	   bc, err := cl.store.GetBreach(ctx, req.BreachID)
	   if err != nil {
		  return BreachCase{}, err
	  }
	  if bc.State != BreachGated {
		  return bc, fmt.Errorf("hotbreach: breach %s is in state %s, expected gated", bc.ID, bc.State)
	  }
	  now := cl.clock()
	   bc.State = BreachCleared
	  t := now
	  bc.ClearedAt = &t
	   bc.ClearedBy = fmt.Sprintf("%s+%s", approverA, approverB)
	  bc.ClearNote = req.Note
	  if err := cl.store.UpdateBreach(ctx, bc); err != nil {
		   return BreachCase{}, fmt.Errorf("hotbreach: update breach cleared: %w", err)
	   }
	  // 释放 DrainFence HardBlock
	   cl.releaseHardBlock(ctx, bc.Asset, now)
	  return bc, nil
   }

// releaseHardBlock 在 asset 下找到 HardBlock DrainFence 并释放。
  func (cl *BreachClearing) releaseHardBlock(ctx context.Context, asset string, now time.Time) {
	  if cl == nil || cl.store == nil {
		  return
	  }
	  df, ok, err := cl.store.ActiveDrainFenceForAsset(ctx, asset)
	  if err != nil || !ok {
		   return
	   }
	   if !df.HardBlock {
		   return
	  }
	  df.State = DrainFenceReleased
	   t := now
	  df.ReleasedAt = &t
	   df.HardBlock = false
	   _ = cl.store.UpdateDrainFence(ctx, df)
   }
