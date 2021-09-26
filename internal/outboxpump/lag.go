package outboxpump

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Lag 出箱滞后快照。
type Lag struct {
	Pending   int           `json:"pending"`
	OldestAge time.Duration `json:"oldest_age"`
	Sampled int `json:"sampled"`
}

// MeasureLag 拉取至多 limit 条 Pending 估算滞后。
   func MeasureLag(ctx context.Context, kv persist.KV, limit int) (Lag, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	  box := persist.NewOutbox(kv)
	  pend, err := box.Pending(ctx, limit)
	  if err != nil {
		   return Lag{}, err
	   }
	  out := Lag{Pending: len(pend), Sampled: len(pend)}
	  var oldest time.Time
	  for _, ev := range pend {
		   if ev.CreatedAt.IsZero() {
			   continue
		  }
		  if oldest.IsZero() || ev.CreatedAt.Before(oldest) {
			   oldest = ev.CreatedAt
		  }
	   }
	   if !oldest.IsZero() {
		   out.OldestAge = time.Since(oldest)
		   if out.OldestAge < 0 {
			  out.OldestAge = 0
		   }
	  }
	  return out, nil
  }

// LagHealthy 相对 SLO 目标判断是否可接受（默认 5s）。
   func LagHealthy(lag Lag, maxAge time.Duration) bool {
	  if maxAge <= 0 {
		   maxAge = 5 * time.Second
	  }
	  return lag.OldestAge <= maxAge
  }
