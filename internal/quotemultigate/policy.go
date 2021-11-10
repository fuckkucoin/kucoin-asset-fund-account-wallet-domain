package quotemultigate

import (
	"strings"
	"time"
)

// 默认价差阈（基点）与采集窗口。数值为资金确认策略常数，非压测结果。
const (
	DefaultMaxSpreadBPS = 30
	DefaultMinVenues = 2
	DefaultDeadline = 800 * time.Millisecond
	DefaultRoundPlaces = 8
)

// Policy 门禁策略。档由 Profile 判定。
type Policy struct {
	Profile string
	MaxSpreadBPS int
	MinVenues int
	CollectDeadline   time.Duration
	RoundPlaces       int32
	AllowSilentSingle bool
}

// DefaultPolicy 非默认：仍要求多源到齐，但允许调用方不附带 Bundle 走单簿（depth1）。

   func DefaultPolicy() Policy {
	  return Policy{
		  MaxSpreadBPS: DefaultMaxSpreadBPS,
		  MinVenues:    DefaultMinVenues,
		  CollectDeadline: DefaultDeadline,
		   RoundPlaces:       DefaultRoundPlaces,
		   AllowSilentSingle: true,
	   }
   }

// ProdPolicy 档：禁止静默取一家，缺源与超时一律拒绝。

   func ProdPolicy() Policy {
	   p := DefaultPolicy()
	   p.Profile = "prod"
	   p.AllowSilentSingle = false
	  return p
  }

// Production 是否档（APP_PROFILE=prod|production）。
  func (p Policy) Production() bool {
	   s := strings.ToLower(strings.TrimSpace(p.Profile))
	   return s == "prod" || s == "production"
  }

// RefuseSilentSingle 档或显式关闭回退时，禁止只用一家报价成交。
   func (p Policy) RefuseSilentSingle() bool {
	   if p.Production() {
		   return true
	  }
	  return !p.AllowSilentSingle
   }

  func (p Policy) maxSpread() int {
	  if p.MaxSpreadBPS <= 0 {
		   return DefaultMaxSpreadBPS
	  }
	   return p.MaxSpreadBPS
   }

  func (p Policy) minVenues() int {
	   if p.MinVenues <= 0 {
		   return DefaultMinVenues
	  }
	  return p.MinVenues
   }

   func (p Policy) deadline() time.Duration {
	  if p.CollectDeadline <= 0 {
		  return DefaultDeadline
	   }
	   return p.CollectDeadline
   }

   func (p Policy) roundPlaces() int32 {
	  if p.RoundPlaces <= 0 {
		   return DefaultRoundPlaces
	  }
	  return p.RoundPlaces
   }
