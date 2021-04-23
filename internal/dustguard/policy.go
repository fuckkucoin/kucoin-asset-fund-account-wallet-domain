package dustguard

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// PolicyManager 策略读取层，带短时缓存避免每信号都查 DB。

type PolicyManager struct {
	store Store
	mu    sync.Mutex
	cache map[string]policyCacheEntry // chain → entry
	ttl   time.Duration
	clock func() time.Time
}

type policyCacheEntry struct {
	pol PoolGuardPolicy
	ok    bool
	expAt time.Time
}

// NewPolicyManager 构造策略管理器。ttl 建议 60s。

  func NewPolicyManager(store Store, ttl time.Duration) *PolicyManager {
	  if ttl <= 0 {
		  ttl = 60 * time.Second
	   }
	  return &PolicyManager{
		   store: store,
		  cache: make(map[string]policyCacheEntry),
		  ttl: ttl,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// Active 返回给定链的活跃策略；缓存命中直接返回，否则查 Store。
// 找不到时返回默认策略，始终有值。

  func (pm *PolicyManager) Active(ctx context.Context, chain string) PoolGuardPolicy {
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	  pm.mu.Lock()
	  entry, ok := pm.cache[ch]
	  pm.mu.Unlock()
	   now := pm.clock()
	   if ok && now.Before(entry.expAt) {
		  if entry.ok {
			   return entry.pol
		   }
		  return DefaultPolicy()
	   }
	  // 缓存过期或未命中，查 Store
	  pol, found := pm.store.ActivePolicy(ctx, ch)
	   pm.mu.Lock()
	  pm.cache[ch] = policyCacheEntry{
		  pol: pol,
		   ok:  found,
		   expAt: now.Add(pm.ttl),
	   }
	  pm.mu.Unlock()
	   if found {
		  return pol
	   }
	  return DefaultPolicy()
   }

// Create 创建一个新策略代次。chain 为空表示全局；代次号自动递增（最大已有代次+1）。
// 若当前有同链 active 策略，新建时不自动停用旧版（运维可按需 Deactivate）。
   func (pm *PolicyManager) Create(ctx context.Context, req PoolGuardPolicy, actor string) (PoolGuardPolicy, error) {
	   ch := strings.ToUpper(strings.TrimSpace(req.Chain))
	  req.Chain = ch
	  // 计算新代次号
	  existing, _ := pm.store.ListPolicies(ctx, ch)
	  maxVer := 0
	   for _, p := range existing {
		  if strings.ToUpper(p.Chain) == ch && p.Version > maxVer {
			  maxVer = p.Version
		  }
	   }
	  now := pm.clock()
	  req.ID = uuid.NewString()
	   req.Version = maxVer + 1
	   req.Active = true
	   req.CreatedBy = strings.TrimSpace(actor)
	   req.CreatedAt = now
	   req.UpdatedAt = now
	   if err := validatePolicy(req); err != nil {
		   return PoolGuardPolicy{}, err
	   }
	  if err := pm.store.CreatePolicy(ctx, req); err != nil {
		  return PoolGuardPolicy{}, err
	  }
	   pm.invalidate(ch)
	  audit := newAudit(AuditPolicy, req.ID, actor, "create", "", "active", req.Note)
	   _ = pm.store.WriteAudit(ctx, audit)
	  return req, nil
   }

// Update 就地修改现有策略字段；代次号不变。
  func (pm *PolicyManager) Update(ctx context.Context, id string, patch PoolGuardPolicy, actor string) (PoolGuardPolicy, error) {
	   pol, err := pm.store.GetPolicy(ctx, id)
	  if err != nil {
		  return PoolGuardPolicy{}, err
	  }
	   applyPatch(&pol, patch)
	  pol.UpdatedAt = pm.clock()
	  if err := validatePolicy(pol); err != nil {
		  return PoolGuardPolicy{}, err
	   }
	   if err := pm.store.UpdatePolicy(ctx, pol); err != nil {
		   return PoolGuardPolicy{}, err
	   }
	  pm.invalidate(strings.ToUpper(pol.Chain))
	  audit := newAudit(AuditPolicy, id, actor, "update", "", "", patch.Note)
	   _ = pm.store.WriteAudit(ctx, audit)
	  return pol, nil
   }

// Deactivate 停用策略。

   func (pm *PolicyManager) Deactivate(ctx context.Context, id, actor string) error {
	  pol, err := pm.store.GetPolicy(ctx, id)
	  if err != nil {
		  return err
	   }
	  pol.Active = false
	  pol.UpdatedAt = pm.clock()
	  if err := pm.store.UpdatePolicy(ctx, pol); err != nil {
		  return err
	  }
	   pm.invalidate(strings.ToUpper(pol.Chain))
	  audit := newAudit(AuditPolicy, id, actor, "deactivate", "active", "inactive", "")
	   _ = pm.store.WriteAudit(ctx, audit)
	  return nil
  }

// List 列出策略。

   func (pm *PolicyManager) List(ctx context.Context, chain string) ([]PoolGuardPolicy, error) {
	   return pm.store.ListPolicies(ctx, chain)
  }

// Get 按 ID 查策略。

  func (pm *PolicyManager) Get(ctx context.Context, id string) (PoolGuardPolicy, error) {
	  return pm.store.GetPolicy(ctx, id)
   }

   func (pm *PolicyManager) invalidate(chain string) {
	   pm.mu.Lock()
	  delete(pm.cache, chain)
	  delete(pm.cache, "") // 全局缓存也淘汰
	  pm.mu.Unlock()
  }

// validatePolicy 检查必要字段范围合法性。
  func validatePolicy(p PoolGuardPolicy) error {
	  if p.FloodThreshold <= 0 {
		   return ErrPolicyDisabled
	   }
	   if p.FloodWindowSec <= 0 || p.ForensicsSec <= 0 {
		   return ErrPolicyDisabled
	   }
	  if p.ScoreAttack <= p.ScoreSuspect {
		   return ErrPolicyDisabled
	  }
	   return nil
  }

  func applyPatch(dst *PoolGuardPolicy, src PoolGuardPolicy) {
	  if src.FloodThreshold > 0 {
		   dst.FloodThreshold = src.FloodThreshold
	   }
	   if src.FloodWindowSec > 0 {
		  dst.FloodWindowSec = src.FloodWindowSec
	  }
	  if src.ClusterN > 0 {
		   dst.ClusterN = src.ClusterN
	   }
	  if src.ScoreAttack > 0 {
		   dst.ScoreAttack = src.ScoreAttack
	  }
	   if src.ScoreSuspect > 0 {
		  dst.ScoreSuspect = src.ScoreSuspect
	   }
	   if src.WaterLevel >= 0 {
		  dst.WaterLevel = src.WaterLevel
	  }
	  if src.ForensicsSec > 0 {
		   dst.ForensicsSec = src.ForensicsSec
	   }
	   dst.AutoQuarantine = src.AutoQuarantine
	   dst.AutoRotate = src.AutoRotate
	  if src.Note != "" {
		   dst.Note = src.Note
	   }
  }
