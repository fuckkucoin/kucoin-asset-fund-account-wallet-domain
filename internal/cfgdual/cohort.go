package cfgdual

import (
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
)

// ─────────────────────────────────────────────────────────────────────────────
// Cohort 灰度队列（进程内内存状态）
// ─────────────────────────────────────────────────────────────────────────────
// Cohort 管理单个 GrayRelease 的切流规则与世代状态。
// 线程安全；SQL 持久化由 Store 负责。
type Cohort struct {
	mu sync.RWMutex
	// releaseID 所属发布 ID。
	releaseID string
	// packName 对应策略包名称。
	packName string
	// generation 当前世代号；每次变更切流参数时自增。
	generation int
	// percent 全局比例 [0,100]；0 = 全走 baseline，100 = 全走 canary。
	percent int
	// userPins user_id → "baseline" | "canary" 显式 Pin。
	userPins map[string]string
	// assetPins asset → "baseline" | "canary" 显式 Pin。
	assetPins map[string]string
}

// NewCohort 构造 Cohort。generation 必须 >= 1。

  func NewCohort(releaseID, packName string, percent, generation int) *Cohort {
	  if generation <= 0 {
		  generation = 1
	   }
	  if percent < 0 {
		   percent = 0
	  }
	   if percent > 100 {
		  percent = 100
	   }
	   return &Cohort{
		  releaseID: releaseID,
		   packName:   packName,
		  generation: generation,
		   percent:    percent,
		   userPins:   map[string]string{},
		   assetPins: map[string]string{},
	  }
  }

// Generation 返回当前世代号（只读）。
   func (c *Cohort) Generation() int {
	  c.mu.RLock()
	   defer c.mu.RUnlock()
	   return c.generation
   }

// Percent 返回当前全局比例（只读）。

  func (c *Cohort) Percent() int {
	   c.mu.RLock()
	   defer c.mu.RUnlock()
	   return c.percent
   }

// SetPercent 变更全局切流比例并自增世代号。
// 比例必须在 [0,100]；返回新世代号。

  func (c *Cohort) SetPercent(percent int) (int, error) {
	  if percent < 0 || percent > 100 {
		  return 0, fmt.Errorf("%w: cohort percent must be in [0,100], got %d", ErrConflict, percent)
	   }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	   c.percent = percent
	   c.generation++
	  return c.generation, nil
  }

// PinUser 为指定用户显式固定切流侧。side 为 "canary" 或 "baseline"。
  func (c *Cohort) PinUser(userID, side string) error {
	  userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return fmt.Errorf("%w: user_id required for cohort pin", ErrConflict)
	  }
	   if side != "canary" && side != "baseline" {
		   return fmt.Errorf("%w: side must be canary or baseline", ErrConflict)
	  }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	   c.userPins[userID] = side
	   return nil
  }

// UnpinUser 移除用户的显式 Pin，还原为比例切流。
  func (c *Cohort) UnpinUser(userID string) {
	  c.mu.Lock()
	  delete(c.userPins, strings.TrimSpace(userID))
	   c.mu.Unlock()
   }

// PinAsset 为指定资产显式固定切流侧。
  func (c *Cohort) PinAsset(asset, side string) error {
	   asset = strings.TrimSpace(asset)
	  if asset == "" {
		  return fmt.Errorf("%w: asset required for cohort pin", ErrConflict)
	  }
	   if side != "canary" && side != "baseline" {
		  return fmt.Errorf("%w: side must be canary or baseline", ErrConflict)
	   }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  c.assetPins[asset] = side
	   return nil
  }

// UnpinAsset 移除资产的显式 Pin。
   func (c *Cohort) UnpinAsset(asset string) {
	  c.mu.Lock()
	  delete(c.assetPins, strings.TrimSpace(asset))
	   c.mu.Unlock()
  }

