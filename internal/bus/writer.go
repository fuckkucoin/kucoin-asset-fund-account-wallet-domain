package bus

import (
	"context"
	"time"
	"github.com/rs/zerolog"
	kafkago "github.com/segmentio/kafka-go"
)

// NewKafkaProd 带 Writer 默认值的 Kafka 总线。
  func NewKafkaProd(brokers []string, cfg ProdConfig, log zerolog.Logger) *Kafka {
	   k := NewKafka(brokers, log)
	   k.prod = cfg
	   return k
  }

// DialProbe 探测首个 broker 可达。

  func (k *Kafka) DialProbe(timeout time.Duration) error {
	  if k == nil || len(k.brokers) == 0 {
		   return ErrNoBroker
	  }
	   if timeout <= 0 {
		  timeout = 2 * time.Second
	  }
	  ctx, cancel := context.WithTimeout(context.Background(), timeout)
	  defer cancel()
	  conn, err := kafkago.DialContext(ctx, "tcp", k.brokers[0])
	  if err != nil {
		  return err
	  }
	  _ = conn.Close()
	  return nil
   }

  func (k *Kafka) writerProd(topic string) *kafkago.Writer {
	  k.mu.Lock()
	   defer k.mu.Unlock()
	  if w, ok := k.writers[topic]; ok {
		  return w
	  }
	   batch := 20 * time.Millisecond
	   attempts := 3
	  clientID := "aa-funding-bus"
	  if k.prod.BatchTimeout > 0 {
		  batch = k.prod.BatchTimeout
	   }
	  if k.prod.MaxAttempts > 0 {
		   attempts = k.prod.MaxAttempts
	   }
	  if k.prod.ClientID != "" {
		  clientID = k.prod.ClientID
	   }
	  w := &kafkago.Writer{
		   Addr: kafkago.TCP(k.brokers...),
		   Topic: topic,
		   Balancer:     &kafkago.Hash{},
		  RequiredAcks: kafkago.RequireAll,
		   Async:        false,
		  BatchTimeout: batch,
		  MaxAttempts: attempts,
		  Transport:   &kafkago.Transport{ClientID: clientID},
	  }
	  k.writers[topic] = w
	  return w
   }
