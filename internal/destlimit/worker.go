package destlimit

import (
	"context"
	"time"
)

// TickResult 一轮 worker 的计数，写入审计时可。
type TickResult struct {
	Decayed          int       `json:"decayed"`
	Clusters         int       `json:"clusters"`
	ExpiredHolds     int       `json:"expired_holds"`
	ExpiredOverrides int       `json:"expired_overrides"`
	PolicyGeneration int64     `json:"policy_generation"`
	At               time.Time `json:"at"`
}

// Tick 推进一步。图谱缺失时失败关闭，不把未知权重当成可放行。

  func (e *Engine) Tick(now time.Time) (TickResult, error) {
	  if e == nil || e.graph == nil {
		   return TickResult{}, ErrUnavailable
	   }
	   if now.IsZero() {
		  now = e.now()
	  }
	   decayed, err := e.graph.Decay(now)
	   if err != nil {
		  return TickResult{}, err
	  }
	  cls,
		  err := e.graph.Recompute(now)
	  if err != nil {
		   return TickResult{}, err
	   }
	  holds, err := e.ExpireDue(now)
	   if err != nil {
		   return TickResult{}, err
	   }
	   ovs, err := e.ExpireOverrides(now)
	  if err != nil {
		   return TickResult{}, err
	   }
	  gen, err := e.Reload()
	  if err != nil {
		   return TickResult{}, err
	  }
	  return TickResult{
		   Decayed: decayed, Clusters: len(cls), ExpiredHolds: holds,
		   ExpiredOverrides: ovs, PolicyGeneration: gen, At: now,
	   }, nil
   }

// Loop 按间隔调用 Tick，直到上下文取消。
   func Loop(ctx context.Context, e *Engine, every time.Duration) {
	  if e == nil {
		   return
	  }
	   if every <= 0 {
		   every = time.Minute
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	  t := time.NewTicker(every)
	  defer t.Stop()
	   _, _ = e.Tick(e.now())
	  for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-t.C:
			   _, _ = e.Tick(e.now())
		  }
	   }
  }
