package floatdesk

import (
	"strings"
	"sync"
)

// RefillObserve 补仓是否进行中的只读挂钩。
// 用途：egressleasegate 可配置在补仓进行中拒绝签发新 lease。
// 禁区：不得替代 RefillMutex / PlanRefillLocked；本结构无代次、无锁序号。
type RefillObserve struct {
	mu     sync.Mutex
	active map[string]bool
}

// NewRefillObserve 创建空观察表。
   func NewRefillObserve() *RefillObserve {
	   return &RefillObserve{active: map[string]bool{}}
  }

  func refillKey(chain, native string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(native))
   }

// Note leftover 标记补仓开始或结束。不分配代次。

  func (o *RefillObserve) Note(chain, native string, active bool) error {
	   if o == nil {
		   return ErrUnavailable
	  }
	  o.SetActive(chain, native, active)
	  return nil
   }

// SetActive 运维标记补仓进行中。非补仓主链写路径。

  func (o *RefillObserve) SetActive(chain, native string, on bool) {
	  if o == nil {
		  return
	   }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	   k := refillKey(chain, native)
	   if on {
		  o.active[k] = true
		   return
	  }
	  delete(o.active, k)
  }

// IsActive 只读：指定链原生币是否标记为补仓进行中。

  func (o *RefillObserve) IsActive(chain, native string) bool {
	   if o == nil {
		  return false
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   return o.active[refillKey(chain,
		   native)]
   }

// AnyActive 是否存在任一补仓进行中标记。
   func (o *RefillObserve) AnyActive() bool {
	   if o == nil {
		   return false
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   return len(o.active) > 0
  }
