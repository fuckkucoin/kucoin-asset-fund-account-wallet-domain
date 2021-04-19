package dustconv

import (
	"fmt"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	// DefaultDriftCapBps 默认漂移帽：50 bps（0.5%）。
	// 超过该幅度的上漂视为危险漂移，Convert 必须 fail-closed。
	DefaultDriftCapBps = 50
	// MaxDriftCapBps 允许配置的上限：500 bps（5%）。
	MaxDriftCapBps = 500
)

// QuoteSnapshot 记录 Convert 启动时刻的牌价快照。
// 该快照在 Convert 的整个执行链路中持有；最终执行时须核对实时牌价确认漂移未超帽。
type QuoteSnapshot struct {
	ID    string `json:"id"`
	Asset string `json:"asset"`
	RateUSDT    string    `json:"rate_usdt"`     // 快照牌价
	SnapAt      time.Time `json:"snap_at"`       // 快照时刻
	DriftCapBps int       `json:"drift_cap_bps"` // 漂移帽（基点）
	PolicyGen uint64 `json:"policy_gen"` // 绑定的策略世代
}

// Copy 返回值拷贝。
  func (q QuoteSnapshot) Copy() QuoteSnapshot { return q }

// CheckDrift 核对实时牌价 liveRate 与快照牌价，判断是否超出漂移帽。
// 只对上漂（liveRate > snapRate）做防护；下漂（liveRate <= snapRate）
// 不增加兑出量，不构成超额泄漏。
// 返回 nil 表示漂移在阈值内，可接着执行；返回 ErrDriftExceeded 时 Convert 必须终止。

  func (q QuoteSnapshot) CheckDrift(liveRate string) error {
	   snapRate,
		   err := money.Parse(q.RateUSDT)
	  if err != nil {
		  return ErrSnapshot
	  }
	   live, err := money.Parse(liveRate)
	   if err != nil {
		   return ErrSnapshot
	   }
	  if !live.IsPositive() || !snapRate.IsPositive() {
		   return ErrSnapshot
	   }
	   // 上漂幅度 = (liveRate - snapRate) / snapRate * 10000 bps
	  diff := live.Dec().Sub(snapRate.Dec())
	  if diff.IsNegative() || diff.IsZero() {
		   // 未上漂，安全
		   return nil
	  }
	   capBps := q.DriftCapBps
	  if capBps <= 0 {
		  capBps = DefaultDriftCapBps
	   }
	   bps := diff.Div(snapRate.Dec()).Mul(decimal.NewFromInt(10000))
	   if bps.GreaterThan(decimal.NewFromInt(int64(capBps))) {
		   return fmt.Errorf("%w: live=%s snap=%s drift_bps=%.2f cap=%d",
			   ErrDriftExceeded, liveRate, q.RateUSDT, bps.InexactFloat64(), capBps)
	  }
	   return nil
   }

// NewQuoteSnapshot 创建一个牌价快照。
// id 由调用方提供（通常为 convert idempotency key 的哈希或 UUID）。

   func NewQuoteSnapshot(id, asset, rateUSDT string, capBps int, policyGen uint64) (QuoteSnapshot, error) {
	  if id == "" {
		   return QuoteSnapshot{}, ErrSnapshot
	   }
	  as, err := money.Parse(rateUSDT)
	  if err != nil || !as.IsPositive() {
		  return QuoteSnapshot{}, ErrSnapshot
	   }
	   if capBps <= 0 {
		  capBps = DefaultDriftCapBps
	   }
	   if capBps > MaxDriftCapBps {
		   capBps = MaxDriftCapBps
	  }
	  return QuoteSnapshot{
		   ID:    id,
		   Asset: asset,
		   RateUSDT: rateUSDT,
		  SnapAt: time.Now().UTC(),
		   DriftCapBps: capBps,
		  PolicyGen:   policyGen,
	   }, nil
  }

// Errors 牌价快照相关。

var (
	ErrSnapshot = fmt.Errorf("dust quote snapshot invalid")
	ErrDriftExceeded = fmt.Errorf("dust quote drift exceeded cap")
)
