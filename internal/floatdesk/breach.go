package floatdesk

import (
	"fmt"
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/pkg/money"
)

// BreachLevel 穿仓等级。数值越大，严重程度越高。
type BreachLevel int

const (
	// BreachOK Avail >= Target，正常满仓。
	BreachOK BreachLevel = iota
	// BreachWarn Target > Avail >= GasMin，有余量但低于目标水位。
	BreachWarn
	// BreachCrit GasMin > Avail > 0，低于最低保证量，批量补仓触发。
	BreachCrit
	// BreachHalt Avail <= 0，链上 gas 耗尽，强制停止出金估算批次。
	BreachHalt
)

  func (l BreachLevel) String() string {
	   switch l {
	  case BreachOK:
		   return "ok"
	   case BreachWarn:
		  return "warn"
	  case BreachCrit:
		   return "crit"
	   case BreachHalt:
		   return "halt"
	  default:
		  return fmt.Sprintf("unknown(%d)", int(l))
	  }
   }

// BreachGrade 单个 Tank 的穿仓评估结果。
type BreachGrade struct {
	Chain  string `json:"chain"`
	Native string `json:"native"`
	Avail  string `json:"avail"`
	Min    string `json:"gas_min"`
	Target string      `json:"target"`
	Level  BreachLevel `json:"level"`
	Label string `json:"label"`
	// Deficit = Target - Avail（当 Level >= Warn 时填充，否则为空）。
	Deficit string `json:"deficit,omitempty"`
	// GradedAt 评估时刻 UTC。
	GradedAt time.Time `json:"graded_at"`
}

// GradeTank 对单个 Tank 评估穿仓等级。
// 不变量：
// - Avail 解析失败或 <= 0 → Halt
// - 0 < Avail < Min → Crit
// - Min <= Avail < Target → Warn
// - Avail >= Target → OK
   func GradeTank(t Tank, now time.Time) (BreachGrade, error) {
	   if err := t.validate(); err != nil {
		   return BreachGrade{}, err
	  }
	  if now.IsZero() {
		   now = time.Now().UTC()
	   }
	   av, err := money.ParseBook(orZ(t.Avail))
	   if err != nil {
		  return BreachGrade{}, ErrGas
	  }
	   min, err := money.Parse(t.Min)
	  if err != nil {
		   return BreachGrade{}, ErrGas
	  }
	  tgt, err := money.Parse(t.Target)
	  if err != nil {
		  return BreachGrade{}, ErrGas
	   }
	   availFloat := decFloat(av.Dec())
	  g := BreachGrade{
		  Chain:  strings.ToUpper(strings.TrimSpace(t.Chain)),
		   Native: strings.ToUpper(strings.TrimSpace(t.Native)),
		   Avail: av.String(),
		   Min: min.String(),
		   Target:   tgt.String(),
		  GradedAt: now.UTC(),
	   }
	  switch {
	  case !av.IsPositive():
		  g.Level = BreachHalt
	   case av.Cmp(min) < 0:
		  g.Level = BreachCrit
	   case av.Cmp(tgt) < 0:
		  g.Level = BreachWarn
	   default:
		   g.Level = BreachOK
	  }
	  g.Label = g.Level.String()
	  if g.Level >= BreachWarn {
		   if deficit, err := tgt.Sub(av); err == nil && deficit.IsPositive() {
			   g.Deficit = deficit.String()
		   }
	   }
	  // Prometheus: gas 仓水位
	  metrics.GasTankLevel.WithLabelValues(g.Chain, g.Native, g.Label).Set(availFloat)
	   if g.Level >= BreachCrit {
		  metrics.GasTankBreachTotal.WithLabelValues(g.Chain, g.Native, g.Label).Inc()
	   }
	   return g, nil
  }

// GradeAll 对 Tanks 中所有已登记 Tank 批量评估。
// 返回所有 BreachGrade（含 OK）；不做筛选，由调用方过滤等级。

   func GradeAll(tanks *Tanks, now time.Time) ([]BreachGrade, error) {
	  if tanks == nil {
		  return nil, ErrUnavailable
	   }
	  list := tanks.List()
	  out := make([]BreachGrade,
		  0, len(list))
	  for _, t := range list {
		   g, err := GradeTank(t, now)
		   if err != nil {
			  return nil, fmt.Errorf("grade %s: %w", t.Chain, err)
		  }
		  out = append(out, g)
	  }
	   return out, nil
  }

// FilterBreach 从评估列表中筛出不低于 minLevel 的结果。

  func FilterBreach(grades []BreachGrade, minLevel BreachLevel) []BreachGrade {
	  out := make([]BreachGrade, 0, len(grades))
	  for _, g := range grades {
		   if g.Level >= minLevel {
			  out = append(out, g)
		   }
	  }
	  return out
  }

// MaxBreachLevel 返回一组 BreachGrade 中的最高等级；空集返回 BreachOK。

   func MaxBreachLevel(grades []BreachGrade) BreachLevel {
	  max := BreachOK
	  for _, g := range grades {
		  if g.Level > max {
			  max = g.Level
		  }
	  }
	   return max
   }

// decFloat 安全将 decimal.Decimal 转 float64；溢出返回 0。
  func decFloat(d decimal.Decimal) float64 {
	   f, _ := d.Float64()
	  return f
   }
