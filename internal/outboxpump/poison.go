package outboxpump

import (
	"context"
	"encoding/json"
	"time"
	"account-assets.kucoin.net/funding/internal/bus"
)

// PoisonEvent 毒丸载荷。

type PoisonEvent struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Payload  string `json:"payload"`
	Attempts int    `json:"attempts"`
	Cause string `json:"cause"`
	PoisonAt time.Time `json:"poison_at"`
}

// PublishPoison 投递毒丸通知。

   func PublishPoison(ctx context.Context, b bus.Bus, ev PoisonEvent) error {
	   if b == nil {
		  return ErrUnavailable
	  }
	  if ev.PoisonAt.IsZero() {
		  ev.PoisonAt = time.Now().UTC()
	  }
	   body, err := json.Marshal(ev)
	  if err != nil {
		   return err
	  }
	  key := ev.Key
	  if key == "" {
		  key = "poison"
	   }
	   return b.Publish(ctx, bus.TopicOutboxPoison, []byte(key), body)
  }