// InCanary 判断本次请求是否命中 canary 队列。
// 先级（从高到低）：
// 1. 用户显式 Pin（"canary" → true；"baseline" → false）；
// 2. 资产显式 Pin（同上）；
// 3. 全局比例：hash(userID + "|" + packName + "|" + generation) % 100 < percent。
// 同一 userID + packName + generation 三元组在任意实例上结论相同（无状态可回放）。
   func (c *Cohort) InCanary(userID, asset string) (inCanary bool, gen int) {
	   c.mu.RLock()
	   defer c.mu.RUnlock()
	  gen = c.generation
	  // 用户显式 Pin 最先。
	  if side, ok := c.userPins[strings.TrimSpace(userID)]; ok {
		  return side == "canary", gen
	  }
	   // 资产显式 Pin 次之。
	  if asset != "" {
		  if side, ok := c.assetPins[strings.TrimSpace(asset)]; ok {
			   return side == "canary", gen
		  }
	  }
	  // 全局比例。
	   if c.percent <= 0 {
		  return false, gen
	  }
	  if c.percent >= 100 {
		  return true, gen
	   }
	  key := strings.TrimSpace(userID) + "|" + c.packName + "|" + fmt.Sprintf("%d", c.generation)
	   h := fnv.New32a()
	  _, _ = h.Write([]byte(key))
	  return int(h.Sum32()%100) < c.percent, gen
  }

// Snapshot 快照当前切流参数，用于调试与 API 展示。
   func (c *Cohort) Snapshot() CohortSnapshot {
	   c.mu.RLock()
	  defer c.mu.RUnlock()
	  up := make(map[string]string, len(c.userPins))
	   for k, v := range c.userPins {
		   up[k] = v
	  }
	  ap := make(map[string]string, len(c.assetPins))
	   for k, v := range c.assetPins {
		   ap[k] = v
	   }
	  return CohortSnapshot{
		  ReleaseID: c.releaseID,
		  PackName: c.packName,
		   Generation: c.generation,
		  Percent:    c.percent,
		   UserPins:  up,
		   AssetPins: ap,
	  }
  }

// CohortSnapshot 切流参数快照值对象（只读展示用）。

type CohortSnapshot struct {
	ReleaseID  string `json:"release_id"`
	PackName   string `json:"pack_name"`
	Generation int    `json:"generation"`
	Percent  int               `json:"percent"`
	UserPins map[string]string `json:"user_pins,omitempty"`
	AssetPins map[string]string `json:"asset_pins,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// CohortRegistry 进程级 Cohort 注册表
// ─────────────────────────────────────────────────────────────────────────────
// CohortRegistry 管理多个 GrayRelease 对应的 Cohort，支持按 releaseID 快速查找。
type CohortRegistry struct {
	mu      sync.RWMutex
	cohorts map[string]*Cohort // key = releaseID
}

// NewCohortRegistry 构造空注册表。
   func NewCohortRegistry() *CohortRegistry {
	  return &CohortRegistry{cohorts: map[string]*Cohort{}}
   }

// Register 注册或替换一个 Cohort（发布激活 canary 时调用）。

   func (r *CohortRegistry) Register(c *Cohort) {
	  if c == nil {
		  return
	  }
	  r.mu.Lock()
	   r.cohorts[c.releaseID] = c
	  r.mu.Unlock()
  }

// Get 按 releaseID 查找 Cohort；不存在返回 nil, ErrNotFound。
   func (r *CohortRegistry) Get(releaseID string) (*Cohort, error) {
	   r.mu.RLock()
	   c, ok := r.cohorts[releaseID]
	   r.mu.RUnlock()
	  if !ok {
		  return nil, ErrNotFound
	  }
	   return c, nil
   }

// Remove 发布封堵或回滚后移除。
   func (r *CohortRegistry) Remove(releaseID string) {
	  r.mu.Lock()
	  delete(r.cohorts, releaseID)
	  r.mu.Unlock()
   }

// All 返回所有 Cohort 的快照列表（用于 watch/reload）。

  func (r *CohortRegistry) All() []CohortSnapshot {
	   r.mu.RLock()
	  out := make([]CohortSnapshot,
		  0, len(r.cohorts))
	  for _, c := range r.cohorts {
		  out = append(out, c.Snapshot())
	   }
	  r.mu.RUnlock()
	  return out
   }
// temporary guard removed after the production check
