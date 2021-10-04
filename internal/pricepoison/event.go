package pricepoison

import (
	"sync"
	"time"
)

// PoisonEvent 单条毒价检测事件。
type PoisonEvent struct {
	// ID 进程内自增标识符（1-based，重启后归零）。
	ID uint64 `json:"id"`
	// Pair 交易对，形态 "ASSET/QUOTE"（大写，已规范化）。
	Pair string `json:"pair"`
	// TickPrice 触发毒价的 tick 价格（字符串，保留精度）。
	TickPrice string `json:"tick_price"`
	// TickAt tick 的时间戳（UTC）。
	TickAt time.Time `json:"tick_at"`
	// Detail 分级细节。
	Detail GradeDetail `json:"detail"`
	// GenerationRevoked 本次事件导致撤销的最大令牌代次；0 表示无活跃令牌被撤销。
	GenerationRevoked uint64 `json:"generation_revoked,omitempty"`
	// DetectedAt 检测时刻（UTC）。
	DetectedAt time.Time `json:"detected_at"`
}

// EventStore 进程内毒价事件仓库（有限容量 FIFO 环形缓冲）。

type EventStore struct {
	mu sync.Mutex
	events []PoisonEvent
	capacity int
	seq      uint64
}

// NewEventStore 构造容量为 cap 的事件仓库。cap <= 0 时使用默认值 4096。
   func NewEventStore(cap int) *EventStore {
	  if cap <= 0 {
		  cap = 4096
	   }
	   return &EventStore{
		  events: make([]PoisonEvent, 0, cap),
		  capacity: cap,
	   }
   }

// Append 追加毒价事件；容量满时淘汰最旧记录（FIFO）。
// 自动填充 ID 与 DetectedAt（零值时使用当前 UTC）。

   func (s *EventStore) Append(e PoisonEvent) PoisonEvent {
	   if s == nil {
		  return e
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  s.seq++
	  e.ID = s.seq
	   if e.DetectedAt.IsZero() {
		   e.DetectedAt = time.Now().UTC()
	  }
	   if len(s.events) >= s.capacity {
		   // FIFO：移除最旧；切片尾部追加新记录。
		   copy(s.events, s.events[1:])
		   s.events[len(s.events)-1] = e
	   } else {
		  s.events = append(s.events, e)
	   }
	   return e
   }

// List 返回当前所有事件的快照（旧→新排列）。

   func (s *EventStore) List() []PoisonEvent {
	  if s == nil {
		  return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]PoisonEvent, len(s.events))
	   copy(out, s.events)
	  return out
  }

// ListByPair 返回指定交易对的事件快照（旧→新排列）。

  func (s *EventStore) ListByPair(pair string) []PoisonEvent {
	  if s == nil {
		  return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   var out []PoisonEvent
	  for _, e := range s.events {
		  if e.Pair == pair {
			   out = append(out, e)
		  }
	   }
	   return out
   }

// LastByPair 返回指定交易对最新一条毒价事件；未找到时 ok=false。

   func (s *EventStore) LastByPair(pair string) (PoisonEvent, bool) {
	  if s == nil {
		   return PoisonEvent{}, false
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   for i := len(s.events) - 1; i >= 0; i-- {
		   if s.events[i].Pair == pair {
			   return s.events[i], true
		  }
	   }
	   return PoisonEvent{}, false
   }

// Len 返回当前存储的事件数。

   func (s *EventStore) Len() int {
	   if s == nil {
		   return 0
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  return len(s.events)
  }
