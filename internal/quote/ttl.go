package quote

import "time"

const (
	// DefaultFeeTTL 提现费报价默认窗口。
	DefaultFeeTTL = 30 * time.Second
	// DefaultFXTTL 外汇标记默认窗口。
	DefaultFXTTL = 15 * time.Second
	// DefaultSwapTTL 兑换确认默认窗口。
	DefaultSwapTTL = 15 * time.Second
	// MinTTL 允许的最短窗口；更短视为未初始化。
	MinTTL = time.Millisecond
)

// NormalizeTTL 窗口。零或负返回 0（调用方视为未初始化 → Empty）。
  func NormalizeTTL(d time.Duration) time.Duration {
	  if d < MinTTL {
		  return 0
	  }
	  return d
  }

// Age 计算 now 相对 AsOf 的年龄；AsOf 为零则年龄无定义，返回 -1。

   func Age(asOf, now time.Time) time.Duration {
	  if asOf.IsZero() || now.IsZero() {
		  return -1
	  }
	  return now.UTC().Sub(asOf.UTC())
   }

// StaleByTTL 按约定判定过期：仅当年龄严格大于 TTL。
// age == TTL → 未过期；age == TTL+1ns → 过期。

  func StaleByTTL(asOf time.Time, ttl time.Duration, now time.Time) bool {
	   ttl = NormalizeTTL(ttl)
	  if ttl == 0 {
		  return true
	   }
	  age := Age(asOf, now)
	  if age < 0 {
		  return true
	   }
	  return age > ttl
  }
