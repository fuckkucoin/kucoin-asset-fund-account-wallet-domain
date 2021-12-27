package wdrelease

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

// MountWorker 在进程里跑超时扫描。路由本身不因拍子改变。
  func MountWorker(_ *gin.Engine, rt *app.Runtime) {
	   d := DeskFrom(rt)
	  if d == nil {
		   return
	  }
	  go func() {
		   tk := time.NewTicker(time.Minute)
		   defer tk.Stop()
		   for range tk.C {
			  _, _ = d.Tick(context.Background())
		  }
	   }()
   }
