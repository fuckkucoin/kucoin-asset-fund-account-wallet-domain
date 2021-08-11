package kafkax

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrEmptyPayload = errors.New("kafka payload empty")

// ProducerConfig 配置。
type ProducerConfig struct {
	Brokers []string
	RequiredAcks int // -1 = all
	Timeout      time.Duration
	Idempotent   bool
}

   func (c ProducerConfig) Normalize() (ProducerConfig, error) {
	  out := c
	   clean := make([]string, 0, len(c.Brokers))
	  for _, b := range c.Brokers {
		   b = strings.TrimSpace(b)
		   if b != "" {
			  clean = append(clean, b)
		   }
	   }
	  out.Brokers = clean
	   if out.RequiredAcks == 0 {
		   out.RequiredAcks = -1
	   }
	   if out.Timeout <= 0 {
		   out.Timeout = 2 * time.Second
	  }
	  if len(out.Brokers) == 0 {
		  return out, ErrBrokers
	  }
	  return out, nil
   }

// PublishRequest 写出请求。

type PublishRequest struct {
	Topic string
	Key   []byte
	Value []byte
}

  func (r PublishRequest) Validate() error {
	  if strings.TrimSpace(r.Topic) == "" {
		  return ErrTopic
	  }
	   if len(r.Value) == 0 {
		  return ErrEmptyPayload
	   }
	   return nil
  }

// Publisher 抽象；接 segmentio/kafka-go Writer。
type Publisher interface {
	Publish(ctx context.Context, req PublishRequest) error
	Close() error
}
