package outboxpump

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/bus"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/redisx"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

// LeaderPumpConfig 领导权泵参数。
type LeaderPumpConfig struct {
	Loop      LoopConfig
	LeaderKey string
	LeaderTTL time.Duration
	Refresh time.Duration
	LagEvery  time.Duration
	LagMaxAge time.Duration
	LagSample int
	AlertBus  bool // 滞后/毒丸写 ops 主题
}

// DefaultLeaderPumpConfig 默认。
  func DefaultLeaderPumpConfig() LeaderPumpConfig {
	   return LeaderPumpConfig{
		  Loop: DefaultLoopConfig(),
		  LeaderKey: redisx.KeyOutboxLeader,
		   LeaderTTL: slo.WorkerLeaderTTL,
		   Refresh:  slo.WorkerLeaderRefresh,
		  LagEvery: 5 * time.Second,
		   LagMaxAge: slo.OutboxLagTarget,
		  LagSample: 50,
		  AlertBus:  true,
	   }
   }

// RunLeaderPump 争抢领导权后阻塞泵出箱，直到 ctx 取消。
   func RunLeaderPump(ctx context.Context, kv persist.KV, b bus.Bus, rdb redisx.Client, cfg LeaderPumpConfig, log zerolog.Logger) error {
	   if b == nil {
		  return ErrUnavailable
	   }
	  if cfg.Loop.Name == "" {
		   cfg.Loop = DefaultLoopConfig()
	   }
	   lead := &workerloop.Leader{
		  Client: rdb,
		  Key:   cfg.LeaderKey,
		  TTL:   cfg.LeaderTTL,
		  Every: cfg.Refresh,
	  }
	  wcfg := workerloop.Config{
		  Name:       cfg.Loop.Name,
		  Interval:   cfg.Loop.Interval,
		  MaxBackoff: 30 * time.Second,
		  FailThresh: 8,
		  CircuitPause: 15 * time.Second,
	   }
	   retry := NewRetry(kv, cfg.Loop.MaxRetry)
	   retry.SetBackoffPolicy(DefaultBackoff())
	  var lastLagCheck time.Time
	  job := func(ctx context.Context) error {
		   var handler persist.Handler
		   if cfg.Loop.Routed {
			   d := RoutedDeliver{Bus: b, Fallback: cfg.Loop.Topic}
			  handler = wrapPoisonHandler(b, retry, d.Handler(), log)
		  } else {
			  d := KafkaDeliver{Bus: b, Topic: cfg.Loop.Topic}
			   handler = wrapPoisonHandler(b, retry, d.Handler(), log)
		  }
		   rep, err := retry.Tick(ctx, handler)
		  if cfg.AlertBus && (time.Since(lastLagCheck) >= cfg.LagEvery || lastLagCheck.IsZero()) {
			   lastLagCheck = time.Now().UTC()
			   _ = maybeAlertLag(ctx, kv, b, cfg, log)
		   }
		   if rep.Err != "" {
			   return fmt.Errorf("outbox tick: %s", rep.Err)
		   }
		   return err
	   }
	  return workerloop.RunWithLeader(ctx, lead, wcfg, log, job)
   }

  func wrapPoisonHandler(b bus.Bus, r *Retry, h persist.Handler, log zerolog.Logger) persist.Handler {
	   return func(ctx context.Context, ev persist.OutboxEvent) error {
		   err := h(ctx, ev)
		   if err == nil {
			   return nil
		  }
		   st := r.Status(ev.Key)
		   // Tick 内已自增；此处在即将毒丸时旁路投递。
		  if st.N+1 >= r.MaxAttempts() && !st.Poison {
			   pe := PoisonEvent{
				   Key: ev.Key,
				  Kind:     ev.Kind,
				   Payload:  ev.Payload,
				   Attempts: st.N + 1,
				   Cause:    err.Error(),
				  PoisonAt: time.Now().UTC(),
			  }
			   if perr := PublishPoison(ctx, b, pe); perr != nil {
				  log.Warn().Err(perr).Str("key", ev.Key).Msg("outbox poison publish")
			   }
			   _ = publishOpsPoison(ctx, b, pe)
		  }
		  return err
	   }
   }

  func publishOpsPoison(ctx context.Context, b bus.Bus, pe PoisonEvent) error {
	  if b == nil {
		   return nil
	  }
	  body, err := json.Marshal(pe)
	  if err != nil {
		   return err
	   }
	  return b.Publish(ctx, bus.TopicWorkerCircuit, []byte("outbox.poison"), body)
   }

  func maybeAlertLag(ctx context.Context, kv persist.KV, b bus.Bus, cfg LeaderPumpConfig, log zerolog.Logger) error {
	   lag, err := MeasureLag(ctx, kv, cfg.LagSample)
	  if err != nil {
		  return err
	   }
	   if LagHealthy(lag, cfg.LagMaxAge) {
		   return nil
	  }
	  log.Warn().Int("pending", lag.Pending).Dur("oldest", lag.OldestAge).Msg("outbox lag over target")
	   slo.FireMiddlewareAlert("outbox.lag", float64(lag.OldestAge.Milliseconds()), float64(cfg.LagMaxAge.Milliseconds()))
	  if !cfg.AlertBus || b == nil {
		   return nil
	   }
	   body, _ := json.Marshal(map[string]any{
		  "pending": lag.Pending,
		  "oldest_ms":  lag.OldestAge.Milliseconds(),
		   "sampled":    lag.Sampled,
		   "target_ms":  cfg.LagMaxAge.Milliseconds(),
		  "checked_at": time.Now().UTC(),
	  })
	   return b.Publish(ctx, bus.TopicOutboxStuck, []byte("lag"), body)
   }
