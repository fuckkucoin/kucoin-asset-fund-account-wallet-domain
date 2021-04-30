package feeoracle

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
	"github.com/shopspring/decimal"
)

// SourceKind oracle 源类型标识。
type SourceKind string

const (
	SourceHTTP SourceKind = "http"
	SourceKafka  SourceKind = "kafka"
	SourceStatic SourceKind = "static"
	SourceInternal SourceKind = "internal"
)

// SourceConfig 单个 fee 源的配置。
type SourceConfig struct {
	// ID 源的唯一标识符。
	ID string `json:"id"`
	// Name 可读名称，用于日志和 ops 面板。
	Name string `json:"name"`
	// Kind 源类型。
	Kind SourceKind `json:"kind"`
	// Chain 该源负责的链名。
	Chain string `json:"chain"`
	// Family 链族。
	Family Family `json:"family"`
	// Endpoint HTTP 源的 URL；Kafka 源的 topic；Static 源留空。
	Endpoint string `json:"endpoint,omitempty"`
	// Weight 聚合权重（1–100）；不填默认 1。
	Weight int `json:"weight"`
	// Enabled 是否启用；false 表示该源被禁用，不参与聚合。
	Enabled bool `json:"enabled"`
	// StaleDur 超过此时间未更新则视为 stale。
	StaleDur time.Duration `json:"stale_dur"`
}

// ChainFeeTick 单一来源或聚合后的链费率 tick。
// 存放原始参数与规范化结果，供 QuoteVersionStore 快照使用。
type ChainFeeTick struct {
	// SourceID 来源 source_id；聚合 tick 填 "aggregated"。
	SourceID string `json:"source_id"`
	// Chain 链名。
	Chain string `json:"chain"`
	// Family 链族。
	Family Family `json:"family"`
	// Input 原始链费率输入；来自源推送。
	Input ChainFeeInput `json:"input"`
	// Normalized 规范化后的原生费用。
	Normalized NormalizedFee `json:"normalized"`
	// TickedAt 该 tick 的产生时间（来自源方）。
	TickedAt time.Time `json:"ticked_at"`
	// RecordedAt 本节点收到并记录的时间。
	RecordedAt time.Time `json:"recorded_at"`
}

// sourceTick 内部保存的单源 tick 记录。

type sourceTick struct {
	cfg  SourceConfig
	tick ChainFeeTick
}

// Oracle 多链费率 oracle：聚合多源 tick，执行 stale 与偏差检查。
type Oracle struct {
	mu sync.RWMutex
	// sources 所有已注册的源配置，按 source_id 索引。
	sources map[string]SourceConfig
	// ticks 最新的单源 tick，按 "chain|source_id" 索引。
	ticks map[string]sourceTick
	// DeviationBPS 允许的最大来源间偏差，单位 BPS（万分之一）；超过则熔断。
	// 典型值：500（5%）。0 表示不检查偏差。
	DeviationBPS int
	// clock 时钟函数，时可替换。
	clock func() time.Time
}

