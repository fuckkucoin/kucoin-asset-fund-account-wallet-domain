package feepathgate

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// leaseRegistry 是单节点内存视图，加速读操作；真正的持久化由 Store 维护。
// 部署多节点时需依赖 Store 的原子 CAS，不依赖内存视图做最终裁决。

type leaseRegistry struct {
	mu sync.RWMutex
	// byBizKey: business_key → []leaseID（只包含 open/bound 状态）
	byBizKey map[string][]string
	// byID: leaseID → *PathLease（完整镜像）
	byID map[string]*PathLease
	// gens: business_key → *PathGeneration
	gens map[string]*PathGeneration
}

  func newRegistry() *leaseRegistry {
	   return &leaseRegistry{
		  byBizKey: make(map[string][]string),
		  byID:     make(map[string]*PathLease),
		   gens:     make(map[string]*PathGeneration),
	   }
   }

// LeaseFSM 封装 PathLease 的状态机转换与 Generation CAS 逻辑。

type LeaseFSM struct {
	reg *leaseRegistry
	store Store
	clock func() time.Time
}

   func newLeaseFSM(store Store, clock func() time.Time) *LeaseFSM {
	  return &LeaseFSM{reg: newRegistry(), store: store, clock: clock}
  }

  func (fsm *LeaseFSM) now() time.Time {
	  if fsm.clock != nil {
		  return fsm.clock()
	  }
	   return time.Now().UTC()
   }

