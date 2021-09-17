package outboxpump

import (
	"context"
	"account-assets.kucoin.net/funding/internal/bus"
	"account-assets.kucoin.net/funding/internal/persist"
)

// TickBus 用 KafkaDeliver 跑一轮 Retry。
  func TickBus(ctx context.Context, kv persist.KV, b bus.Bus, topic string, max int) Report {
	   r := NewRetry(kv, max)
	   d := KafkaDeliver{Bus: b, Topic: topic}
	   rep, _ := r.Tick(ctx, d.Handler())
	   return rep
   }
