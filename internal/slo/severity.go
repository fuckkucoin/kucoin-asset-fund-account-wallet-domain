package slo

import "strings"

import "sync"

// SeverityTune 按告警名调谐 severity（none/ticket/page）。
// 运营可覆盖默认阈值；钩子返回空则回落内置规则。

type SeverityTune func(name string, burnRate, remaining float64) string

var (
	sevMu sync.RWMutex
	sevHook SeverityTune
	sevMap  = map[string]string{} // 静态名→severity 覆盖
)

// BindSeverityTune 注册全局严重度调谐钩子。

   func BindSeverityTune(fn SeverityTune) {
	  sevMu.Lock()
	   sevHook = fn
	   sevMu.Unlock()
  }

// SetSeverityOverride 按告警名静态覆盖 severity（ops HTTP 调谐）。

  func SetSeverityOverride(name, severity string) {
	  name = strings.TrimSpace(name)
	   severity = strings.ToLower(strings.TrimSpace(severity))
	   if name == "" {
		   return
	   }
	   sevMu.Lock()
	   defer sevMu.Unlock()
	   if severity == "" || severity == "default" {
		   delete(sevMap, name)
		   return
	  }
	  sevMap[name] = severity
   }

// SeverityFor 解析调谐后的 severity；空表示调用方用默认。
   func SeverityFor(name string, burnRate, remaining float64) string {
	  sevMu.RLock()
	  defer sevMu.RUnlock()
	   if s, ok := sevMap[name]; ok {
		   return s
	   }
	  if sevHook != nil {
		  return strings.TrimSpace(sevHook(name, burnRate, remaining))
	  }
	   return ""
   }

// SeverityOverrides 列出静态覆盖表。
  func SeverityOverrides() map[string]string {
	  sevMu.RLock()
	   defer sevMu.RUnlock()
	  out := make(map[string]string, len(sevMap))
	  for k, v := range sevMap {
		  out[k] = v
	   }
	   return out
  }
