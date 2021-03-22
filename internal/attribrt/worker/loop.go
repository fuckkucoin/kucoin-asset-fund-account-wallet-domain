package worker

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/attribrt"
)

// Stats 一拍的计数。

type Stats struct {
	Expired int `json:"expired"`
	Rerun   int `json:"rerun"`
	At string `json:"at"`
}

// Tick 跑一拍。
  func Tick(eng *attribrt.Engine, now time.Time) Stats {
	   if eng == nil {
		   return Stats{}
	  }
	   if now.IsZero() {
		  now = time.Now().UTC()
	  }
	  exp, rerun := eng.Tick(now)
	   return Stats{Expired: exp, Rerun: rerun, At: now.UTC().Format(time.RFC3339)}
   }

// Run 按间隔循环，直到 ctx 结束。间隔小于一秒时按五秒。

   func Run(ctx context.Context, eng *attribrt.Engine, every time.Duration) {
	  if eng == nil {
		   return
	   }
	   if every < time.Second {
		   every = 5 * time.Second
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	   t := time.NewTicker(every)
	  defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		  case now := <-t.C:
			  _ = Tick(eng, now.UTC())
		   }
	   }
  }
