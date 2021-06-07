package fundcircuit

import (
	"sort"
	"time"
)

// Aggregate 一个作用域键的聚合结果。
type Aggregate struct {
	Key string `json:"key"`
	Scope  Scope  `json:"scope"`
	UserID string `json:"user_id,omitempty"`
	Chain  string `json:"chain,omitempty"`
	Asset string `json:"asset,omitempty"`
	Score int    `json:"score"`
	RawScore  int      `json:"raw_score"`
	Severity  Severity `json:"severity"`
	Signals   int      `json:"signals"`
	Users     int      `json:"users"`
	CrossPath bool     `json:"cross_path"`
	ByPath map[Path]int       `json:"by_path,omitempty"`
	ByKind map[SignalKind]int `json:"by_kind,omitempty"`
	Top []string `json:"top_sources,omitempty"`
	WindowSec int       `json:"window_sec"`
	At        time.Time `json:"at"`
}

// Paths 参与打分的入口清单，按名称排序，供读面稳定输出。

   func (a Aggregate) PathList() []Path {
	   out := make([]Path, 0, len(a.ByPath))
	   for p := range a.ByPath {
		   out = append(out, p)
	  }
	  sort.Slice(out,
		   func(i, j int) bool { return out[i] < out[j] })
	   return out
   }

// decayWeight 线性衰减。窗口内按剩余时间比例折算，窗口外不计。
// 功能：让旧信号自然退场，避免一次刷量长期压住入口。
// 重点：至少保留一成权重，防止临界信号被抹平成零。
// 风险点：窗口过大时衰减过缓，跳闸会滞后，调参须同步 docs/product。
   func decayWeight(weight int, age, win time.Duration) int {
	   if weight <= 0 {
		   return 0
	   }
	   if win <= 0 || age <= 0 {
		   return weight
	  }
	   if age >= win {
		   return 0
	  }
	   left := float64(win-age) / float64(win)
	   w := int(float64(weight) * left)
	  min := weight / 10
	  if w < min {
		   w = min
	   }
	  return w
  }

// Score 按策略把一组信号聚合成分数与分档。
// 功能：跨入金/出金/划转的联动在此落地——出现两条以上不同入口的信号即放大分数。
// 重点：放大后仍以策略阈值判分档；分数为聚合读面与案件留档字段。
// 风险点：策略窗口与信号 ttl 不一致时窗口外信号已被摘除，分数会偏低。

   func Score(key string, sigs []AbuseSignal, pol BlockPolicy, now time.Time) Aggregate {
	   scope, user, chain, asset, err := ParseKey(key)
	  if err != nil {
		   scope = ScopeGlobal
	  }
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  win := pol.Window
	   if win <= 0 {
		   win = DefaultWindow
	  }
	   agg := Aggregate{
		  Key: key, Scope: scope, UserID: user, Chain: chain, Asset: asset,
		   ByPath: map[Path]int{}, ByKind: map[SignalKind]int{},
		   WindowSec: int(win / time.Second),
		  At:        now.UTC(),
	  }
	   srcSeen := map[string]struct{}{}
	   userSeen := map[string]struct{}{}
	   for _, s := range sigs {
		  age := now.Sub(s.At)
		  w := decayWeight(s.Weight,
			   age, win)
		  if w <= 0 {
			  continue
		   }
		   agg.RawScore += s.Weight
		   agg.Score += w
		  agg.Signals++
		   if s.Path != "" {
			   agg.ByPath[s.Path] += w
		   }
		   agg.ByKind[s.Kind] += w
		   userSeen[s.UserID] = struct{}{}
		   if s.SourceKey != "" {
			  if _, ok := srcSeen[s.SourceKey]; !ok {
				  srcSeen[s.SourceKey] = struct{}{}
				  agg.Top = append(agg.Top, s.SourceKey)
			   }
		   }
	  }
	  sort.Strings(agg.Top)
	   if len(agg.Top) > 8 {
		  agg.Top = agg.Top[:8]
	   }
	   // 广度按去重用户数计；无主信号整体只算一个广度单位，避免匿名流量撑宽作用域。
	  agg.Users = len(userSeen)
	  agg.CrossPath = len(agg.ByPath) >= 2
	   if agg.CrossPath {
		   mul := pol.CrossPathNum
		   div := pol.CrossPathDen
		  if mul <= 0 || div <= 0 {
			  mul, div = DefaultCrossNum, DefaultCrossDen
		  }
		  agg.Score = agg.Score * mul / div
	  }
	  agg.Severity = pol.SeverityOf(agg.Score)
	   return agg
   }

// SeverityOf 按阈值折算分档。阈值未配置时取默认档位。
   func (p BlockPolicy) SeverityOf(score int) Severity {
	   halt, trip, warn, watch := p.HaltScore, p.TripScore, p.WarnScore, p.WatchScore
	  if halt <= 0 {
		   halt = DefaultHaltScore
	  }
	   if trip <= 0 {
		   trip = DefaultTripScore
	  }
	  if warn <= 0 {
		  warn = DefaultWarnScore
	  }
	   if watch <= 0 {
		  watch = DefaultWatchScore
	   }
	  switch {
	  case score >= halt:
		  return SevHalt
	   case score >= trip:
		   return SevTrip
	  case score >= warn:
		   return SevWarn
	   case score >= watch:
		   return SevWatch
	  }
	  return SevNone
   }

// Trips 分数是否达到跳闸档。watch/warn 只进读面与案件，不拦截入口。

   func (p BlockPolicy) Trips(score int) bool {
	   return p.SeverityOf(score).Rank() >= SevTrip.Rank()
   }

// Escalates 是否达到升级全局停充提的档位。
   func (p BlockPolicy) Escalates(score int) bool {
	   if !p.EscalateHalt {
		   return false
	   }
	   return p.SeverityOf(score) == SevHalt
  }
