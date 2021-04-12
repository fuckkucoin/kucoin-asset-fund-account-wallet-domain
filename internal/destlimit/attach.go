package destlimit

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
)

// Attach 取得或创建本进程的限额引擎，并铺上默认宽策略。

  func Attach(rt *app.Runtime) *Engine {
	   if rt == nil {
		  e := New()
		  _, _ = e.UseDefaultPolicy()
		  return e
	   }
	  if e, ok := rt.DestLimit.(*Engine); ok && e != nil {
		   return e
	  }
	   e := New()
	   if rt.SQL != nil && !rt.Mem {
		   e.BindSQL(rt.SQL)
	  }
	   _, _ = e.UseDefaultPolicy()
	  rt.DestLimit = e
	  return e
   }

// FromRuntime 读取已挂载引擎。未挂载返回 nil，不隐式创建。
   func FromRuntime(rt *app.Runtime) *Engine {
	   if rt == nil {
		  return nil
	   }
	  e, _ := rt.DestLimit.(*Engine)
	   return e
  }

// StartBackground 在风控 worker 进程里按分钟推进一步。
   func StartBackground(rt *app.Runtime) {
	  e := Attach(rt)
	   go Loop(context.Background(),
		  e, time.Minute)
   }
