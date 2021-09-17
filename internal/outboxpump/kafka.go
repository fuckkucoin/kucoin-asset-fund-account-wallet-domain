package outboxpump

import (
	"context"
	"encoding/json"
	"account-assets.kucoin.net/funding/internal/bus"
	"account-assets.kucoin.net/funding/internal/persist"
)

// KafkaDeliver 把出箱泵到 Bus。

type KafkaDeliver struct {
	Bus   bus.Bus
	Topic string
}

// Handler 供 Runner.Tick / Retry.Tick 使用。

  func (d KafkaDeliver) Handler() persist.Handler {
	  topic := d.Topic
	   if topic == "" {
		   topic = bus.TopicLedgerPosted
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
		  return d.Bus.Publish(ctx, topic, []byte(key), body)
	   }
   }
