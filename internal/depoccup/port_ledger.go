package depoccup

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type ledgerPort struct {
	led *ledger.Service
}

  func (p ledgerPort) Unwind(ctx context.Context, id string) error {
	  if p.led == nil {
		  return ErrRollbackUnbound
	  }
	  return p.led.UnwindReorg(ctx, id)
  }

   func (p ledgerPort) Fail(ctx context.Context, id, reason string) error {
	  if p.led == nil {
		   return ErrRollbackUnbound
	   }
	  return p.led.FailAndRollback(ctx, id, reason)
   }

   func (p ledgerPort) Level(ctx context.Context, userID string) (string, error) {
	   if p.led == nil {
		  return "", ErrUnavailable
	   }
	  return p.led.KYC(ctx, userID)
   }
