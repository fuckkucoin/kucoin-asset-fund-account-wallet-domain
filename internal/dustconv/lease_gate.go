package dustconv

import "context"

// LeaseGate Convert 绑定与结清。

type LeaseGate interface {
	AssertBeforeConvert(ctx context.Context,
		leaseID, user, asset, amount, actor string) error
	SettleAfterConvert(ctx context.Context, leaseID, convertID, usdt, actor string) error
	FailClosed(ctx context.Context, leaseID, actor, detail string) error
}