// NewOracle 创建一个新的 Oracle，设置偏差阈值（BPS）。

   func NewOracle(deviationBPS int) *Oracle {
	  return &Oracle{
		   sources: map[string]SourceConfig{},
		   ticks:        map[string]sourceTick{},
		   DeviationBPS: deviationBPS,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 替换时钟函数（用）。
  func (o *Oracle) WithClock(c func() time.Time) *Oracle {
	   if o != nil && c != nil {
		   o.clock = c
	   }
	   return o
  }

  // RegisterSource 注册一个 fee 源。若 ID 已存在则覆盖。
   func (o *Oracle) RegisterSource(cfg SourceConfig) error {
	  if o == nil {
		   return ErrUnavailable
	  }
	   if strings.TrimSpace(cfg.ID) == "" {
		  return fmt.Errorf("%w: source id required", ErrSource)
	   }
	   if strings.TrimSpace(cfg.Chain) == "" {
		  return fmt.Errorf("%w: source chain required", ErrSource)
	  }
	   if cfg.Family == "" {
		  return fmt.Errorf("%w: source family required", ErrSource)
	  }
	  if cfg.Weight <= 0 {
		  cfg.Weight = 1
	   }
	  if cfg.StaleDur <= 0 {
		  cfg.StaleDur = 120 * time.Second
	  }
	   o.mu.Lock()
	   o.sources[cfg.ID] = cfg
	   o.mu.Unlock()
	   return nil
   }

// Sources 返回所有已注册源的拷贝列表。
  func (o *Oracle) Sources() []SourceConfig {
	  if o == nil {
		  return nil
	   }
	  o.mu.RLock()
	   defer o.mu.RUnlock()
	  out := make([]SourceConfig, 0, len(o.sources))
	  for _, s := range o.sources {
		   out = append(out, s)
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	   return out
   }

// PushTick 接收来自某个源的 tick 并存储。
// sourceID 必须已通过 RegisterSource 注册且 Enabled=true。

  func (o *Oracle) PushTick(sourceID string, tick ChainFeeTick) error {
	   if o == nil {
		   return ErrUnavailable
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	  cfg, ok := o.sources[sourceID]
	   if !ok {
		  return fmt.Errorf("%w: source %q not registered", ErrSource, sourceID)
	   }
	   if !cfg.Enabled {
		  return fmt.Errorf("%w: source %q is disabled", ErrSource, sourceID)
	   }
	  now := o.clock()
	  if tick.RecordedAt.IsZero() {
		  tick.RecordedAt = now
	  }
	  if tick.TickedAt.IsZero() {
		   tick.TickedAt = now
	   }
	  tick.SourceID = sourceID
	  tick.Chain = cfg.Chain
	  tick.Family = cfg.Family
	  o.ticks[tickKey(cfg.Chain, sourceID)] = sourceTick{cfg: cfg, tick: tick}
	  return nil
  }

// GetAggregated 返回指定链的聚合 tick。
// stale 源自动排除；若所有源 stale 则返回 ErrStale；
// 若无源配置则返回 ErrNoSources；
// 若偏差超过阈值则返回 ErrDeviation。
  func (o *Oracle) GetAggregated(chain string) (ChainFeeTick, error) {
	   if o == nil {
		  return ChainFeeTick{}, ErrUnavailable
	   }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   o.mu.RLock()
	   defer o.mu.RUnlock()
	  // 收集该链的所有活跃源。
	   var activeSources []sourceTick
	   for _, st := range o.ticks {
		  if strings.ToUpper(st.cfg.Chain) != chain {
			   continue
		  }
		   if !st.cfg.Enabled {
			  continue
		  }
		  activeSources = append(activeSources,
			  st)
	   }
	  // 没有源配置（非 stale，是真没有）。
	  if len(activeSources) == 0 {
		   // 检查是否有已注册但未推过 tick 的源。
		  hasCfg := false
		   for _, cfg := range o.sources {
			  if strings.ToUpper(cfg.Chain) == chain && cfg.Enabled {
				   hasCfg = true
				   break
			   }
		   }
		  if !hasCfg {
			  return ChainFeeTick{}, fmt.Errorf("%w: chain=%s", ErrNoSources, chain)
		   }
		  return ChainFeeTick{}, fmt.Errorf("%w: chain=%s no ticks received yet", ErrMissing, chain)
	   }
	  now := o.clock()
	  // 过滤 stale tick。
	   var fresh []sourceTick
	  for _, st := range activeSources {
		   age := now.Sub(st.tick.RecordedAt)
		  if age <= st.cfg.StaleDur {
			  fresh = append(fresh, st)
		   }
	  }
	   if len(fresh) == 0 {
		   return ChainFeeTick{}, fmt.Errorf("%w: chain=%s all sources stale", ErrStale, chain)
	  }
	   // 偏差检查：比较所有存活源的 NativeAmount。
	   if o.DeviationBPS > 0 && len(fresh) > 1 {
		   if err := checkDeviation(fresh, o.DeviationBPS); err != nil {
			   return ChainFeeTick{}, err
		   }
	   }
	  // 权重加权平均 NativeAmount。
	  aggregated := weightedAggregate(fresh)
	   aggregated.SourceID = "aggregated"
	   aggregated.RecordedAt = now
	  return aggregated, nil
   }

// AllTicks 返回所有已存储 tick 的拷贝（调试/ops 用）。
  func (o *Oracle) AllTicks() []ChainFeeTick {
	   if o == nil {
		  return nil
	   }
	   o.mu.RLock()
	  defer o.mu.RUnlock()
	   out := make([]ChainFeeTick, 0, len(o.ticks))
	   for _, st := range o.ticks {
		  out = append(out, st.tick)
	  }
	  sort.Slice(out, func(i, j int) bool {
		  if out[i].Chain != out[j].Chain {
			  return out[i].Chain < out[j].Chain
		  }
		   return out[i].SourceID < out[j].SourceID
	   })
	  return out
   }

// HealthReport 返回 oracle 各链健康状态摘要。

  func (o *Oracle) HealthReport() OracleHealth {
	   if o == nil {
		  return OracleHealth{Healthy: false, Reason: "oracle nil"}
	  }
	  o.mu.RLock()
	  defer o.mu.RUnlock()
	   now := o.clock()
	   chainMap := map[string]*ChainHealth{}
	  for _, st := range o.ticks {
		  chain := strings.ToUpper(st.cfg.Chain)
		  ch, ok := chainMap[chain]
		   if !ok {
			   ch = &ChainHealth{Chain: chain,
				  Family: string(st.cfg.Family)}
			  chainMap[chain] = ch
		   }
		  ch.TotalSources++
		  age := now.Sub(st.tick.RecordedAt)
		   src := SourceHealth{
			   SourceID: st.cfg.ID,
			  Name: st.cfg.Name,
			  Age:   age,
			   Stale: age > st.cfg.StaleDur,
		  }
		  if !src.Stale {
			   ch.FreshSources++
		   }
		   ch.Sources = append(ch.Sources, src)
	  }
	   chains := make([]ChainHealth, 0, len(chainMap))
	   allHealthy := true
	   for _, ch := range chainMap {
		   if ch.FreshSources == 0 {
			   ch.Stale = true
			   allHealthy = false
		   }
		  chains = append(chains, *ch)
	   }
	   sort.Slice(chains, func(i, j int) bool { return chains[i].Chain < chains[j].Chain })
	  reason := ""
	  if !allHealthy {
		  reason = "one or more chains have no fresh sources"
	  }
	   return OracleHealth{Healthy: allHealthy, Chains: chains, Reason: reason, At: now}
   }

// OracleHealth oracle 整体健康快照。
type OracleHealth struct {
	Healthy bool   `json:"healthy"`
	Reason  string `json:"reason,omitempty"`
	Chains []ChainHealth `json:"chains"`
	At time.Time `json:"at"`
}

// ChainHealth 单链健康快照。

type ChainHealth struct {
	Chain  string `json:"chain"`
	Family string `json:"family"`
	TotalSources int `json:"total_sources"`
	FreshSources int  `json:"fresh_sources"`
	Stale        bool `json:"stale"`
	Sources []SourceHealth `json:"sources"`
}

// SourceHealth 单源健康快照。

type SourceHealth struct {
	SourceID string        `json:"source_id"`
	Name     string        `json:"name"`
	Age      time.Duration `json:"age_ns"`
	Stale bool `json:"stale"`
}

// tickKey 按链和源组合索引键。
  func tickKey(chain, sourceID string) string {
	  return strings.ToUpper(chain) + "|" + sourceID
  }

// checkDeviation 校验所有存活源间偏差是否在允许范围内。
// 偏差定义：(max - min) / min > DeviationBPS / 10000。
// 任意两源偏差超过阈值即返回 ErrDeviation。
  func checkDeviation(fresh []sourceTick, thresholdBPS int) error {
	  amounts := make([]decimal.Decimal, 0, len(fresh))
	   for _, st := range fresh {
		  if !st.tick.Normalized.NativeAmount.IsZero() {
			  amounts = append(amounts,
				  st.tick.Normalized.NativeAmount)
		   }
	   }
	  if len(amounts) < 2 {
		  return nil
	   }
	   minAmt := amounts[0]
	  maxAmt := amounts[0]
	   for _, a := range amounts[1:] {
		   if a.Cmp(minAmt) < 0 {
			   minAmt = a
		   }
		  if a.Cmp(maxAmt) > 0 {
			   maxAmt = a
		   }
	  }
	  if minAmt.IsZero() {
		  // 如果 min 为零，无法计算百分比偏差；若 max 非零则视为偏差无穷大。
		  if !maxAmt.IsZero() {
			   return fmt.Errorf("%w: min=0 max=%s", ErrDeviation, maxAmt)
		  }
		  return nil
	   }
	   // deviation = (max - min) / min × 10000 BPS
	   diff := maxAmt.Sub(minAmt)
	  bps := diff.Div(minAmt).Mul(decimal.NewFromInt(10000))
	  threshold := decimal.NewFromInt(int64(thresholdBPS))
	   if bps.Cmp(threshold) > 0 {
		   return fmt.Errorf("%w: deviation=%s BPS > threshold=%d BPS", ErrDeviation, bps.StringFixed(2), thresholdBPS)
	   }
	  return nil
  }

// weightedAggregate 对存活源按权重加权平均，合并 ChainFeeTick。
// NativeAmount 加权平均；Input 采用权重最大的源（代表性最强）；
// TickedAt 取最新值。
   func weightedAggregate(fresh []sourceTick) ChainFeeTick {
	   // 找权重最大的源作为 Input/Family 代表。
	   best := fresh[0]
	   for _, st := range fresh[1:] {
		  if st.cfg.Weight > best.cfg.Weight {
			  best = st
		   }
	   }
	   // 权重加权平均 NativeAmount。
	  totalWeight := big.NewInt(0)
	  sumAmount := decimal.Zero
	  latestTick := fresh[0].tick.TickedAt
	   for _, st := range fresh {
		   w := int64(st.cfg.Weight)
		  if w <= 0 {
			  w = 1
		  }
		  wDec := decimal.NewFromInt(w)
		   sumAmount = sumAmount.Add(st.tick.Normalized.NativeAmount.Mul(wDec))
		   totalWeight.Add(totalWeight,
			  big.NewInt(w))
		   if st.tick.TickedAt.After(latestTick) {
			  latestTick = st.tick.TickedAt
		   }
	   }
	   var avgAmount decimal.Decimal
	   tw := decimal.NewFromBigInt(totalWeight, 0)
	   if !tw.IsZero() {
		   avgAmount = sumAmount.Div(tw)
	   }
	  norm := best.tick.Normalized
	  norm.NativeAmount = avgAmount
	  norm.RawDetail = fmt.Sprintf("aggregated from %d sources, total_weight=%s", len(fresh), tw)
	  return ChainFeeTick{
		   Chain: best.cfg.Chain,
		   Family: best.cfg.Family,
		  Input:      best.tick.Input,
		  Normalized: norm,
		   TickedAt:   latestTick,
	   }
  }
