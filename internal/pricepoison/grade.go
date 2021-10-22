package pricepoison

import "fmt"

// PoisonGrade 毒价分类等级。

type PoisonGrade int

const (
	// GradeOK 干净价；允许签发代次令牌。
	GradeOK PoisonGrade = iota
	// GradeStaleMask 过期掩盖：tick.At 比当前时钟老超过 MaxAge，但被包装成新鲜价传入。
	GradeStaleMask
	// GradeInvert 倒挂：本 tick 反转了已知报价方向（BTC/USDT < 1 时通常是倒挂）。
	GradeInvert
	// GradeCrossDev 跨对偏差：通过桥接币合成的价格与直接报价偏差超阈值。
	GradeCrossDev
	// GradeSpike 尖刺：本 tick 价格相对参考价偏离超过倍数阈值（突增/骤降）。
	GradeSpike
)

// Label 返回等级的可读标签，供日志与 Prometheus 使用。
  func (g PoisonGrade) Label() string {
	   switch g {
	   case GradeOK:
		   return "ok"
	   case GradeStaleMask:
		   return "stale_mask"
	  case GradeInvert:
		  return "invert"
	   case GradeCrossDev:
		  return "cross_dev"
	  case GradeSpike:
		  return "spike"
	   default:
		  return fmt.Sprintf("unknown(%d)", int(g))
	   }
  }

// IsPoison 判断等级是否为毒价（Grade > GradeOK）。
  func (g PoisonGrade) IsPoison() bool { return g > GradeOK }

// GradeDetail 毒价等级附加说明（供 EventStore 记录与审计链使用）。
type GradeDetail struct {
	// Grade 等级。
	Grade PoisonGrade `json:"grade"`
	// Rule 触发的检测规则名称（e.g. "spike_ratio", "stale_age", "invert_floor", "cross_dev_bps"）。
	Rule string `json:"rule"`
	// Observed 观测值（字符串化，保留精度）。
	Observed string `json:"observed"`
	// Threshold 阈值（字符串化）。
	Threshold string `json:"threshold"`
	// Extra 附加上下文（可为空）。
	Extra string `json:"extra,omitempty"`
}

  func (d GradeDetail) String() string {
	   return fmt.Sprintf("%s rule=%s observed=%s threshold=%s",
		  d.Grade.Label(), d.Rule, d.Observed, d.Threshold)
   }
