package ledseal

import (
	"context"
	"encoding/json"
	"github.com/rs/zerolog"
)

// Alerter 告警投递器。

type Alerter struct {
	log zerolog.Logger
}

// NewAlerter 构造告警投递器。
   func NewAlerter(log zerolog.Logger) *Alerter {
	   return &Alerter{log: log.With().Str("component", "ledseal-alert").Logger()}
   }

// PublishDrift 发布漂移告警事件。
// 环境可替换为向 Kafka/bus 写消息；此处写结构化日志。

   func (a *Alerter) PublishDrift(ctx context.Context, ev AlertEvent) error {
	  if a == nil {
		  return nil
	   }
	  body, _ := json.Marshal(ev)
	   a.log.Warn().
		  Str("batch_id", ev.BatchID).
		   Str("plan_id", ev.PlanID).
		  Int("drift_count", ev.DriftCount).
		   Str("max_severity", string(ev.MaxSeverity)).
		  Int("block_hints", len(ev.BlockWriteSuggestions)).
		  RawJSON("event", body).
		  Msg("ledseal drift alert")
	  return nil
   }

// PublishGap 发布序号间隙/回滚告警事件。

   func (a *Alerter) PublishGap(ctx context.Context, inc GapIncident) error {
	   if a == nil {
		   return nil
	   }
	   body, _ := json.Marshal(inc)
	   a.log.Error().
		   Str("plan_id", inc.PlanID).
		  Int64("expected_seq", inc.ExpectedSeq).
		  Int64("actual_seq", inc.ActualSeq).
		   Str("kind", inc.Kind).
		  RawJSON("incident", body).
		  Msg("ledseal gap incident")
	  return nil
   }
