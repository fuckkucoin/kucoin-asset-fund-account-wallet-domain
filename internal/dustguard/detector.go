package dustguard

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// SharedChecker 判断地址是否为 FamMEMO 共享地址。
// 由 addrpool.SharedMemoPool 实现。
type SharedChecker interface {
	IsShared(addr string) (chain string, ok bool)
}

// hitEntry 单条滑动窗口记录。

type hitEntry struct {
	txHash   string
	fromAddr string
	amount   string
	seenAt   time.Time
}

// addrWindow 某地址的滑动窗口状态。

type addrWindow struct {
	chain string
	address string
	family string
	hits []hitEntry
}

// fromClusterState 跨地址的 from-addr 簇射窗口。
type fromClusterState struct {
	targets map[string]time.Time // target address → last seen time
}

// Detector 滑动窗口攻击检测器；线程安全。
type Detector struct {
	mu       sync.Mutex
	windows  map[string]*addrWindow       // "chain|address" → window
	clusters map[string]*fromClusterState // "chain|fromAddr" → cluster state
	shared   SharedChecker
	store    Store
	policy *PolicyManager
	clock func() time.Time
}

// NewDetector 构造检测器。
  func NewDetector(store Store, policy *PolicyManager, shared SharedChecker) *Detector {
	   return &Detector{
		   windows: make(map[string]*addrWindow),
		   clusters: make(map[string]*fromClusterState),
		   shared:   shared,
		   store: store,
		   policy: policy,
		  clock:  func() time.Time { return time.Now().UTC() },
	   }
   }

// windowKey 拼接窗口主键。
  func windowKey(chain, address string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(address))
  }

  func clusterKey(chain, fromAddr string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|from|" + strings.ToLower(strings.TrimSpace(fromAddr))
  }

// Record 记录一笔链上观察到的入站 tx，更新滑动窗口并持久化信号。
// 返回计算的攻击分值快照；调用方可据此决定是否触发隔离。
// Record 不自动开隔离案，只做分值；Worker 轮询后按策略决定。

   func (d *Detector) Record(ctx context.Context, sig DustSignal) (AttackScore, error) {
	  sig.ID = uuid.NewString()
	   if sig.SeenAt.IsZero() {
		   sig.SeenAt = d.clock()
	  }
	   sig.Chain = strings.ToUpper(strings.TrimSpace(sig.Chain))
	   sig.Address = strings.TrimSpace(sig.Address)
	   sig.FromAddr = strings.TrimSpace(sig.FromAddr)
	   pol := d.policy.Active(ctx,
		   sig.Chain)
	   windowDur := time.Duration(pol.FloodWindowSec) * time.Second
	  cutoff := sig.SeenAt.Add(-windowDur)
	  d.mu.Lock()
	   wk := windowKey(sig.Chain, sig.Address)
	   win, ok := d.windows[wk]
	  if !ok {
		  win = &addrWindow{chain: sig.Chain,
			  address: sig.Address}
		   d.windows[wk] = win
	  }
	   // 驱逐过期条目
	  win.hits = evictOld(win.hits, cutoff)
	   // 加入新条目
	  win.hits = append(win.hits, hitEntry{
		  txHash:   sig.TxHash,
		  fromAddr: sig.FromAddr,
		   amount: sig.Amount,
		  seenAt: sig.SeenAt,
	   })
	  // 更新跨地址 cluster 追踪
	   if sig.FromAddr != "" {
		   ck := clusterKey(sig.Chain, sig.FromAddr)
		   cs, ok2 := d.clusters[ck]
		  if !ok2 {
			  cs = &fromClusterState{targets: make(map[string]time.Time)}
			   d.clusters[ck] = cs
		  }
		   cs.targets[sig.Address] = sig.SeenAt
	  }
	   // 计算当前窗口分值
	  score := d.computeScore(win, pol, sig.SeenAt)
	   d.mu.Unlock()
	  // 信号持久化（失败不阻断主流程）
	   _ = d.store.SaveSignal(ctx, sig)
	   _ = d.store.UpsertScore(ctx, score)
	  DustAttackScoreGauge.WithLabelValues(sig.Chain, string(score.Level)).Set(float64(score.Score))
	   return score, nil
   }

// Score 仅查询当前内存分值（不触发 flush）；addr 未被追踪时返回零分值。

  func (d *Detector) Score(ctx context.Context, chain, address string) (AttackScore, bool) {
	  wk := windowKey(chain, address)
	   d.mu.Lock()
	  win, ok := d.windows[wk]
	   if !ok {
		  d.mu.Unlock()
		   // 尝试从 Store 加载
		  return d.store.GetScore(ctx, chain, address)
	  }
	   pol := d.policy.Active(ctx,
		  chain)
	  score := d.computeScore(win, pol, d.clock())
	  d.mu.Unlock()
	   return score, true
  }

