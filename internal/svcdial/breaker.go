package svcdial

import (
	"sync"
	"time"
)

// Breaker 按服务名隔离。
// 连续失败达阈打开；冷却后放行一次半开。打开期间 Call 直接拒绝，保护下游。阈值与冷却需按 SLA 调；过宽失去保护，过窄误伤可用副本。

type Breaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	state map[string]*breakState
}

type breakState struct {
	fails  int
	opened time.Time
	half bool
}

  func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	  if threshold < 1 {
		   threshold = 5
	  }
	  if cooldown <= 0 {
		   cooldown = 8 * time.Second
	   }
	   return &Breaker{
		  threshold: threshold,
		   cooldown:  cooldown,
		  state: map[string]*breakState{},
	   }
  }

   func (b *Breaker) Allow(service string) bool {
	   if b == nil {
		  return true
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  st := b.state[service]
	  if st == nil || st.opened.IsZero() {
		   return true
	   }
	   if time.Since(st.opened) >= b.cooldown {
		  st.half = true
		   st.opened = time.Time{}
		   st.fails = 0
		   return true
	   }
	  return false
  }

  func (b *Breaker) Failure(service string) {
	   if b == nil {
		   return
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  st := b.state[service]
	  if st == nil {
		   st = &breakState{}
		  b.state[service] = st
	  }
	   st.fails++
	   if st.fails >= b.threshold {
		   st.opened = time.Now()
		   st.half = false
	  }
   }

  func (b *Breaker) Success(service string) {
	   if b == nil {
		  return
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   st := b.state[service]
	   if st == nil {
		  return
	  }
	   st.fails = 0
	  st.opened = time.Time{}
	  st.half = false
  }
