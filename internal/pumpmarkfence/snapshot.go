package pumpmarkfence

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/outboxpump"
	"account-assets.kucoin.net/funding/internal/persist"
)

// GenSnapshot 他包只读世代。

type GenSnapshot struct {
	DeliverEffectGen string `json:"deliver_effect_gen"`
	OutboxID         string `json:"outbox_id"`
	PayloadHash      string `json:"payload_hash"`
	EffectKey        string `json:"effect_key"`
	Generation int64     `json:"generation"`
	State      string    `json:"state"`
	Deadline   time.Time `json:"deadline"`
	Applied    bool      `json:"applied"`
}

  func SnapshotOf(g DeliverEffectGen) GenSnapshot {
	  return GenSnapshot{
		  DeliverEffectGen: g.GenID, OutboxID: g.OutboxID, PayloadHash: g.PayloadHash,
		  EffectKey: g.EffectKey, Generation: g.Generation, State: string(g.State),
		   Deadline: g.Deadline,
		   Applied:  g.Applied,
	   }
  }

// ReadGen 只读查询。
  func (e *Engine) ReadGen(ctx context.Context, genID string) (GenSnapshot, error) {
	  if e == nil {
		   return GenSnapshot{}, ErrUnavailable
	   }
	  g, err := e.gens.Get(ctx, strings.TrimSpace(genID))
	   if err != nil {
		  return GenSnapshot{}, err
	   }
	   return SnapshotOf(*g), nil
  }

// MeasureLagReadonly 复用出箱泵滞后面，明确不得 Mark。
   func (e *Engine) MeasureLagReadonly(ctx context.Context, limit int) (outboxpump.Lag, error) {
	  if e == nil || e.box == nil {
		   return outboxpump.Lag{}, ErrUnavailable
	   }
	   return outboxpump.MeasureLag(ctx, e.box.KV(), limit)
  }

// LagMustNotMark 验收：滞后快照不能驱动 Mark。

   func (e *Engine) LagMustNotMark(ctx context.Context, kv persist.KV, outboxID string) error {
	  if e == nil {
		   return ErrUnavailable
	  }
	  _, err := outboxpump.MeasureLag(ctx, kv, 50)
	  if err != nil {
		   return err
	  }
	  return e.MarkDeniedFromLag(ctx, outboxID, "lag-reader")
   }
