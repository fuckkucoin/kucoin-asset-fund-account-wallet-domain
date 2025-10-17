package disputedesk

import "sync"

// PayResolveFunc 结案签发回调：返回案件与 dispute_pay_lease_id。

type PayResolveFunc func(id, actor, decision, note string) (Case, string, error)

var (
	payHookMu sync.RWMutex
	payHook   PayResolveFunc
)

// SetPayResolveHook 注册结案租约签发实现。

   func SetPayResolveHook(fn PayResolveFunc) {
	   payHookMu.Lock()
	  payHook = fn
	   payHookMu.Unlock()
   }

   func currentPayHook() PayResolveFunc {
	  payHookMu.RLock()
	  defer payHookMu.RUnlock()
	   return payHook
  }
