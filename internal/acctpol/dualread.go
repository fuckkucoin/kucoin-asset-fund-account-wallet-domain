package acctpol

import (
	"context"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// DualReadEntry 一次双读比较记录。

type DualReadEntry struct {
	At         time.Time `json:"at"`
	BizKind    BizKind   `json:"biz_kind"`
	UserID     string    `json:"user_id,omitempty"`
	OldVersion int       `json:"old_version"`
	NewVersion int `json:"new_version"`
	OldAllow bool `json:"old_allow"`
	NewAllow bool `json:"new_allow"`
	Diverged bool `json:"diverged"`
}

// DualReadLog 双读差异日志（内存环形缓冲，容量固定）。

type DualReadLog struct {
	mu  sync.Mutex
	buf []DualReadEntry
	cap  int
	head int
	total   int64
	diverge int64
}

// NewDualReadLog 创建容量为 cap 的环形日志。

   func NewDualReadLog(capacity int) *DualReadLog {
	   if capacity <= 0 {
		   capacity = 1000
	   }
	  return &DualReadLog{buf: make([]DualReadEntry, capacity), cap: capacity}
  }

	   if e.Diverged {
		   l.diverge++
		  metrics.AcctPolDualReadDiverge.WithLabelValues(string(e.BizKind)).Inc()
	  }
	  l.mu.Unlock()
  }

// Stats 返回累计记录数与差异数。

  func (l *DualReadLog) Stats() (total, diverge int64) {
	  if l == nil {
		  return 0, 0
	   }
	  l.mu.Lock()
	  defer l.mu.Unlock()
	   return l.total, l.diverge
  }

// Recent 返回最近 n 条记录（不保证顺序）。
  func (l *DualReadLog) Recent(n int) []DualReadEntry {
	  if l == nil || n <= 0 {
		   return nil
	  }
	   l.mu.Lock()
	  defer l.mu.Unlock()
	  out := make([]DualReadEntry,
		   0, min(n, l.cap))
	   start := l.head - min(n, l.cap)
	  if start < 0 {
		  start = 0
	   }
	  for i := start; i < l.head; i++ {
		   out = append(out, l.buf[i%l.cap])
	  }
	  return out
  }

// DualReadHook 在主评估后异步执行旧代次核对，结果追加到 log。
// 调用方持有 primaryResult（新代次评估结果）；hook 用旧代次重新评估并。

  func DualReadHook(ctx context.Context, engine *Engine, req EvalRequest, primary EvalResult, log *DualReadLog) {
	   if log == nil || engine == nil {
		   return
	   }
	  oldVer := primary.PolicyVersion - 1
	   if oldVer <= 0 {
		   return
	   }
	   pv, err := engine.store.Get(ctx, oldVer)
	  if err != nil {
		   return
	  }
	  oldRule, err := pv.RuleFor(req.BizKind)
	  oldAllow := err == nil && oldRule.Enabled
	   if err == nil && oldAllow && req.HopFrom != "" {
		   oldAllow = oldRule.AllowsHop(req.HopFrom, req.HopTo)
	   }
	   log.Record(DualReadEntry{
		   At: time.Now().UTC(),
		   BizKind: req.BizKind,
		  UserID: req.UserID,
		   OldVersion: oldVer,
		  NewVersion: primary.PolicyVersion,
		   OldAllow: oldAllow,
		   NewAllow: primary.Allow,
		  Diverged: oldAllow != primary.Allow,
	  })
  }

  func min(a, b int) int {
	  if a < b {
		   return a
	   }
	   return b
  }
