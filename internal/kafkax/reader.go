package kafkax

import (
	"context"
	"fmt"
	"time"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

// Run 阻塞消费直到 ctx 取消。

   func Run(ctx context.Context, cfg ConsumerConfig, log zerolog.Logger, h Handler) error {
	   if h == nil {
		  return fmt.Errorf("kafkax handler required")
	   }
	  n, err := cfg.Normalize()
	  if err != nil {
		  return err
	  }
	  if err := ValidateRegionBrokers(n.Region, n.Brokers); err != nil {
		   return err
	  }
	  r := kafkago.NewReader(kafkago.ReaderConfig{
		   Brokers: n.Brokers,
		   GroupID: n.Group,
		   Topic:   n.Topic,
		  MinBytes: n.MinBytes,
		  MaxBytes:       n.MaxBytes,
		  MaxWait:        500 * time.Millisecond,
		   StartOffset:    kafkago.FirstOffset,
		   CommitInterval: 0, // 手动提交
		  Dialer:         &kafkago.Dialer{Timeout: 3 * time.Second, ClientID: "aa-funding-kafkax-cg"},
	  })
	   defer r.Close()
	   for {
		  m, err := r.FetchMessage(ctx)
		   if err != nil {
			   if ctx.Err() != nil {
				  return ctx.Err()
			   }
			  log.Error().Err(err).Str("topic", n.Topic).Str("group", n.Group).Msg("kafkax fetch")
			   return err
		   }
		   msg := Message{
			   Topic:     m.Topic,
			   Partition: m.Partition,
			   Offset: m.Offset,
			  Key:    append([]byte(nil), m.Key...),
			  Value:  append([]byte(nil), m.Value...),
			  Time:   m.Time,
		   }
		   if err := h(ctx, msg); err != nil {
			   log.Error().Err(err).Str("topic", msg.Topic).Int64("offset", msg.Offset).Msg("kafkax handle")
			   continue
		  }
		   if err := r.CommitMessages(ctx, m); err != nil {
			  log.Error().Err(err).Str("topic", msg.Topic).Msg("kafkax commit")
			   return err
		   }
	  }
  }

// GroupForTopic 建议组名。
   func GroupForTopic(topic, role string) string {
	  if role == "" {
		   role = "funding"
	  }
	  return fmt.Sprintf("aa.%s.%s", role, topic)
  }
