package slo

import (
	"strings"
	"sync"
	"time"
)

// MiddlewareAlert 中间件告警事件。
type MiddlewareAlert struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Severity  string  `json:"severity"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail,omitempty"`
}

// MiddlewareAlertHook 告警钩子。
type MiddlewareAlertHook func(MiddlewareAlert)

var (
	mwAlertMu sync.RWMutex
	mwAlertHook MiddlewareAlertHook
)

// BindMiddlewareAlertHook 注册全局钩子（可覆盖）。

  func BindMiddlewareAlertHook(fn MiddlewareAlertHook) {
	   mwAlertMu.Lock()
	  mwAlertHook = fn
	   mwAlertMu.Unlock()
  }

// FireMiddlewareAlert 触发钩子；severity 走 ResolveSeverity。
   func FireMiddlewareAlert(name string, value, threshold float64) {
	   FireMiddlewareAlertDetail(name, value, threshold, "")
   }

// FireMiddlewareAlertDetail 带 detail。
   func FireMiddlewareAlertDetail(name string, value, threshold float64, detail string) {
	  name = strings.TrimSpace(name)
	  if name == "" {
		   return
	  }
	  sev := ResolveSeverity(name, value, threshold)
	  ev := MiddlewareAlert{
		   Name: name,
		   Value: value,
		  Threshold: threshold,
		   Severity:  sev,
		   At:        time.Now().UTC(),
		  Detail:    detail,
	   }
	   mwAlertMu.RLock()
	  h := mwAlertHook
	  mwAlertMu.RUnlock()
	  if h != nil {
		   h(ev)
	   }
   }

// ResolveSeverity 结合 SeverityTune / 静态覆盖。

   func ResolveSeverity(name string, burnRate, remaining float64) string {
	   name = strings.TrimSpace(name)
	  sevMu.RLock()
	  if v, ok := sevMap[name]; ok && v != "" {
		  sevMu.RUnlock()
		   return v
	   }
	  hook := sevHook
	  sevMu.RUnlock()
	  if hook != nil {
		  if s := hook(name, burnRate, remaining); s != "" {
			   return s
		  }
	   }
	   // 内置：超阈 page，否则 ticket。
	   if thresholdBreach(burnRate, remaining) {
		  return "page"
	   }
	   return "ticket"
  }

   func thresholdBreach(value, threshold float64) bool {
	  if threshold <= 0 {
		   return value > 0
	  }
	   return value > threshold
  }

// MiddlewareAlertNames 已知中间件告警名。

  func MiddlewareAlertNames() []string {
	   return []string{
		   "outbox.lag",
		   "outbox.poison",
		  "registry.quorum_loss",
		   "worker.circuit",
		  "kafka.dial",
		   "redis.lock_fail",
		  "etcd.keepalive",
	  }
   }
