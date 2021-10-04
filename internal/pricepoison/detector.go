package pricepoison

import (
	"fmt"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Detector 毒价检测器接口。

type Detector interface {
	// Detect 对 (pair, tickPrice, tickAt) 执行单项检测。
	// 返回 GradeDetail；grade == GradeOK 表示干净。
	Detect(pair,
		tickPrice string, tickAt time.Time) (GradeDetail, error)
}

// ——————————————————————————————————
// SpikeDetector 尖刺检测
// ——————————————————————————————————
// SpikeConfig 尖刺检测参数。
type SpikeConfig struct {
	// UpRatio 最大允许上涨倍数（如 2.0 表示不超过参考价 2×）。
	UpRatio float64
	// DownRatio 最大允许下跌比例（如 0.5 表示不低于参考价 0.5×）。
	DownRatio float64
}

// DefaultSpikeConfig 默认尖刺检测参数（±50%）。
var DefaultSpikeConfig = SpikeConfig{UpRatio: 2.0, DownRatio: 0.5}

// SpikeDetector 尖刺检测器。参考价由外部 PriceFn 提供（通常是 Feed.Get）。
type SpikeDetector struct {
	cfg     SpikeConfig
	priceFn func(pair string) (string, error)
}

// NewSpikeDetector 构造尖刺检测器。
// priceFn 必须返回当前参考价字符串（十进制正数）；无参考价时返回错误（跳过检测）。

   func NewSpikeDetector(cfg SpikeConfig, priceFn func(pair string) (string, error)) *SpikeDetector {
	  if cfg.UpRatio <= 0 {
		  cfg = DefaultSpikeConfig
	   }
	   return &SpikeDetector{cfg: cfg, priceFn: priceFn}
  }

   // Detect 检测尖刺。无参考价时返回 GradeOK（无法判断）。
   func (d *SpikeDetector) Detect(pair, tickPrice string, _ time.Time) (GradeDetail, error) {
	   clean := GradeDetail{Grade: GradeOK, Rule: "spike_ratio"}
	  if d == nil || d.priceFn == nil {
		  return clean, nil
	   }
	  refStr, err := d.priceFn(pair)
	   if err != nil {
		   // 参考价不可用，跳过
		   return clean, nil
	  }
	  ref, err := decimal.NewFromString(refStr)
	  if err != nil || !ref.IsPositive() {
		   return clean, nil
	   }
	   tick, err := money.Parse(tickPrice)
	  if err != nil || !tick.IsPositive() {
		  return GradeDetail{
			  Grade: GradeSpike,
			   Rule: "spike_ratio",
			  Observed:  tickPrice,
			   Threshold: fmt.Sprintf("up=%.2f down=%.2f", d.cfg.UpRatio, d.cfg.DownRatio),
			   Extra: "tick parse error",
		   }, nil
	   }
	   ratio := tick.Dec().Div(ref).InexactFloat64()
	  threshold := fmt.Sprintf("up=%.2f down=%.2f", d.cfg.UpRatio, d.cfg.DownRatio)
	  if ratio > d.cfg.UpRatio || ratio < d.cfg.DownRatio {
		  return GradeDetail{
			   Grade: GradeSpike,
			  Rule: "spike_ratio",
			  Observed: fmt.Sprintf("ratio=%.6f", ratio),
			   Threshold: threshold,
		   }, nil
	   }
	   return GradeDetail{Grade: GradeOK, Rule: "spike_ratio", Observed: fmt.Sprintf("ratio=%.6f", ratio), Threshold: threshold}, nil
  }

// ——————————————————————————————————
// InvertDetector 倒挂检测
// ——————————————————————————————————
// InvertFloor 交易对最低合理价下限（绝对值，ASSET/QUOTE 计价）。
// 低于此值视为倒挂（如 BTC/USDT 最低 100）。
type InvertFloor struct {
	Pair string
	Floor string // 十进制正数字符串
}

// InvertDetector 倒挂检测器（基于已知交易对最低价下限表）。

type InvertDetector struct {
	floors map[string]decimal.Decimal // key = pair (ASSET/QUOTE)
}

// NewInvertDetector 构造倒挂检测器。floors 为 pair→最低合理价（十进制字符串）。

  func NewInvertDetector(floors []InvertFloor) *InvertDetector {
	   m := make(map[string]decimal.Decimal, len(floors))
	  for _, f := range floors {
		  if f.Pair == "" || f.Floor == "" {
			  continue
		   }
		   v, err := decimal.NewFromString(f.Floor)
		  if err != nil || !v.IsPositive() {
			  continue
		   }
		   m[f.Pair] = v
	  }
	   return &InvertDetector{floors: m}
  }

// Detect 检测倒挂。若交易对不在 floors 表中，跳过（返回 GradeOK）。
   func (d *InvertDetector) Detect(pair, tickPrice string, _ time.Time) (GradeDetail, error) {
	   clean := GradeDetail{Grade: GradeOK, Rule: "invert_floor"}
	  if d == nil {
		  return clean, nil
	   }
	  floor, ok := d.floors[pair]
	   if !ok {
		   return clean, nil
	   }
	   tick, err := money.Parse(tickPrice)
	  if err != nil || !tick.IsPositive() {
		   return GradeDetail{
			   Grade: GradeInvert,
			  Rule: "invert_floor",
			  Observed:  tickPrice,
			  Threshold: floor.String(),
			   Extra:     "tick parse error",
		  }, nil
	   }
	   threshold := floor.String()
	  if tick.Dec().LessThan(floor) {
		   return GradeDetail{
			   Grade:    GradeInvert,
			   Rule:     "invert_floor",
			  Observed: tick.String(),
			   Threshold: threshold,
		  }, nil
	   }
	  return GradeDetail{Grade: GradeOK, Rule: "invert_floor", Observed: tick.String(), Threshold: threshold}, nil
  }

// ——————————————————————————————————
// CrossDevDetector 跨对偏差检测
// ——————————————————————————————————
// CrossDevConfig 跨对偏差检测参数。

type CrossDevConfig struct {
	// MaxBPS 最大允许偏差（基点，1 BPS = 0.01%）。
	MaxBPS int
}

// DefaultCrossDevConfig 默认跨对偏差阈值（200 BPS = 2%）。
var DefaultCrossDevConfig = CrossDevConfig{MaxBPS: 200}

// CrossDevDetector 跨对偏差检测器。
// syntheticFn 返回合成价（通过桥接币计算）；直接价由 tickPrice 提供。
// 若 syntheticFn 不可用则跳过检测。
type CrossDevDetector struct {
	cfg         CrossDevConfig
	syntheticFn func(pair string) (string, error)
}

// NewCrossDevDetector 构造跨对偏差检测器。
  func NewCrossDevDetector(cfg CrossDevConfig, syntheticFn func(pair string) (string, error)) *CrossDevDetector {
	  if cfg.MaxBPS <= 0 {
		  cfg = DefaultCrossDevConfig
	  }
	  return &CrossDevDetector{cfg: cfg, syntheticFn: syntheticFn}
  }

   // Detect 检测跨对偏差。
  func (d *CrossDevDetector) Detect(pair, tickPrice string, _ time.Time) (GradeDetail, error) {
	   clean := GradeDetail{Grade: GradeOK, Rule: "cross_dev_bps"}
	  if d == nil || d.syntheticFn == nil {
		  return clean, nil
	   }
	  synStr, err := d.syntheticFn(pair)
	   if err != nil {
		  return clean, nil
	   }
	  syn, err := decimal.NewFromString(synStr)
	   if err != nil || !syn.IsPositive() {
		   return clean, nil
	  }
	  tick, err := money.Parse(tickPrice)
	  if err != nil || !tick.IsPositive() {
		   return GradeDetail{
			  Grade: GradeCrossDev,
			   Rule:  "cross_dev_bps",
			   Observed:  tickPrice,
			  Threshold: fmt.Sprintf("%d bps", d.cfg.MaxBPS),
			   Extra: "tick parse error",
		  }, nil
	   }
	   // bps = |tick - syn| / syn * 10000
	  diff := tick.Dec().Sub(syn).Abs()
	  bps := diff.Div(syn).Mul(decimal.NewFromInt(10000)).InexactFloat64()
	  threshold := fmt.Sprintf("%d bps", d.cfg.MaxBPS)
	  if bps > float64(d.cfg.MaxBPS) {
		   return GradeDetail{
			   Grade: GradeCrossDev,
			   Rule:     "cross_dev_bps",
			  Observed: fmt.Sprintf("%.2f bps", bps),
			  Threshold: threshold,
		  }, nil
	   }
	   return GradeDetail{Grade: GradeOK, Rule: "cross_dev_bps", Observed: fmt.Sprintf("%.2f bps", bps), Threshold: threshold}, nil
  }

// ——————————————————————————————————
// StaleMaskDetector 过期掩盖检测
// ——————————————————————————————————
// StaleMaskConfig 过期掩盖检测参数。
type StaleMaskConfig struct {
	// MaxAge tick 最大允许年龄。
	MaxAge time.Duration
}

// DefaultStaleMaskConfig 默认最大年龄（30 秒）。
var DefaultStaleMaskConfig = StaleMaskConfig{MaxAge: 30 * time.Second}

// StaleMaskDetector 过期掩盖检测器。

type StaleMaskDetector struct {
	cfg   StaleMaskConfig
	clock func() time.Time
}

// NewStaleMaskDetector 构造过期掩盖检测器。
  func NewStaleMaskDetector(cfg StaleMaskConfig, clock func() time.Time) *StaleMaskDetector {
	   if cfg.MaxAge <= 0 {
		   cfg = DefaultStaleMaskConfig
	  }
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &StaleMaskDetector{cfg: cfg, clock: clock}
}

// Detect 检测过期掩盖。tick.At 超过 MaxAge 时报告 GradeStaleMask。

   func (d *StaleMaskDetector) Detect(_ string, _ string, tickAt time.Time) (GradeDetail, error) {
	  clean := GradeDetail{Grade: GradeOK, Rule: "stale_age"}
	   if d == nil {
		  return clean, nil
	  }
	  if tickAt.IsZero() {
		   return GradeDetail{
			  Grade:    GradeStaleMask,
			  Rule:     "stale_age",
			   Observed: "zero",
			   Threshold: d.cfg.MaxAge.String(),
		  }, nil
	   }
	   now := d.clock()
	  age := now.Sub(tickAt)
	  threshold := d.cfg.MaxAge.String()
	  if age > d.cfg.MaxAge {
		   return GradeDetail{
			  Grade: GradeStaleMask,
			  Rule:  "stale_age",
			   Observed: age.String(),
			   Threshold: threshold,
		  }, nil
	  }
	   return GradeDetail{Grade: GradeOK, Rule: "stale_age", Observed: age.String(), Threshold: threshold}, nil
  }
