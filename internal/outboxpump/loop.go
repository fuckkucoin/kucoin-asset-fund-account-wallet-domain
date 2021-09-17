package outboxpump

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/bus"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

// LoopConfig 出箱泵循环参数。

type LoopConfig struct {
	Name     string
	Interval time.Duration
	MaxRetry int
	Topic    string
	Routed bool
}

// DefaultLoopConfig 一致中间件 SLO 默认轮询。
   func DefaultLoopConfig() LoopConfig {
	  return LoopConfig{
		   Name: "outbox-pump",
		   Interval: slo.OutboxPollDefault,
		   MaxRetry: 5,
		   Topic:    bus.TopicLedgerPosted,
		   Routed: true,
	  }
  }

// RunLoop 阻塞运行出箱泵直到 ctx 取消。
   func RunLoop(ctx context.Context, kv persist.KV, b bus.Bus, cfg LoopConfig, log zerolog.Logger) error {
	   if b == nil {
		  return ErrUnavailable
	   }
	   if cfg.Name == "" {
		  cfg.Name = "outbox-pump"
	   }
	   if cfg.Interval <= 0 {
		  cfg.Interval = slo.OutboxPollDefault
	   }
	   if cfg.MaxRetry <= 0 {
		  cfg.MaxRetry = 5
	  }
	  wcfg := workerloop.Config{
		   Name: cfg.Name,
		   Interval: cfg.Interval,
		   MaxBackoff:   30 * time.Second,
		  FailThresh:   8,
		   CircuitPause: 15 * time.Second,
	  }
	   return workerloop.Run(ctx, wcfg, log, func(ctx context.Context) error {
		   var rep Report
		  if cfg.Routed {
			   r := NewRetry(kv, cfg.MaxRetry)
			  d := RoutedDeliver{Bus: b, Fallback: cfg.Topic}
			   rep, _ = r.Tick(ctx, d.Handler())
		   } else {
			   rep = TickBus(ctx, kv, b, cfg.Topic, cfg.MaxRetry)
		   }
		   if rep.Err != "" {
			   return fmt.Errorf("outbox tick: %s", rep.Err)
		  }
		   return nil
	   })
  }

// Job 返回可供外部 workerloop 组合的单次 Tick。

  func Job(kv persist.KV, b bus.Bus, cfg LoopConfig) workerloop.Job {
	  return func(ctx context.Context) error {
		   if b == nil {
			   return ErrUnavailable
		   }
		   var rep Report
		   if cfg.Routed {
			   r := NewRetry(kv, cfg.MaxRetry)
			  if cfg.MaxRetry <= 0 {
				  r = NewRetry(kv, 5)
			   }
			   d := RoutedDeliver{Bus: b, Fallback: cfg.Topic}
			  rep, _ = r.Tick(ctx, d.Handler())
		   } else {
			   rep = TickBus(ctx, kv, b, cfg.Topic, cfg.MaxRetry)
		   }
		  if rep.Err != "" {
			  return fmt.Errorf("outbox tick: %s", rep.Err)
		   }
		  return nil
	   }
  }
