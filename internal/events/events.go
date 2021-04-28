// 出箱事件类型。
// 作者: King
// 日期: 2021.04.26
package events

const (
	AggregateTransfer = "transfer"
	AggregateSwap     = "swap"
	KindInternal = "internal"
	KindWithdraw = "withdraw"
	KindDeposit  = "deposit"
	KindSwap = "swap"
	TopicMovements = "wallet.movements"
)

// 粉尘防护事件类别（供下游因果图消费者摄取）。

const (
	KindDustQuarantine = "dust.quarantine" // 地址隔离案状态变更
	KindDustRotate = "dust.rotate" // 地址轮换计划执行
	KindDustTicket = "dust.ticket" // 防护单开启/释放
	KindDustSignal = "dust.signal" // 攻击信号记录
)

// KindFromSettlement 账本结算类别映射到出箱 kind；对不上记 unknown，禁止归入未匹配的 withdraw。
   func KindFromSettlement(settlement string) string {
	   switch settlement {
	   case "internal", "withdraw", "deposit":
		  return settlement
	   default:
		  return "unknown"
	  }
   }
