package fiatjnl

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// GenSeq 凭证世代序号，单调递增。
type GenSeq uint64

// JournalGen 记录单次世代抬升事件；提供给 fiatbalgate 只读绑定。
type JournalGen struct {
	// Seq 本次抬升后的世代号。
	Seq GenSeq `json:"seq"`
	// EntryID 触发抬升的凭证 ID。
	EntryID string `json:"entry_id"`
	// Kind 触发抬升的操作种类（post / adjust）。
	Kind string `json:"kind"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// BumpedAt 世代抬升时间戳（UTC）。
	BumpedAt time.Time `json:"bumped_at"`
}

// GenTracker 线程安全的世代计数器，嵌入 fiatjnl.Service 后通过方法组暴露。
// 外包只读引用通过 History / ActiveSeq 获取；不得直接操作内部字段。

type GenTracker struct {
	// seq 当前世代号（atomic 操作，无锁读写）。
	seq uint64
	// mu 保护 history 列表。
	mu      sync.RWMutex
	history []JournalGen
}

const maxGenHistory = 512

// newGenTracker 构造世代追踪器，起始世代为 0（未入账）。
   func newGenTracker() *GenTracker {
	   return &GenTracker{
		  history: make([]JournalGen, 0, 64),
	   }
   }

// BumpGen 抬升世代号，返回新世代快照。由 Post/Adjust 成功路径调用。
// clock 由 Service.clock 传入，确保时间戳一致性。
   func (g *GenTracker) BumpGen(entryID, kind, asset string, clock func() time.Time) JournalGen {
	  next := atomic.AddUint64(&g.seq, 1)
	  gen := JournalGen{
		   Seq:     GenSeq(next),
		   EntryID: entryID,
		  Kind: kind,
		   Asset: asset,
		  BumpedAt: clock(),
	   }
	  g.mu.Lock()
	   if len(g.history) >= maxGenHistory {
		   // 滑动窗口：保留后半段，避免无界增长。
		  half := maxGenHistory / 2
		   copy(g.history[:half],
			g.history[maxGenHistory-half:])
		g.history = g.history[:half]
	}
	g.history = append(g.history, gen)
	g.mu.Unlock()
	return gen
}

// ActiveSeq 返回当前最新世代号（只读，无锁）。
   func (g *GenTracker) ActiveSeq() GenSeq {
	  return GenSeq(atomic.LoadUint64(&g.seq))
  }

// History 返回最近 n 条世代快照副本（线程安全）。n <= 0 返回全部缓存记录。
   func (g *GenTracker) History(n int) []JournalGen {
	  g.mu.RLock()
	   defer g.mu.RUnlock()
	  src := g.history
	  if n > 0 && n < len(src) {
		   src = src[len(src)-n:]
	  }
	  out := make([]JournalGen, len(src))
	   copy(out, src)
	   return out
  }

// String 可打印表示（调试/日志用）。

  func (s GenSeq) String() string { return fmt.Sprintf("gen#%d", uint64(s)) }
