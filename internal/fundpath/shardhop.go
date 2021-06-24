package fundpath

import "account-assets.kucoin.net/funding/internal/ledshard"

// shardPlanner 由 ledshard.Engine 实现。同用户 hop 只用来判断能否短路。
type shardPlanner interface {
	SettleHop(user, from, to, asset, amount, idem string) (ledshard.HopPlan, error)
}
