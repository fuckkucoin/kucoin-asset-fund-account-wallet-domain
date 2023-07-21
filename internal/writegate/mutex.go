package writegate

import (
	"strings"
	"sync"
	"time"
)

// ProbeState 单个交易对的探针状态。
type PairProbe struct {
	// MarkedAt 探针标记时刻（UTC）。
	MarkedAt time.Time `json:"marked_at"`
	// Probing true = 当前处于 half-open 探针状态。
	Probing bool `json:"probing"`
}

// ProbeMutex 跨交易对探针互斥锁注册中心。

type ProbeMutex struct {
	mu sync.RWMutex
	pairs map[string]PairProbe
	clock func() time.Time
}

// NewProbeMutex 构造探针互斥锁注册中心。

   func NewProbeMutex(clock func() time.Time) *ProbeMutex {
	   if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &ProbeMutex{
		pairs: map[string]PairProbe{},
		clock: clock,
	}
}

   func normKey(pair string) string { return strings.ToUpper(strings.TrimSpace(pair)) }

// MarkProbe 将指定交易记为半开探针状态。
// oracle Guard 在发出 half-open 探针 tick 之前调用。
  func (m *ProbeMutex) MarkProbe(pair string) {
	   if m == nil {
		   return
	  }
	  key := normKey(pair)
	   m.mu.Lock()
	   m.pairs[key] = PairProbe{MarkedAt: m.clock(), Probing: true}
	   m.mu.Unlock()
   }

// ClearProbe 清除指定交易对的探针标记（幂等）。
// oracle Guard 在探针 tick 完成（成功/失败）后调用。

  func (m *ProbeMutex) ClearProbe(pair string) {
	   if m == nil {
		   return
	   }
	   key := normKey(pair)
	   m.mu.Lock()
	  delete(m.pairs, key)
	   m.mu.Unlock()
   }

// IsProbing 查询指定交易对是否处于探针状态。

  func (m *ProbeMutex) IsProbing(pair string) bool {
	   if m == nil {
		   return false
	   }
	   key := normKey(pair)
	   m.mu.RLock()
	  s, ok := m.pairs[key]
	  m.mu.RUnlock()
	  return ok && s.Probing
  }

// Snapshot 返回当前所有探针状态快照。

   func (m *ProbeMutex) Snapshot() map[string]PairProbe {
	   if m == nil {
		   return nil
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   out := make(map[string]PairProbe,
		  len(m.pairs))
	  for k, v := range m.pairs {
		   out[k] = v
	  }
	  return out
  }