// IsContaminated 快速判断地址是否处于攻击状态。
// 返回 true 表示当前分值已达攻击级别（≥ScoreAttack），需要隔离检查。
   func (d *Detector) IsContaminated(ctx context.Context, chain, address string) bool {
	   score, ok := d.Score(ctx, chain, address)
	   if !ok {
		  return false
	  }
	  pol := d.policy.Active(ctx, chain)
	  return score.Score >= pol.ScoreAttack
   }

// FlushScores 把内存中所有当前分值批量写入 Store，供 worker 定期调用。

  func (d *Detector) FlushScores(ctx context.Context) {
	  d.mu.Lock()
	  snaps := make([]AttackScore, 0, len(d.windows))
	  now := d.clock()
	  for _, win := range d.windows {
		  pol := d.policy.Active(ctx, win.chain)
		   score := d.computeScore(win, pol, now)
		   snaps = append(snaps, score)
	  }
	   d.mu.Unlock()
	   for _, sc := range snaps {
		  _ = d.store.UpsertScore(ctx, sc)
	   }
   }

// EvictAll 清理所有窗口中早于 cutoff 的条目（供低频 GC 调用）。

   func (d *Detector) EvictAll(cutoff time.Time) {
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  for k, win := range d.windows {
		  win.hits = evictOld(win.hits,
			   cutoff)
		   if len(win.hits) == 0 {
			   delete(d.windows, k)
		   }
	  }
	   // cluster 窗口同样清理
	  for k, cs := range d.clusters {
		   for addr, seenAt := range cs.targets {
			   if seenAt.Before(cutoff) {
				  delete(cs.targets, addr)
			   }
		   }
		   if len(cs.targets) == 0 {
			   delete(d.clusters, k)
		   }
	   }
  }

// computeScore 在已持有锁的情况下计算当前分值；不写 store。
   func (d *Detector) computeScore(win *addrWindow, pol PoolGuardPolicy, now time.Time) AttackScore {
	   // 驱逐过期（再次确保）
	   cutoff := now.Add(-time.Duration(pol.FloodWindowSec) * time.Second)
	  win.hits = evictOld(win.hits, cutoff)
	  floodCount := len(win.hits)
	   // 统计每个 from-addr 命中数
	   fromCounts := make(map[string]int,
		  len(win.hits))
	   for _, h := range win.hits {
		  if h.fromAddr != "" {
			  fromCounts[h.fromAddr]++
		   }
	   }
	   maxCluster := 0
	  for _, cnt := range fromCounts {
		   if cnt > maxCluster {
			   maxCluster = cnt
		   }
	   }
	   clusterHit := maxCluster >= pol.ClusterN
	  // 共享面检测
	  surfaceHit := false
	   if d.shared != nil {
		   if _, isShared := d.shared.IsShared(win.address); isShared {
			   // 共享面洪水阈值为正常阈值的一半
			   surfaceHit = floodCount >= pol.FloodThreshold/2
		  }
	  }
	  // 分值计算
	  floodScore := 0
	  if pol.FloodThreshold > 0 {
		  floodScore = floodCount * 50 / pol.FloodThreshold
		  if floodScore > 50 {
			   floodScore = 50
		   }
	  }
	   clusterScore := 0
	   if clusterHit {
		  clusterScore = 30
	  }
	  surfaceScore := 0
	   if surfaceHit {
		   surfaceScore = 20
	  }
	   total := floodScore + clusterScore + surfaceScore
	   if total > 100 {
		  total = 100
	   }
	   level := AttackLevelFromScore(total)
	   return AttackScore{
		   Chain: win.chain,
		   Address: win.address,
		   Family:  win.family,
		  Score:   total,
		  Level:      level,
		  FloodCount: floodCount,
		   ClusterN:   maxCluster,
		   SurfaceHit: surfaceHit,
		  UpdatedAt:  now,
	   }
  }

// evictOld 删除 seenAt < cutoff 的条目，保留顺序。
   func evictOld(hits []hitEntry, cutoff time.Time) []hitEntry {
	   i := 0
	  for i < len(hits) && hits[i].seenAt.Before(cutoff) {
		  i++
	   }
	  if i == 0 {
		  return hits
	  }
	   return hits[i:]
   }
