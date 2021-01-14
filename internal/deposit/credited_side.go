package deposit

import (
	"context"

	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ArrivalNotifier 链上 credited 后通知 Java funding-bff；实现见 internal/depositrails。
type ArrivalNotifier interface {
	NotifyArrival(ctx context.Context, tr *ledger.Transfer, confirmations int) error
}

// CreditedSide 入账后的 outbox 与 Java 平台单回调。
type CreditedSide struct {
	KV      persist.KV
	Cfg     *config.Config
	Arrival ArrivalNotifier
}

func (s *Service) BindCreditedSide(side *CreditedSide) *Service {
	if s != nil {
		s.creditedSide = side
	}
	return s
}

func (s *Service) emitCredited(ctx context.Context, tr *ledger.Transfer, confirmations int) {
	if s == nil || tr == nil || tr.State != ledger.StateCredited {
		return
	}
	if s.creditedSide != nil {
		s.creditedSide.Run(ctx, tr, confirmations)
	}
}

func (side *CreditedSide) Run(ctx context.Context, tr *ledger.Transfer, confirmations int) {
	if side == nil || tr == nil || tr.State != ledger.StateCredited {
		return
	}
	if side.KV != nil {
		_ = notify.FromConfig(side.KV, side.Cfg).Publish(ctx, "dep:"+tr.ID, notify.KindDepositCredited, tr.ID)
	}
	if side.Arrival != nil {
		_ = side.Arrival.NotifyArrival(ctx, tr, confirmations)
	}
}
