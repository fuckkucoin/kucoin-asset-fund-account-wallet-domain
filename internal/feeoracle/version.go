package feeoracle

import (
	"strings"
	"time"
)

// VersionState 报价代次的生命周期状态。
type VersionState string

const (
	// StateActive 刚创建，可被绑定，未过期。
	StateActive VersionState = "active"
	// StateExpired 已超过 ExpiresAt，拒绝绑定和使用。
	StateExpired VersionState = "expired"
	// StateBound 已绑定到某个出金单，等待广播确认。
	StateBound VersionState = "bound"
	// StateSettled 广播后确认实际费用已对账入库（只读）。
	StateSettled VersionState = "settled"
)

// QuoteVersion 出金预检时绑定的手续费报价快照。一旦创建不得修改字段值。

type QuoteVersion struct {
	// ID 全局唯一代次标识（UUID）。
	ID string `json:"id"`
	// WithdrawID 绑定的出金单号；StateActive 时为空，StateBound 后非空。
	WithdrawID string `json:"withdraw_id,omitempty"`
	// Chain 报价时的链名。
	Chain string `json:"chain"`
	// Family 链族。
	Family Family `json:"family"`
	// Asset 用户资产代码（报价计价币）。
	Asset string `json:"asset"`
	// Gross 用户出金总额（用户资产计价）。
	Gross string `json:"gross"`
	// QuoteFee 此代次的估算手续费（用户资产计价）。
	QuoteFee string `json:"quote_fee"`
	// QuoteNet 此代次的估算到账额（用户资产计价）：Gross - QuoteFee。
	QuoteNet string `json:"quote_net"`
	// Tick 产生此代次时所用的聚合 tick 快照。
	Tick ChainFeeTick `json:"tick"`
	// ScheduleID 配合的 feeeng Schedule 标识（可为空，表示纯动态报价）。
	ScheduleID string `json:"schedule_id,omitempty"`
	// BPS 生效的 BPS（来自 Schedule 或 0 表示纯链费）。
	BPS int `json:"bps"`
	// State 当前生命周期状态。
	State VersionState `json:"state"`
	// ExpiresAt 过此时间代次失效，必须重新报价。
	ExpiresAt time.Time `json:"expires_at"`
	// BoundAt 绑定出金单的时间。
	BoundAt *time.Time `json:"bound_at,omitempty"`
	// CreatedAt 代次创建时间。
	CreatedAt time.Time `json:"created_at"`
}

// IsExpired 检查代次是否已过期（可传入自定义 now 以便）。
  func (v *QuoteVersion) IsExpired(now time.Time) bool {
	  if v == nil {
		   return true
	   }
	  return !now.Before(v.ExpiresAt)
  }

// CanBind 检查代次是否处于可绑定状态（StateActive 且未过期）。
  func (v *QuoteVersion) CanBind(now time.Time) error {
	   if v == nil {
		   return ErrVersionNotFound
	  }
	   if v.IsExpired(now) {
		   return ErrVersionExpired
	   }
	  switch v.State {
	   case StateActive:
		   return nil
	   case StateBound:
		   return ErrVersionBound
	   case StateExpired:
		  return ErrVersionExpired
	   case StateSettled:
		  return ErrVersionSettled
	   default:
		   return ErrVersionNotFound
	  }
  }

// WithdrawMatches 检查代次绑定的出金单号是否匹配（已绑定时校验一致性）。
  func (v *QuoteVersion) WithdrawMatches(withdrawID string) bool {
	   if v == nil {
		   return false
	  }
	   wid := strings.TrimSpace(withdrawID)
	  if v.WithdrawID == "" || v.WithdrawID == wid {
		   return true
	  }
	  return false
  }

// DefaultQuoteTTL 报价代次默认有效期。出金预检到广播提交必须在此窗口内完成。
// 值 10 分钟；在出金队列拥堵时应拒绝旧代次并强制重新报价。
const DefaultQuoteTTL = 10 * time.Minute

// ShortQuoteTTL 高波动期（熔断刚恢复）建议的短有效期。

const ShortQuoteTTL = 3 * time.Minute
