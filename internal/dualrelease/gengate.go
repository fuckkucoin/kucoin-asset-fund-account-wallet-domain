package dualrelease

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// GenGate 并发世代互斥门卫（内存 CAS，用于单进程并发）。
// 场景中 gen 检查依赖数据库行级 CAS（sqlrepo 实现）。
type GenGate struct {
	mu sync.Mutex
	cur map[string]int64 // key: requestID → 当前世代
}

// NewGenGate 初始化内存世代门卫。
   func NewGenGate() *GenGate {
	   return &GenGate{cur: map[string]int64{}}
  }

// Bump 原子递增世代并返回新世代号。

   func (g *GenGate) Bump(requestID string) int64 {
	   g.mu.Lock()
	   defer g.mu.Unlock()
	   g.cur[requestID]++
	  return g.cur[requestID]
   }

// Check 校验调用方持有的 gen 是否等于当前世代。
// gen 不符 → ErrStaleGen（旧世代请求被拒，无副作用）。
   func (g *GenGate) Check(requestID string, gen int64) error {
	   g.mu.Lock()
	   defer g.mu.Unlock()
	   cur, ok := g.cur[requestID]
	   if !ok {
		   return fmt.Errorf("dualrelease: gen gate: no record for request %s", requestID)
	  }
	   if cur != gen {
		  return fmt.Errorf("%w: request %s cur=%d got=%d", ErrStaleGen, requestID, cur, gen)
	  }
	  return nil
  }

// Current 返回 requestID 的当前世代号。

  func (g *GenGate) Current(requestID string) (int64, bool) {
	   g.mu.Lock()
	  defer g.mu.Unlock()
	   v, ok := g.cur[requestID]
	   return v, ok
  }

// ──────────────────────────────────────────────────
// AtomicGen 单个请求的原子世代计数器（无锁，用于高并发压测）
// ──────────────────────────────────────────────────
// AtomicGen 基于 atomic.Int64 的单请求世代计数器。
type AtomicGen struct {
	v atomic.Int64
}

// Next 递增并返回新值。

   func (a *AtomicGen) Next() int64 { return a.v.Add(1) }

// Load 读取当前值。

   func (a *AtomicGen) Load() int64 { return a.v.Load() }

// CAS 尝试从 old 换到 new；只有一个并发调用能成功。
   func (a *AtomicGen) CAS(old,
	   new int64) bool {
	  return a.v.CompareAndSwap(old, new)
   }

// ──────────────────────────────────────────────────
// GenRaceResult 并发竞态结果（用于断言）
// ──────────────────────────────────────────────────
// GenRaceResult 并发 Execute 竞态结果统计。
type GenRaceResult struct {
	Winners int // 成功获世代的数量（应恒为 1）
	Losers int // ErrStaleGen 的数量
	Errors int // 其他错误数量
}

// VerifyRace 断言竞态结果：恰好一个赢家，其余均为 ErrStaleGen。

   func VerifyRace(r GenRaceResult, total int) error {
	  if r.Winners != 1 {
		   return fmt.Errorf("gen race: expected 1 winner, got %d", r.Winners)
	   }
	   if r.Losers != total-1 {
		  return fmt.Errorf("gen race: expected %d losers, got %d", total-1, r.Losers)
	   }
	   if r.Errors != 0 {
		  return fmt.Errorf("gen race: unexpected errors: %d", r.Errors)
	   }
	  return nil
   }
