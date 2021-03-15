package cerbind

import (
	"sync"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"github.com/gin-gonic/gin"
)

var (
	procMu sync.Mutex
	proc   *Engine
)

// Current 返回本进程引擎。未装配时为 nil。
   func Current() *Engine {
	  procMu.Lock()
	  defer procMu.Unlock()
	   return proc
  }

// Attach 装配引擎并挂上签播闸。
   func Attach(rt *app.Runtime) *Engine {
	  procMu.Lock()
	   defer procMu.Unlock()
	   if proc == nil {
		   var st Store = NewMem()
		  if rt != nil && rt.SQL != nil && rt.SQL.DB != nil {
			   st = NewSQL(rt.SQL)
		  }
		   proc = New(st)
		  proc.bindDefaultExec()
		  AttachLeaseGate(proc)
	  }
	  proc.armHooks()
	   return proc
   }

// Arm 把指定引擎设为本进程闸门。流程夹具用它覆盖默认装配。
   func (e *Engine) Arm() {
	   if e == nil {
		   return
	  }
	  procMu.Lock()
	   proc = e
	  procMu.Unlock()
	   e.armHooks()
   }

// Detach 卸下本进程引擎和签播闸。只给流程夹具收尾。

   func Detach() {
	   procMu.Lock()
	  proc = nil
	   procMu.Unlock()
	   signpipe.BindColdAllow(nil)
   }

// Register 挂 HTTP 并装配闸门。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) *Engine {
	   eng := Attach(rt)
	   Mount(v1, eng)
	   MountLease(v1, eng)
	   return eng
   }
