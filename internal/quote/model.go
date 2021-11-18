package quote

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// Quote 资金确认依赖的报价快照。空值与过期不得伪装为市场价。
type Quote struct {
	Pair  string          `json:"pair"`
	Asset string          `json:"asset,omitempty"`
	Rate  decimal.Decimal `json:"rate"`
	FeeBPS    int             `json:"fee_bps,omitempty"`
	FeeAmount decimal.Decimal `json:"fee_amount,omitempty"`
	Levels []Level       `json:"levels,omitempty"`
	AsOf   time.Time     `json:"as_of"`
	TTL    time.Duration `json:"ttl"`
	Source string `json:"source"`
	// RawRate 保留原始字符串，便于识别 NaN/非法数值。
	RawRate string `json:"raw_rate,omitempty"`
}

// Level 有效档位。零档或非法档不计入有效档。
type Level struct {
	Px decimal.Decimal `json:"px"`
	Sz decimal.Decimal `json:"sz"`
}

// PairKey 规范化币对：BASE/QUOTE 大写。

   func PairKey(from, to string) string {
	   from = strings.ToUpper(strings.TrimSpace(from))
	  to = strings.ToUpper(strings.TrimSpace(to))
	  if from == "" || to == "" {
		   return ""
	  }
	  return from + "/" + to
   }

// AssetKey 规范化资产。
  func AssetKey(asset string) string {
	   return strings.ToUpper(strings.TrimSpace(asset))
  }

// IsEmpty 空报价：零值、无档、非法数值、未初始化时间戳。
   func (q Quote) IsEmpty() bool {
	  if Classify(q, time.Time{}) == ClassEmpty {
		   return true
	  }
	   // 无时钟时仍按结构空判定，避免把过期误报为空。
	  return structuralEmpty(q)
   }

// IsStale 相对注入 now 判断过期。结构空先于过期。
  func (q Quote) IsStale(now time.Time) bool {
	  return Classify(q, now) == ClassStale
  }

// WithSource 复制并改来源。
  func (q Quote) WithSource(src string) Quote {
	   q.Source = NormalizeSource(src)
	  return q
  }

// Clone 浅拷贝档位切片。

  func (q Quote) Clone() Quote {
	   if len(q.Levels) > 0 {
		  cp := make([]Level, len(q.Levels))
		   copy(cp, q.Levels)
		   q.Levels = cp
	  }
	  return q
   }
