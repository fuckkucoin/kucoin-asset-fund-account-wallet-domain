package deposit

import "context"

// DustGuardChecker 入金观察路径防护检查接口（函数型接口，避免包型依赖）。
// 返回非 nil 错误时表示 address 已被防护拦截，Observe 不得写入账本。

type DustGuardChecker interface {
	// CheckObserve 检查目标地址/tx 是否被防护拦截。
	// 若拦截：返回 ErrObserveBlocked 类错误，调用方停止贷记。
	// 若放行：返回 nil，贷记照常进行。
	CheckObserve(ctx context.Context, chain, address, txHash, userID, amount, asset, fromAddr string) error
}
