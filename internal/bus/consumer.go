package bus

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

// ConsumerGroupConfig 消费者组参数。
type ConsumerGroupConfig struct {
	Brokers []string
	GroupID string
	Topic   string
	ClientID string
	MinBytes int
	MaxBytes int
	MaxWait time.Duration
	StartOffset int64 // kafkago.FirstOffset / LastOffset
}

// DefaultConsumerGroup 资金面默认组前缀。
  func DefaultConsumerGroup(role string) string {
	  role = strings.TrimSpace(role)
	   if role == "" {
		   role = "funding"
	  }
	   return "aa." + role + ".cg"
  }

// NormalizeConsumer 填默认值。
  func NormalizeConsumer(cfg ConsumerGroupConfig) (ConsumerGroupConfig, error) {
	  cfg.Brokers = filterBrokers(cfg.Brokers)
	   if len(cfg.Brokers) == 0 {
		   return cfg, ErrNoBroker
	   }
	  cfg.Topic = strings.TrimSpace(cfg.Topic)
	   if cfg.Topic == "" {
		  return cfg, fmt.Errorf("%w: topic required", ErrUnavailable)
	   }
	   cfg.GroupID = strings.TrimSpace(cfg.GroupID)
	   if cfg.GroupID == "" {
		  cfg.GroupID = DefaultConsumerGroup("funding")
	  }
	   if cfg.ClientID == "" {
		  cfg.ClientID = "aa-funding-consumer"
	   }
	  if cfg.MinBytes <= 0 {
		  cfg.MinBytes = 1
	  }
	   if cfg.MaxBytes <= 0 {
		   cfg.MaxBytes = 10e6
	   }
	  if cfg.MaxWait <= 0 {
		   cfg.MaxWait = 500 * time.Millisecond
	   }
	  if cfg.StartOffset == 0 {
		  cfg.StartOffset = kafkago.FirstOffset
	   }
	  return cfg, nil
   }

// NewReader 构造 kafka-go Reader（调用方负责 Close）。
   func NewReader(cfg ConsumerGroupConfig) (*kafkago.Reader, error) {
	  cfg, err := NormalizeConsumer(cfg)
	   if err != nil {
		  return nil, err
	   }
	   return kafkago.NewReader(kafkago.ReaderConfig{
		  Brokers:  cfg.Brokers,
		  GroupID:  cfg.GroupID,
		   Topic:    cfg.Topic,
		   MinBytes: cfg.MinBytes,
		  MaxBytes: cfg.MaxBytes,
		   MaxWait:  cfg.MaxWait,
		  StartOffset:    cfg.StartOffset,
		   CommitInterval: time.Second,
		   Dialer: &kafkago.Dialer{Timeout: 3 * time.Second,
			   ClientID: cfg.ClientID},
	  }), nil
  }

// RunConsumer 阻塞消费直到 ctx 取消；handler 失败不提交（依赖 CommitInterval 前成功路径）。
  func RunConsumer(ctx context.Context, cfg ConsumerGroupConfig, log zerolog.Logger, h Handler) error {
	   if h == nil {
		  return fmt.Errorf("%w: handler required", ErrUnavailable)
	   }
	   r, err := NewReader(cfg)
	  if err != nil {
		  return err
	   }
	  defer r.Close()
	   for {
		  m, err := r.FetchMessage(ctx)
		  if err != nil {
			  if ctx.Err() != nil {
				  return ctx.Err()
			  }
			  log.Error().Err(err).Str("topic", cfg.Topic).Str("group", cfg.GroupID).Msg("kafka fetch")
			   return err
		  }
		   if err := h(ctx, m.Key, m.Value); err != nil {
			  log.Error().Err(err).Str("topic", m.Topic).Int("partition", m.Partition).Msg("kafka handle")
			  // 不 Commit，保留至少一次。
			  continue
		   }
		   if err := r.CommitMessages(ctx, m); err != nil {
			   log.Error().Err(err).Str("topic", m.Topic).Msg("kafka commit")
			  return err
		  }
	  }
   }

// FundingConsumerGroups 默认角色→组映射。

  func FundingConsumerGroups() map[string]string {
	  return map[string]string{
		   "notify": DefaultConsumerGroup("notify"),
		   "recon": DefaultConsumerGroup("recon"),
		  "asset": DefaultConsumerGroup("asset"),
		  "risk":   DefaultConsumerGroup("risk"),
		  "outbox": DefaultConsumerGroup("outbox-ops"),
	   }
   }
