package outboxpump

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/bus"
	"account-assets.kucoin.net/funding/internal/persist"
)

// TopicForKind 按业务 kind 选主题。
  func TopicForKind(kind, fallback string) string {
	   k := strings.ToLower(strings.TrimSpace(kind))
	   switch k {
	  case "deposit.credited", "deposit_credited", "deposit.credit":
		  return bus.TopicDepositCredited
	   case "deposit.observed", "deposit_observed":
		  return bus.TopicDepositObserved
	  case "withdraw.requested", "withdraw_requested":
		   return bus.TopicWithdrawRequested
	   case "withdraw.broadcast", "withdraw_broadcast":
		   return bus.TopicWithdrawBroadcast
	   case "withdraw.confirmed", "withdraw_confirmed":
		   return bus.TopicWithdrawConfirmed
	  case "withdraw.cancelled", "withdraw_cancelled":
		   return bus.TopicWithdrawCancelled
	   case "ledger.posted", "ledger_posted", "posted":
		  return bus.TopicLedgerPosted
	  case "recon.diff", "recon_diff":
		   return bus.TopicReconDiff
	  case "kyt.hit", "kyt_hit":
		  return bus.TopicKYTHit
	  case "ledger.drift_blocked", "drift_blocked":
		  return bus.TopicDriftBlocked
	  case "outbox.poison", "poison":
		  return bus.TopicOutboxPoison
	   }
	  if fallback != "" {
		   return fallback
	  }
	   return bus.TopicLedgerPosted
  }

// RoutedDeliver 按事件 Kind 投递到对应主题。
type RoutedDeliver struct {
	Bus bus.Bus
	Fallback string
}

// Handler 供 Retry / Runner 使用。

   func (d RoutedDeliver) Handler() persist.Handler {
	   fb := d.Fallback
	  if fb == "" {
		   fb = bus.TopicLedgerPosted
	  }
	   return func(ctx context.Context, ev persist.OutboxEvent) error {
		   if d.Bus == nil {
			   return ErrUnavailable
		   }
		   body, err := json.Marshal(ev)
		   if err != nil {
			  return err
		   }
		  key := ev.Key
		   if key == "" {
			   key = ev.ID
		   }
		   topic := TopicForKind(ev.Kind, fb)
		   return d.Bus.Publish(ctx, topic, []byte(key), body)
	  }
  }
