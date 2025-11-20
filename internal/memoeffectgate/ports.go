package memoeffectgate

import "context"

// TicketPort 工单读取与状态推进。
type TicketPort interface {
	GetMemoTicket(ctx context.Context, id string) (MemoTicketView, error)
	AdvanceTicket(ctx context.Context, id, status string) (MemoTicketView, error)
}

// CreditPort BindMemo → ObserveCredit → ApplyInbound。金额只认观察。
type CreditPort interface {
	BindMemo(userID,
		chain, memo string) error
	ObserveCredit(ctx context.Context,
		userID, chain, txHash, asset, memo, addr string) (CreditSight, error)
	ApplyInbound(ctx context.Context, userID, chain, asset, amount, idem string) error
	ReverseInbound(ctx context.Context, userID, asset, amount, idem string) error
}

// ReversePort 受控冲正/隔离。
type ReversePort interface {
	Quarantine(ctx context.Context, userID, asset, amount, reason string) error
}
