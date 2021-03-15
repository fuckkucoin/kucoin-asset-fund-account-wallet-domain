package brokerfence

import (
	"strings"
	"sync"
	"time"
)

// GenerationStore 世代持久化接口，由 persist.go 实现。
type GenerationStore interface {
	// GetGeneration 读取子账当前世代。若不存在返回初始零值。
	GetGeneration(subID string) (FenceGeneration, error)
	// BumpGeneration 原子 CAS：仅当 expectGen 匹配时将世代+1，更新 fenced 位。
	// 返回更新后的世代。并发冲突返回 ErrGenerationMismatch。
	BumpGeneration(subID string,
		expectGen int64, fenced bool, now time.Time) (FenceGeneration, error)
	// ListFencedSubs 返回当前处于围栏态的子账 ID 列表（用于 worker 巡检）。
	ListFencedSubs() ([]string, error)
}

// generationCache 内存缓存，减少频繁读 DB。

type generationCache struct {
	mu sync.RWMutex
	cache map[string]FenceGeneration
}

  func newGenerationCache() *generationCache {
	  return &generationCache{cache: make(map[string]FenceGeneration)}
   }

  func (c *generationCache) set(g FenceGeneration) {
	   c.mu.Lock()
	   c.cache[g.SubID] = g
	   c.mu.Unlock()
  }

  func (c *generationCache) get(subID string) (FenceGeneration, bool) {
	  c.mu.RLock()
	  g, ok := c.cache[subID]
	   c.mu.RUnlock()
	  return g, ok
   }

  func (c *generationCache) invalidate(subID string) {
	   c.mu.Lock()
	  delete(c.cache, subID)
	  c.mu.Unlock()
   }

// GenerationManager 围栏世代管理器，封装 Store + 内存缓存。
type GenerationManager struct {
	store GenerationStore
	cache *generationCache
	clock func() time.Time
}

// NewGenerationManager 构造世代管理器。
   func NewGenerationManager(store GenerationStore) *GenerationManager {
	   return &GenerationManager{
		  store: store,
		   cache: newGenerationCache(),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (gm *GenerationManager) now() time.Time {
	  if gm.clock != nil {
		  return gm.clock()
	   }
	   return time.Now().UTC()
   }

// Current 读取子账当前世代（先读缓存，缓存未命中走 Store）。

   func (gm *GenerationManager) Current(subID string) (FenceGeneration, error) {
	  subID = strings.TrimSpace(subID)
	  if subID == "" {
		  return FenceGeneration{}, ErrInput
	   }
	  if g, ok := gm.cache.get(subID); ok {
		   return g, nil
	  }
	   g, err := gm.store.GetGeneration(subID)
	  if err != nil {
		  return FenceGeneration{}, err
	  }
	  gm.cache.set(g)
	   return g, nil
   }

// BumpFence 围栏时抬升世代：fenced=true，generation+1。
// expectGen 必须等于当前世代，否则返回 ErrGenerationMismatch（CAS 失败）。

   func (gm *GenerationManager) BumpFence(subID string, expectGen int64) (FenceGeneration, error) {
	  subID = strings.TrimSpace(subID)
	  if subID == "" {
		   return FenceGeneration{}, ErrInput
	  }
	  now := gm.now()
	  g,
		   err := gm.store.BumpGeneration(subID, expectGen, true, now)
	   if err != nil {
		  return FenceGeneration{}, err
	   }
	  gm.cache.set(g)
	   return g, nil
   }

// BumpUnfence 解围时抬升世代：fenced=false，generation+1。
   func (gm *GenerationManager) BumpUnfence(subID string, expectGen int64) (FenceGeneration, error) {
	   subID = strings.TrimSpace(subID)
	  if subID == "" {
		  return FenceGeneration{}, ErrInput
	   }
	   now := gm.now()
	  g, err := gm.store.BumpGeneration(subID, expectGen, false, now)
	   if err != nil {
		  return FenceGeneration{}, err
	  }
	  gm.cache.set(g)
	   return g, nil
  }

// CheckWriteEligible 写路径前置校验：
// 1. 世代号与请求携带的 generation 必须一致；
// 2. 若当前处于围栏态，调用方须持有有效互斥令牌。
// 返回当前世代供调用方记录审计。

   func (gm *GenerationManager) CheckWriteEligible(req AllocCheckReq, tokens *MutexStore) (FenceGeneration, error) {
	  g, err := gm.Current(req.SubID)
	   if err != nil {
		   return FenceGeneration{}, err
	  }
	  if g.Generation != req.Generation {
		   return FenceGeneration{}, ErrGenerationMismatch
	  }
	  if g.Fenced {
		  // 围栏期间：必须持有有效互斥令牌，否则 fail-closed。
		  if strings.TrimSpace(req.Token) == "" {
			   return FenceGeneration{}, ErrMutexDenied
		   }
		   if err := tokens.ValidateToken(req.SubID, req.Token, gm.now()); err != nil {
			  return FenceGeneration{}, err
		   }
	  }
	  return g, nil
  }

// InvalidateCache 让指定子账的世代缓存失效（如 worker 发现变更后调用）。

   func (gm *GenerationManager) InvalidateCache(subID string) {
	   gm.cache.invalidate(strings.TrimSpace(subID))
   }

// ListFencedSubs 列出当前处于围栏态的子账（用于旁路巡检）。
  func (gm *GenerationManager) ListFencedSubs() ([]string, error) {
	   return gm.store.ListFencedSubs()
  }
