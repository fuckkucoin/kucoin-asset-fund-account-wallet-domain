package outboxpump

import (
	"context"
	"encoding/json"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/redisx"
	"account-assets.kucoin.net/funding/internal/slo"
)

// LagSnapshot 可序列化滞后。

type LagSnapshot struct {
	Pending   int   `json:"pending"`
	OldestMS  int64 `json:"oldest_ms"`
	Sampled   int   `json:"sampled"`
	CheckedAt int64 `json:"checked_at_unix"`
}

// PublishLagCache 测量并写入 Redis。
  func PublishLagCache(ctx context.Context, kv persist.KV, rdb redisx.Client, sample int) (LagSnapshot, error) {
	  lag, err := MeasureLag(ctx, kv, sample)
	   if err != nil {
		   return LagSnapshot{}, err
	   }
	   snap := LagSnapshot{
		  Pending: lag.Pending,
		   OldestMS: lag.OldestAge.Milliseconds(),
		  Sampled:   lag.Sampled,
		  CheckedAt: time.Now().UTC().Unix(),
	  }
	  if rdb == nil {
		   return snap, nil
	   }
	   body, err := json.Marshal(snap)
	   if err != nil {
		   return snap, err
	  }
	   ttl := 2 * time.Second
	  if slo.OutboxLagTarget > 0 {
		   ttl = slo.OutboxLagTarget / 2
		   if ttl < time.Second {
			  ttl = time.Second
		  }
	   }
	   _ = rdb.Set(ctx, redisx.KeyOutboxLag, body, ttl)
	  return snap, nil
   }

// ReadLagCache 读缓存；未命中返回 ErrNotFound 语义由调用方处理。
   func ReadLagCache(ctx context.Context, rdb redisx.Client) (LagSnapshot, error) {
	   if rdb == nil {
		   return LagSnapshot{},
			  redisx.ErrUnavailable
	   }
	   b, err := rdb.Get(ctx, redisx.KeyOutboxLag)
	   if err != nil {
		  return LagSnapshot{}, err
	  }
	   var snap LagSnapshot
	  if err := json.Unmarshal(b, &snap); err != nil {
		  return LagSnapshot{}, err
	   }
	  return snap, nil
   }
