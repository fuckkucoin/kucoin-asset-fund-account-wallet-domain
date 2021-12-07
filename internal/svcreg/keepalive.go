package svcreg

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/slo"
	"github.com/rs/zerolog"
)

// KeepAliveLoop 阻塞直到 ctx 取消。

   func KeepAliveLoop(ctx context.Context, e *Etcd, every time.Duration, log zerolog.Logger) {
	  if e == nil {
		   return
	   }
	   if every <= 0 {
		  every = slo.EtcdKeepAliveEvery
	  }
	  t := time.NewTicker(every)
	   defer t.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return
		   case <-t.C:
			   if err := e.KeepAlive(ctx); err != nil {
				   log.Debug().Err(err).Msg("registry keepalive")
			   }
		  }
	   }
   }