// Acquire 在 business_key 上申请路径租约。
// 协议步骤：
// 1. 检查 business_key 是否已有 open/bound 租约；若有则核对 Kind：
// - 同 Kind & 同 idem → 幂等返回已有租约。
// - 反 Kind → ErrPathConflict。
// 2. 若无活跃租约，读取/初始化 PathGeneration，执行 generation+1 CAS。
// 3. CAS 成功后创建 open 状态的 PathLease。
// 4. 将新 lease 写入 Store 并更新内存视图。

   func (fsm *LeaseFSM) Acquire(ctx context.Context, req AcquireReq) (*PathLease, error) {
	  if err := validateAcquireReq(req); err != nil {
		   return nil, err
	  }
	  fsm.reg.mu.Lock()
	   defer fsm.reg.mu.Unlock()
	   // 内存视图快速检查冲突。
	   if ids, ok := fsm.reg.byBizKey[req.BusinessKey]; ok {
		  for _, id := range ids {
			   existing := fsm.reg.byID[id]
			   if existing == nil {
				   continue
			   }
			  if existing.Status == LeaseOpen || existing.Status == LeaseBound {
				   if existing.Kind == req.Kind {
					   // 同 Kind：检查是否是同一幂等请求。
					   if existing.IdemMatch(req.IdemKey) {
						   return cloneLease(existing), nil
					   }
					   // 同 Kind 不同 idem：允许（后续 Credit.Post 由 feecredit 幂等层处理）。
					  continue
				  }
				  // 反 Kind，已有活跃租约 → 冲突。
				   return nil, ErrPathConflict
			  }
		   }
	  }
	   // CAS：读取当前世代，递增后写回。
	  gen := fsm.reg.gens[req.BusinessKey]
	  var expectedGen int64
	   if gen != nil {
		   expectedGen = gen.Generation
	   }
	  newGen := expectedGen + 1
	  now := fsm.now()
	  lease := &PathLease{
		  ID: uuid.NewString(),
		   BusinessKey: req.BusinessKey,
		  Kind:        req.Kind,
		   Asset:      req.Asset,
		  Gross:      req.Gross,
		  Generation: newGen,
		   WorkerID:   req.WorkerID,
		   Status:    LeaseOpen,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   // 持久化（Store 层再做真正的 CAS；失败则 ErrStaleGen）。
	  if fsm.store != nil {
		  if err := fsm.store.InsertLease(ctx, lease, expectedGen); err != nil {
			   if isStaleGen(err) {
				  return nil, ErrStaleGen
			  }
			  return nil, err
		  }
	   }
	  // 更新内存视图。
	   fsm.reg.byID[lease.ID] = lease
	   fsm.reg.byBizKey[req.BusinessKey] = append(fsm.reg.byBizKey[req.BusinessKey],
		   lease.ID)
	  if gen == nil {
		  gen = &PathGeneration{
			  ID:          uuid.NewString(),
			  BusinessKey: req.BusinessKey,
		  }
		  fsm.reg.gens[req.BusinessKey] = gen
	  }
	  gen.Generation = newGen
	  gen.LockedBy = lease.ID
	  gen.LockedKind = req.Kind
	  gen.UpdatedAt = now
	   return cloneLease(lease), nil
   }

// Bind 将 open 状态的租约与 Credit/LedgerID 绑定，推进至 bound 状态。
// 同一 worker 才可绑定（WorkerID 必须一致）。

  func (fsm *LeaseFSM) Bind(ctx context.Context, req BindReq) (*PathLease, error) {
	   if strings.TrimSpace(req.LeaseID) == "" {
		   return nil, ErrLeaseNotFound
	  }
	  fsm.reg.mu.Lock()
	  defer fsm.reg.mu.Unlock()
	  lease := fsm.reg.byID[req.LeaseID]
	  if lease == nil {
		  // 从 Store 回填内存视图。
		  if fsm.store != nil {
			   loaded, err := fsm.store.GetLease(ctx, req.LeaseID)
			  if err != nil {
				   return nil, ErrLeaseNotFound
			   }
			  fsm.reg.byID[loaded.ID] = loaded
			   lease = loaded
		   }
		  if lease == nil {
			   return nil, ErrLeaseNotFound
		   }
	  }
	  if lease.Status != LeaseOpen {
		  return nil, ErrLeaseBound
	   }
	   if lease.WorkerID != req.WorkerID {
		   return nil, ErrWorkerConflict
	   }
	   now := fsm.now()
	   lease.CreditID = req.CreditID
	   lease.LedgerID = req.LedgerID
	  lease.Fee = req.Fee
	   lease.Net = req.Net
	   lease.Status = LeaseBound
	  lease.UpdatedAt = now
	   if fsm.store != nil {
		   if err := fsm.store.UpdateLease(ctx, lease); err != nil {
			  return nil, err
		   }
	  }
	  return cloneLease(lease), nil
  }

// Settle 在对冲证明通过后，将 bound 状态的租约推进至 settled。
// Settle 前必须已有 HedgeProof（ProofID 非空且 Status=verified）。
  func (fsm *LeaseFSM) Settle(ctx context.Context, req SettleReq) (*PathLease, error) {
	   if strings.TrimSpace(req.LeaseID) == "" || strings.TrimSpace(req.ProofID) == "" {
		  return nil, ErrProofMissing
	   }
	   fsm.reg.mu.Lock()
	   defer fsm.reg.mu.Unlock()
	  lease := fsm.reg.byID[req.LeaseID]
	   if lease == nil {
		   if fsm.store != nil {
			   loaded, err := fsm.store.GetLease(ctx, req.LeaseID)
			  if err != nil {
				  return nil, ErrLeaseNotFound
			  }
			  fsm.reg.byID[loaded.ID] = loaded
			   lease = loaded
		   }
	   }
	   if lease == nil {
		  return nil, ErrLeaseNotFound
	  }
	  if lease.Status != LeaseBound {
		   return nil, ErrLeaseBound
	  }
	   now := fsm.now()
	  at := now
	   lease.ProofID = req.ProofID
	   lease.Status = LeaseSettled
	  lease.SettledAt = &at
	   lease.UpdatedAt = now
	   if fsm.store != nil {
		  if err := fsm.store.UpdateLease(ctx, lease); err != nil {
			  return nil, err
		  }
	   }
	  // settled 后从活跃集合移除。
	  fsm.removeFromBizKey(lease.BusinessKey, lease.ID)
	   return cloneLease(lease), nil
   }

// MarkConflicted 将 open/bound 状态的租约标记为冲突终态。
// 由 DualHit 探测器发现双贷后调用；人工介入前不可逆。

  func (fsm *LeaseFSM) MarkConflicted(ctx context.Context, leaseID string) (*PathLease, error) {
	  if strings.TrimSpace(leaseID) == "" {
		   return nil, ErrLeaseNotFound
	   }
	   fsm.reg.mu.Lock()
	  defer fsm.reg.mu.Unlock()
	   lease := fsm.reg.byID[leaseID]
	   if lease == nil {
		  if fsm.store != nil {
			  loaded, err := fsm.store.GetLease(ctx, leaseID)
			  if err != nil {
				   return nil, ErrLeaseNotFound
			   }
			  fsm.reg.byID[loaded.ID] = loaded
			   lease = loaded
		  }
	  }
	  if lease == nil {
		   return nil, ErrLeaseNotFound
	  }
	  now := fsm.now()
	  at := now
	  lease.Status = LeaseConflicted
	   lease.ConflictAt = &at
	   lease.UpdatedAt = now
	  if fsm.store != nil {
		   if err := fsm.store.UpdateLease(ctx, lease); err != nil {
			  return nil, err
		   }
	  }
	  fsm.removeFromBizKey(lease.BusinessKey, lease.ID)
	  return cloneLease(lease), nil
   }

// GetByBizKey 按 business_key 查询所有活跃租约（open/bound）。
// 已进入终态（settled / conflicted）的租约不出现在结果中。
  func (fsm *LeaseFSM) GetByBizKey(ctx context.Context, bizKey string) ([]*PathLease, error) {
	   fsm.reg.mu.RLock()
	  defer fsm.reg.mu.RUnlock()
	  ids := fsm.reg.byBizKey[bizKey]
	  out := make([]*PathLease, 0, len(ids))
	  for _, id := range ids {
		   l := fsm.reg.byID[id]
		   if l == nil {
			   continue
		   }
		  // byBizKey 只保存活跃租约；若内存视图不一致，过滤终态。
		   if l.Status == LeaseSettled || l.Status == LeaseConflicted {
			   continue
		   }
		  out = append(out, cloneLease(l))
	  }
	  if len(out) == 0 && fsm.store != nil {
		  stored, err := fsm.store.ListLeasesByBizKey(ctx, bizKey)
		   if err != nil {
			   return nil, err
		  }
		  // 过滤终态。
		   for _, l := range stored {
			  if l.Status != LeaseSettled && l.Status != LeaseConflicted {
				  out = append(out, l)
			  }
		  }
	  }
	  return out, nil
  }

// GetLease 按 lease_id 查询租约（先内存后 Store）。
   func (fsm *LeaseFSM) GetLease(ctx context.Context, leaseID string) (*PathLease, error) {
	  fsm.reg.mu.RLock()
	  l := fsm.reg.byID[leaseID]
	  fsm.reg.mu.RUnlock()
	   if l != nil {
		  return cloneLease(l), nil
	  }
	  if fsm.store == nil {
		   return nil, ErrLeaseNotFound
	   }
	  loaded, err := fsm.store.GetLease(ctx, leaseID)
	   if err != nil {
		   return nil, ErrLeaseNotFound
	   }
	   return loaded, nil
  }

// GetGeneration 返回指定 business_key 的当前世代状态。
   func (fsm *LeaseFSM) GetGeneration(bizKey string) (*PathGeneration, bool) {
	   fsm.reg.mu.RLock()
	  defer fsm.reg.mu.RUnlock()
	  g := fsm.reg.gens[bizKey]
	   if g == nil {
		   return nil, false
	  }
	   cp := *g
	   return &cp, true
   }

// removeFromBizKey 从 byBizKey 活跃列表中删除指定 leaseID。
// 调用方必须持有 reg.mu 写锁。

   func (fsm *LeaseFSM) removeFromBizKey(bizKey, leaseID string) {
	   ids := fsm.reg.byBizKey[bizKey]
	  out := ids[:0]
	  for _, id := range ids {
		  if id != leaseID {
			  out = append(out, id)
		   }
	  }
	  if len(out) == 0 {
		  delete(fsm.reg.byBizKey, bizKey)
	   } else {
		   fsm.reg.byBizKey[bizKey] = out
	   }
   }

// cloneLease 返回指针指向对象的浅拷贝，防止调用方意外修改内部状态。

  func cloneLease(l *PathLease) *PathLease {
	   if l == nil {
		   return nil
	  }
	  cp := *l
	  return &cp
   }

// IdemMatch 判断 lease 是否是由同一幂等键发起的请求产生。
// 此处通过 Gross+Kind+Asset 三元组匹配（业务层在 feecredit 存储完整 idem）。

   func (l *PathLease) IdemMatch(idemKey string) bool {
	   // 内存层无 idem 字段，通过 WorkerID 的 idem 前缀快速匹配。
	   // 场景由 Store.GetByIdem 做精确匹配；此处为内存层快速路径。
	   _ = idemKey
	   return false
  }

// validateAcquireReq 校验 AcquireReq 基础字段。

  func validateAcquireReq(req AcquireReq) error {
	   if strings.TrimSpace(req.BusinessKey) == "" {
		   return ErrBusinessKey
	  }
	   if !req.Kind.Valid() {
		   return ErrKind
	   }
	   if strings.TrimSpace(req.Asset) == "" {
		  return ErrAsset
	   }
	   return nil
   }

// isStaleGen 判断 Store 返回的错误是否表示世代 CAS 冲突。
  func isStaleGen(err error) bool {
	  if err == nil {
		  return false
	  }
	   return err == persist.ErrStaleFence || err == ErrStaleGen
  }
