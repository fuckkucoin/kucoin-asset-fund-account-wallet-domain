// 进程内消息总线。
// 作者: Span
// 日期: 2021.03.08
package bus

import (
	"context"
	"sync"
	"time"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

type Handler func(ctx context.Context, key, value []byte) error

// Bus 消息总线（Kafka 或进程内）。出入金以账本 journal 为准，bus 只投递事件。

type Bus interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
	Subscribe(ctx context.Context,
		topic, group string, h Handler)
	Kind() string
	Close() error
}

type Memory struct {
	mu sync.RWMutex
	subs map[string][]chan msg
}

type msg struct{ key, value []byte }

  func NewMemory() *Memory {
	  return &Memory{subs: map[string][]chan msg{}}
   }

   func (m *Memory) Kind() string { return "memory" }

  func (m *Memory) Publish(ctx context.Context, topic string, key, value []byte) error {
	  m.mu.RLock()
	  chs := append([]chan msg(nil), m.subs[topic]...)
	  m.mu.RUnlock()
	  cp := msg{append([]byte(nil), key...), append([]byte(nil), value...)}
	   for _, ch := range chs {
		   select {
		  case ch <- cp:
		  case <-ctx.Done():
			  return ctx.Err()
		  }
	  }
	  return nil
   }

   func (m *Memory) Subscribe(ctx context.Context, topic, _ string, h Handler) {
	   ch := make(chan msg, 256)
	  m.mu.Lock()
	   m.subs[topic] = append(m.subs[topic], ch)
	   m.mu.Unlock()
	   go func() {
		   for {
			  select {
			  case <-ctx.Done():
				  return
			   case n := <-ch:
				  deliver(ctx, h, n.key, n.value)
			  }
		   }
	  }()
   }

   func (m *Memory) Close() error { return nil }

type Kafka struct {
	brokers []string
	log     zerolog.Logger
	writers map[string]*kafkago.Writer
	mu      sync.Mutex
	prod    ProdConfig
}

   func NewKafka(brokers []string, log zerolog.Logger) *Kafka {
	  return &Kafka{brokers: brokers, log: log, writers: map[string]*kafkago.Writer{}}
   }

  func (k *Kafka) Kind() string { return "kafka" }

  func (k *Kafka) writer(topic string) *kafkago.Writer {
	   k.mu.Lock()
	  defer k.mu.Unlock()
	  if w, ok := k.writers[topic]; ok {
		  return w
	   }
	   w := &kafkago.Writer{
		  Addr: kafkago.TCP(k.brokers...),
		   Topic: topic,
		  Balancer: &kafkago.LeastBytes{},
		  RequiredAcks: kafkago.RequireAll,
		  Async:        false,
		  BatchTimeout: 20 * time.Millisecond,
	  }
	   k.writers[topic] = w
	  return w
  }

   func (k *Kafka) writerFor(topic string) *kafkago.Writer {
	   if k.prod.ClientID != "" || k.prod.MaxAttempts > 0 || k.prod.BatchTimeout > 0 {
		  return k.writerProd(topic)
	  }
	  return k.writer(topic)
   }

   func (k *Kafka) Publish(ctx context.Context, topic string, key, value []byte) error {
	   w := k.writerFor(topic)
	  msg := kafkago.Message{Key: key, Value: value, Time: time.Now()}
	   if k.prod.PublishTimeout > 0 {
		  var cancel context.CancelFunc
		   ctx, cancel = context.WithTimeout(ctx, k.prod.PublishTimeout)
		  defer cancel()
	  }
	   return w.WriteMessages(ctx, msg)
   }

   func (k *Kafka) Subscribe(ctx context.Context, topic, group string, h Handler) {
	  r := kafkago.NewReader(kafkago.ReaderConfig{
		   Brokers: k.brokers,
		  Topic:   topic,
		  GroupID: group,
		  MinBytes: 1,
		   MaxBytes: 1e6,
		   MaxWait:  200 * time.Millisecond,
	   })
	   go func() {
		   defer r.Close()
		  for {
			  m, err := r.FetchMessage(ctx)
			  if err != nil {
				   if ctx.Err() != nil {
					   return
				  }
				   k.log.Warn().Err(err).Str("topic", topic).Msg("kafka fetch")
				   time.Sleep(400 * time.Millisecond)
				   continue
			  }
			  deliver(ctx, h, m.Key, m.Value)
			   for {
				   if err := r.CommitMessages(ctx, m); err != nil {
					   if ctx.Err() != nil {
						   return
					   }
					   k.log.Warn().Err(err).Msg("kafka commit")
					   time.Sleep(400 * time.Millisecond)
					   continue
				   }
				   break
			   }
		  }
	  }()
  }

// deliver 当前消息处理成功才返回。失败留在这条上重试，不读下一条。
  func deliver(ctx context.Context, h Handler, key, value []byte) {
	   if h == nil {
		   return
	   }
	   for {
		   if ctx.Err() != nil {
			   return
		   }
		   if err := h(ctx, key, value); err == nil {
			   return
		   }
		   t := time.NewTimer(400 * time.Millisecond)
		   select {
		   case <-ctx.Done():
			   t.Stop()
			   return
		   case <-t.C:
		   }
	   }
  }

  func (k *Kafka) Close() error {
	  k.mu.Lock()
	  defer k.mu.Unlock()
	   for _, w := range k.writers {
		  _ = w.Close()
	  }
	  return nil
   }

  func Open(brokers []string, optional bool, log zerolog.Logger) Bus {
	   if len(brokers) == 0 || brokers[0] == "" {
		   log.Warn().Msg("kafka brokers empty; using in-memory bus")
		   return NewMemory()
	  }
	  k := NewKafka(brokers, log)
	  ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	  defer cancel()
	  conn, err := kafkago.DialContext(ctx, "tcp", brokers[0])
	   if err != nil {
		  if optional {
			  log.Warn().Err(err).Msg("kafka dial")
			  return NewMemory()
		   }
		   log.Error().Err(err).Msg("kafka dial")
		  return k
	   }
	  _ = conn.Close()
	   log.Info().Str("broker", brokers[0]).Msg("kafka bus ready")
	  return k
  }
