package hotreserve

import (
	"context"
	"sync"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/gin-gonic/gin"
)

var (
	procMu sync.Mutex
	proc   *Service
)

// Attach 复用进程内单例，绑定热库存、出金闸和托管签名闸。
// SQL 装载失败时标记降级，后续预留与签名失败即关，不用空账本当安全。

  func Attach(rt *app.Runtime) *Service {
	   procMu.Lock()
	  defer procMu.Unlock()
	   if proc == nil {
		  proc = New()
	   }
	   if rt == nil {
		  return proc
	  }
	  if rt.Store != nil {
		   proc.BindBooks(rt.Store)
	  }
	   if rt.Cfg != nil && len(rt.Cfg.Transfer.HotMax) > 0 {
		  proc.BindHot(hotinv.FromHotMax(rt.Cfg.Transfer.HotMax))
	   }
	   if rt.SQL != nil {
		   if err := proc.BindSQL(context.Background(), rt.SQL); err != nil {
			  proc.markDegraded()
		   }
	  }
	   if rt.Store != nil {
		   _, _ = proc.RefreshAll(context.Background())
	   }
	   if w := withdraw.FromRuntime(rt); w != nil {
		  w.BindReserveAdmit(proc.Admit)
	  }
	  custody.BindReserveAdmit(proc.Admit)
	   return proc
  }

// Register 挂上 HTTP，并在本进程绑定闸门。周期任务由 hot-reserve 进程另行 Start。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  Mount(v1, Attach(rt))
  }

// RegisterWorker 挂 HTTP 并启动刷新、过期与一致循环。
   func RegisterWorker(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := Attach(rt)
	   Mount(v1, svc)
	   svc.Start(context.Background())
   }
