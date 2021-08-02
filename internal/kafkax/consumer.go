package kafkax

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBrokers = errors.New("kafka brokers required")
	ErrTopic   = errors.New("kafka topic required")
	ErrGroup = errors.New("kafka group required")
)

// Message 一条消费记录。

type Message struct {
	Topic string
	Partition int
	Offset int64
	Key    []byte
	Value  []byte
	Time   time.Time
}

// Handler 处理成功才应提交。
type Handler func(ctx context.Context, msg Message) error

// ConsumerConfig 消费组配置。
type ConsumerConfig struct {
	Brokers []string
	Topic   string
	Group string
	Region string // sg | tyo
	MinBytes int
	MaxBytes int
}

// Normalize 清洗配置。

   func (c ConsumerConfig) Normalize() (ConsumerConfig, error) {
	  out := c
	  clean := make([]string, 0, len(c.Brokers))
	  for _, b := range c.Brokers {
		   b = strings.TrimSpace(b)
		   if b != "" {
			  clean = append(clean, b)
		   }
	  }
	  out.Brokers = clean
	  out.Topic = strings.TrimSpace(c.Topic)
	   out.Group = strings.TrimSpace(c.Group)
	   out.Region = strings.ToLower(strings.TrimSpace(c.Region))
	  if out.Region == "" {
		  out.Region = "sg"
	   }
	  if out.MinBytes <= 0 {
		  out.MinBytes = 1e3
	   }
	  if out.MaxBytes <= 0 {
		   out.MaxBytes = 10e6
	  }
	  if len(out.Brokers) == 0 {
		  return out, ErrBrokers
	  }
	   if out.Topic == "" {
		  return out, ErrTopic
	  }
	   if out.Group == "" {
		   return out, ErrGroup
	   }
	  return out, nil
   }

// DefaultFundingTopics 资金域主题清单。
  func DefaultFundingTopics() []string {
	  return []string{
		   "deposit.credited",
		   "withdraw.requested",
		   "ledger.posted",
		  "recon.diff",
		   "kyt.hit",
		   "ops.outbox.stuck",
		  "ops.registry.quorum_loss",
		  "ops.worker.circuit",
	   }
   }

// ValidateRegionBrokers 粗检 SG/TYO 主机命名。
   func ValidateRegionBrokers(region string, brokers []string) error {
	   region = strings.ToLower(strings.TrimSpace(region))
	   if region != "sg" && region != "tyo" {
		   return fmt.Errorf("region must be sg or tyo")
	  }
	  if len(brokers) == 0 {
		  return ErrBrokers
	  }
	   return nil
   }
