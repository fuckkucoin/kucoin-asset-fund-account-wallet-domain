package hotbreach

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// generationCounter 全局世代计数器（进程内单调递增，持久化后以 DB 值为准）。

var generationCounter int64

// nextGeneration 返回下一个世代序号（进程内原子递增）。

  func nextGeneration() int64 {
	  return atomic.AddInt64(&generationCounter,
		   1)
  }

// newToken 生成 32 字节随机 hex token。

  func newToken() string {
	  var b [16]byte
	   _, _ = rand.Read(b[:])
	  return hex.EncodeToString(b[:])
   }

// ─────────────────────────────────────────────
// LeaseManager 租约管理器
// ─────────────────────────────────────────────
// LeaseManager 管理 PreLease 的完整生命周期。
// 依赖 hotinv.Book 只读读取热库存快照。
type LeaseManager struct {
	mu sync.Mutex
	store *Store
	hot *hotinv.Book
	ledSt ledger.Store
	clock func() time.Time
}

// NewLeaseManager 构造租约管理器。
   func NewLeaseManager(store *Store, hot *hotinv.Book, ledSt ledger.Store) *LeaseManager {
	   return &LeaseManager{
		   store: store,
		  hot:   hot,
		   ledSt: ledSt,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入可控时钟（用）。

  func (m *LeaseManager) WithClock(c func() time.Time) *LeaseManager {
	  if m != nil && c != nil {
		  m.clock = c
	  }
	  return m
  }

   // IssueLease 在 PreSubmit 通过后签发租约。
  // 步骤：
  // 1. 校验入参非空。
  // 2. 读取热库存快照（只读，不执行归集）。
   // 3. 检查该 asset 是否存在活跃击穿 → 若有则拒绝签发（fail-closed）。
   // 4. 签发 PreLease，写入 KV。
  // 5. 创建对应的 DrainFence。

  func (m *LeaseManager) IssueLease(ctx context.Context, req IssueLeaseReq) (PreLease, error) {
	  if m == nil || m.store == nil {
		  return PreLease{}, fmt.Errorf("hotbreach: lease manager not initialized")
	   }
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   if asset == "" {
		  return PreLease{}, fmt.Errorf("%w: asset required", ErrInvalidInput)
	   }
	  issuedBy := strings.TrimSpace(req.IssuedBy)
	  if issuedBy == "" {
		  return PreLease{}, fmt.Errorf("%w: issued_by required", ErrInvalidInput)
	   }
	  // 1. 失败即关：存在活跃击穿则拒绝签发
	   blocked, breachID, err := m.store.HasActiveBreachForAsset(ctx, asset)
	  if err != nil {
		   return PreLease{}, fmt.Errorf("hotbreach: check breach for asset %s: %w", asset, err)
	  }
	   if blocked {
		  return PreLease{}, fmt.Errorf("%w: active breach %s blocks lease issuance", ErrBreachGated, breachID)
	   }
	  // 2. 读取热库存快照
	   var snapHot, snapCap string
	  if m.hot != nil && m.ledSt != nil {
		   snaps,
			   serr := m.hot.Observe(ctx, m.ledSt)
		   if serr != nil {
			  return PreLease{}, fmt.Errorf("hotbreach: read hot inventory snapshot: %w", serr)
		  }
		  for _, sn := range snaps {
			  if strings.ToUpper(sn.Asset) == asset {
				  snapHot = sn.HotAvail
				   snapCap = sn.HotMax
				   break
			   }
		  }
	  }
	   // 3. TTL
	   ttl := req.TTL
	   if ttl <= 0 {
		  ttl = DefaultLeaseTTL
	   }
	  now := m.clock()
	  // 4. 签发
	  l := PreLease{
		   ID:         uuid.NewString(),
		  Asset:      asset,
		  Generation: nextGeneration(),
		   SnapshotHot: snapHot,
		  SnapshotCap: snapCap,
		   IssuedBy:  issuedBy,
		   State:     LeaseIssued,
		   IssuedAt:  now,
		   ExpiresAt: now.Add(ttl),
		  FenceEpoch: 0,
		   FenceToken: newToken(),
	   }
	  if err := m.store.SaveLease(ctx, l); err != nil {
		  return PreLease{}, fmt.Errorf("hotbreach: save lease: %w", err)
	   }
	   // 5. 同步创建 DrainFence
	  fence := DrainFence{
		   ID: uuid.NewString(),
		  Asset: asset,
		  LeaseID:   l.ID,
		  HardBlock: false,
		   State:     DrainFenceActive,
		   CreatedAt: now,
	   }
	   if err := m.store.SaveDrainFence(ctx, fence); err != nil {
		  // DrainFence 写失败不影响租约有效性，记录即可
		  _ = err
	   }
	   return l, nil
  }

// ConsumeLease 出金/归集预扣前消费租约。
// 验证顺序：
// 1. 租约存在且 State == issued。
// 2. 租约未过期。
// 3. 租约 Asset 与请求 Asset 一致。
// 4. 检查当前 asset 热库存是否已超 cap → 超则触发 BreachCase 并返回 ErrBreachGated。
// 5. CAS 转 consumed。

  func (m *LeaseManager) ConsumeLease(ctx context.Context, req ConsumeLeaseReq) (PreLease, error) {
	   if m == nil || m.store == nil {
		   return PreLease{}, fmt.Errorf("hotbreach: lease manager not initialized")
	  }
	   leaseID := strings.TrimSpace(req.LeaseID)
	   if leaseID == "" {
		   return PreLease{}, ErrNoLease
	  }
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   consumedBy := strings.TrimSpace(req.ConsumedBy)
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  lease, err := m.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return PreLease{}, err
	  }
	   now := m.clock()
	   // State 检查
	  switch lease.State {
	  case LeaseConsumed:
		  return PreLease{}, ErrLeaseConsumed
	   case LeaseExpired:
		   return PreLease{}, ErrLeaseExpired
	   case LeaseRevoked:
		   return PreLease{}, ErrLeaseRevoked
	   case LeaseIssued:
	   // continue
	  default:
		   return PreLease{}, fmt.Errorf("hotbreach: unexpected lease state %s", lease.State)
	   }
	   if lease.Expired(now) {
		   // 时钟竞态下可能已到期但 state 还未更新
		   lease.State = LeaseExpired
		   t := now
		  lease.RevokedAt = &t
		  lease.RevokeReason = "clock expired at consume"
		  _ = m.store.UpdateLease(ctx, lease)
		   return PreLease{}, ErrLeaseExpired
	   }
	   if lease.Asset != asset {
		  return PreLease{}, fmt.Errorf("%w: lease asset %s != requested %s", ErrInvalidInput, lease.Asset, asset)
	   }
	   // 检查热库存是否已超 cap（并发扣减可能已先到）
	  if m.hot != nil && m.ledSt != nil {
		   snaps, serr := m.hot.Observe(ctx, m.ledSt)
		  if serr == nil {
			   for _, sn := range snaps {
				   if strings.ToUpper(sn.Asset) == asset && sn.Over {
					  // 热库存超 cap → 击穿
					   _, berr := m.triggerBreach(ctx, lease, sn, consumedBy, now)
					  if berr != nil {
						   return PreLease{}, fmt.Errorf("hotbreach: trigger breach: %w", berr)
					  }
					   return PreLease{}, ErrBreachGated
				  }
			  }
		  }
	  }
	   // CAS 转 consumed
	  lease.State = LeaseConsumed
	   t := now
	   lease.ConsumedAt = &t
	  lease.ConsumedBy = consumedBy
	   if err := m.store.UpdateLease(ctx, lease); err != nil {
		  if err == persist.ErrStaleFence {
			   return PreLease{}, fmt.Errorf("%w: concurrent consume conflict", ErrLeaseRevoked)
		   }
		  return PreLease{}, fmt.Errorf("hotbreach: update lease consumed: %w", err)
	   }
	  // 释放对应 DrainFence（尽力释放）
	   m.releaseDrainFenceForLease(ctx, lease.ID, now)
	   return lease, nil
   }

// RevokeLease 撤销租约（管理员或击穿检测调用）。
   func (m *LeaseManager) RevokeLease(ctx context.Context, leaseID, reason string) error {
	  if m == nil || m.store == nil {
		   return fmt.Errorf("hotbreach: lease manager not initialized")
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   lease, err := m.store.GetLease(ctx, leaseID)
	   if err != nil {
		   return err
	  }
	  if lease.State != LeaseIssued {
		   return nil // 幂等
	   }
	  now := m.clock()
	  lease.State = LeaseRevoked
	   t := now
	  lease.RevokedAt = &t
	  lease.RevokeReason = reason
	   if err := m.store.UpdateLease(ctx, lease); err != nil {
		   return fmt.Errorf("hotbreach: revoke lease: %w", err)
	   }
	  m.releaseDrainFenceForLease(ctx, leaseID, now)
	   return nil
  }

// ExpireLeases 过期 worker 调用：扫描所有 issued 租约，TTL 到期则转 expired。
// 返回本轮过期数量。

   func (m *LeaseManager) ExpireLeases(ctx context.Context) (int, error) {
	   if m == nil || m.store == nil {
		   return 0, nil
	  }
	   leases, err := m.store.ListIssuedLeases(ctx, 500)
	   if err != nil {
		   return 0, fmt.Errorf("hotbreach: expire leases list: %w", err)
	   }
	  now := m.clock()
	  count := 0
	   for _, l := range leases {
		   if !l.Expired(now) {
			   continue
		  }
		  l.State = LeaseExpired
		  t := now
		   l.RevokedAt = &t
		   l.RevokeReason = "ttl expired"
		   if uerr := m.store.UpdateLease(ctx, l); uerr != nil {
			   continue // CAS 冲突忽略，下轮再试
		   }
		  m.releaseDrainFenceForLease(ctx, l.ID, now)
		  count++
	   }
	  return count, nil
   }

// triggerBreach 检测到超 cap 时触发 BreachCase。
// 内部调用，调用方已持 mu。
   func (m *LeaseManager) triggerBreach(
	   ctx context.Context,
	   lease PreLease,
	   snap hotinv.Snapshot,
	  concurrentParty string,
	   now time.Time,
   ) (BreachCase, error) {
	   cpHash := hashParty(concurrentParty)
	   bc := BreachCase{
		   ID:    uuid.NewString(),
		   Asset: lease.Asset,
		   LeaseID: lease.ID,
		  PreCheckHot: lease.SnapshotHot,
		   BreachHot:   snap.HotAvail,
		   CapValue:            snap.HotMax,
		  Excess:              snap.Excess,
		   ConcurrentParty:     concurrentParty,
		   ConcurrentPartyHash: cpHash,
		  State:               BreachOpen,
		  DetectedAt:          now,
	  }
	   if err := m.store.SaveBreach(ctx, bc); err != nil {
		  return BreachCase{}, err
	  }
	  // 同步激活 DrainFence HardBlock
	  if df, ok, ferr := m.store.ActiveDrainFenceForAsset(ctx, lease.Asset); ferr == nil && ok {
		  df.HardBlock = true
		   df.BreachID = bc.ID
		   t := now
		  _ = t
		  _ = m.store.UpdateDrainFence(ctx, df)
	   }
	   // 生成证明
	   proof, perr := buildProof(bc, lease, snap, now)
	   if perr == nil {
		  bc.ProofID = proof.ID
		   _ = m.store.SaveProof(ctx, proof)
		  // 更新 BreachCase.ProofID（尽力写）
		  fresh, gerr := m.store.GetBreach(ctx, bc.ID)
		  if gerr == nil {
			   fresh.ProofID = proof.ID
			   _ = m.store.UpdateBreach(ctx, fresh)
		   }
	   }
	   // 租约转 revoked
	   lease.State = LeaseRevoked
	  t := now
	   lease.RevokedAt = &t
	  lease.RevokeReason = fmt.Sprintf("breach %s detected", bc.ID)
	  _ = m.store.UpdateLease(ctx, lease)
	   return bc, nil
   }

// releaseDrainFenceForLease 释放租约关联的 DrainFence（尽力，非 hard-block 才释放）。
   func (m *LeaseManager) releaseDrainFenceForLease(ctx context.Context, leaseID string, now time.Time) {
	  rows, err := m.store.kv.List(ctx, kindDrainFence, 500)
	   if err != nil {
		   return
	   }
	   for _, row := range rows {
		  var f DrainFence
		  if err := unmarshalFence(row.Body, &f); err != nil {
			   continue
		   }
		  if f.LeaseID != leaseID || f.State != DrainFenceActive {
			  continue
		  }
		   if f.HardBlock {
			  continue // 硬挡写须四眼解除，不在此释放
		  }
		  f.State = DrainFenceReleased
		   t := now
		  f.ReleasedAt = &t
		  f.FenceEpoch = row.Fence.Epoch
		   f.FenceToken = row.Fence.Token
		   _ = m.store.UpdateDrainFence(ctx, f)
	  }
  }
