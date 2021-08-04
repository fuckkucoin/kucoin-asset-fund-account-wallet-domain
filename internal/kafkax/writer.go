package kafkax

import (
	"context"
	"fmt"
	"sync"
	"time"
	kafkago "github.com/segmentio/kafka-go"
)

// WriterPublisher 按 topic 复用 Writer。

type WriterPublisher struct {
	cfg ProducerConfig
	mu  sync.Mutex
	writers map[string]*kafkago.Writer
}

// NewWriterPublisher 构造。

   func NewWriterPublisher(cfg ProducerConfig) (*WriterPublisher, error) {
	   n, err := cfg.Normalize()
	   if err != nil {
		  return nil, err
	   }
	   return &WriterPublisher{cfg: n, writers: map[string]*kafkago.Writer{}}, nil
   }

  func (p *WriterPublisher) writer(topic string) *kafkago.Writer {
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   if w, ok := p.writers[topic]; ok {
		   return w
	   }
	  acks := kafkago.RequireAll
	   if p.cfg.RequiredAcks == 1 {
		  acks = kafkago.RequireOne
	  } else if p.cfg.RequiredAcks == 0 {
		   acks = kafkago.RequireNone
	  }
	   w := &kafkago.Writer{
		  Addr: kafkago.TCP(p.cfg.Brokers...),
		   Topic:        topic,
		   Balancer:     &kafkago.Hash{},
		  RequiredAcks: acks,
		  Async:        false,
		   BatchTimeout: 20 * time.Millisecond,
		   MaxAttempts:  3,
		   Transport:    &kafkago.Transport{ClientID: "aa-funding-kafkax"},
	  }
	   p.writers[topic] = w
	   return w
  }

// Publish 同步写出。

  func (p *WriterPublisher) Publish(ctx context.Context, req PublishRequest) error {
	  if p == nil {
		  return ErrBrokers
	   }
	  if err := req.Validate(); err != nil {
		  return err
	   }
	  if _, ok := ctx.Deadline(); !ok && p.cfg.Timeout > 0 {
		   var cancel context.CancelFunc
		   ctx,
			   cancel = context.WithTimeout(ctx, p.cfg.Timeout)
		  defer cancel()
	  }
	   w := p.writer(req.Topic)
	  err := w.WriteMessages(ctx, kafkago.Message{Key: req.Key, Value: req.Value})
	   if err != nil {
		   return fmt.Errorf("kafkax publish %s: %w", req.Topic, err)
	  }
	  return nil
   }

// Close 关闭全部 Writer。
   func (p *WriterPublisher) Close() error {
	   if p == nil {
		  return nil
	  }
	   p.mu.Lock()
	   defer p.mu.Unlock()
	  var first error
	   for t, w := range p.writers {
		  if err := w.Close(); err != nil && first == nil {
			  first = err
		   }
		   delete(p.writers, t)
	  }
	   return first
   }

// OpenPublisherFromEnv 读 KAFKA_BROKERS。
  func OpenPublisherFromEnv() (Publisher, error) {
	   cfg := ProducerConfig{RequiredAcks: -1, Timeout: 2 * time.Second, Idempotent: true}
	  // brokers 由调用方填；此处仅用目录默认。
	  cfg.Brokers = []string{
		   "kafka-0.kucoin.net:9092",
		  "kafka-1.kucoin.net:9092",
		  "kafka-2.kucoin.net:9092",
	   }
	  return NewWriterPublisher(cfg)
  }
