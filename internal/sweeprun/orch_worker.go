package sweeprun

import (
	"context"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

var (
	boundMu sync.Mutex
	bound   *Orch
)

   func BindGlobal(o *Orch) {
	  boundMu.Lock()
	   bound = o
	   boundMu.Unlock()
  }

  func Bound() *Orch {
	   boundMu.Lock()
	   defer boundMu.Unlock()
	   return bound
   }

  func (o *Orch) Loop(ctx context.Context, every time.Duration) {
	  if o == nil {
		   return
	  }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   if every <= 0 {
		   every = 2 * time.Second
	   }
	   tick := time.NewTicker(every)
	  defer tick.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		   case <-tick.C:
			  _, _ = o.Tick(ctx, 16)
		  }
	   }
  }

// MountWorker 在 sweep-orch 进程里起拍。主 API 不挂这个循环。

  func MountWorker(r *gin.Engine, rt *app.Runtime) {
	  _ = r
	   _ = rt
	  o := Bound()
	  if o == nil {
		   return
	  }
	   go o.Loop(context.Background(), 2*time.Second)
  }
