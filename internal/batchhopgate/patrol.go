package batchhopgate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// OccupyHold 是未闭合残留期间的科目占用记录。
// 同用户同资产并行 BatchHop 检查时读取此内存镜像。
type OccupyHold struct {
	ResidualCaseID string `json:"residual_case_id"`
	UserID         string `json:"user_id"`
	Asset          string `json:"asset"`
	Generation int64    `json:"generation"`
	HopCount   int      `json:"hop_count"`
	Books      []string `json:"books"`
	HeldAt time.Time `json:"held_at"`
}

// PartialConserveEngine 部分守恒与占用巡检。

type PartialConserveEngine struct {
	mu    sync.Mutex
	holds map[string]*OccupyHold
	clock func() time.Time
}

// NewPartialConserveEngine 构造巡检引擎。
   func NewPartialConserveEngine() *PartialConserveEngine {
	   return &PartialConserveEngine{
		  holds: make(map[string]*OccupyHold),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (e *PartialConserveEngine) WithClock(c func() time.Time) *PartialConserveEngine {
	  if e != nil && c != nil {
		  e.clock = c
	   }
	  return e
  }

  func holdKey(userID, asset string) string {
	  return strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

// PinHold 在残留案落库后钉住占用：未释放前同键并行 Batch 视为占用冲突。

  func (e *PartialConserveEngine) PinHold(c *BatchResidualCase) *OccupyHold {
	   if e == nil || c == nil {
		  return nil
	  }
	   books := uniqueBooks(c.SucceededHops)
	   h := &OccupyHold{
		  ResidualCaseID: c.ID,
		   UserID:         c.UserID,
		  Asset:          c.Asset,
		   Generation:     c.Generation,
		  HopCount: len(c.SucceededHops),
		  Books:    books,
		  HeldAt: e.clock(),
	  }
	   e.mu.Lock()
	   e.holds[holdKey(c.UserID, c.Asset)] = h
	   e.mu.Unlock()
	  return h
  }

// ReleaseHold 残留闭合后释放占用。

  func (e *PartialConserveEngine) ReleaseHold(userID, asset string) {
	  if e == nil {
		  return
	  }
	  e.mu.Lock()
	   delete(e.holds, holdKey(userID, asset))
	  e.mu.Unlock()
   }

// HasHold 查询是否存在未释放占用。

   func (e *PartialConserveEngine) HasHold(userID, asset string) bool {
	   if e == nil {
		  return false
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   _, ok := e.holds[holdKey(userID, asset)]
	   return ok
  }

// GetHold 只读占用快照。

   func (e *PartialConserveEngine) GetHold(userID, asset string) (*OccupyHold, bool) {
	   if e == nil {
		   return nil, false
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   h, ok := e.holds[holdKey(userID, asset)]
	  if !ok {
		   return nil, false
	   }
	  cp := *h
	  return &cp, true
   }

// ListHolds 列出全部占用（供 ops 巡检）。

  func (e *PartialConserveEngine) ListHolds() []OccupyHold {
	  if e == nil {
		  return nil
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  out := make([]OccupyHold, 0, len(e.holds))
	  for _, h := range e.holds {
		  out = append(out, *h)
	  }
	   sort.Slice(out, func(i, j int) bool {
		   if out[i].UserID != out[j].UserID {
			   return out[i].UserID < out[j].UserID
		  }
		   return out[i].Asset < out[j].Asset
	  })
	  return out
  }

// AssertPartialConserve 对已成功 hop 做科目方向配对检查。
// 规则：
// - 空集视为守恒（第 0 笔即失败）；
// - 每条 hop 必须 from != to 且 amount 非空；
// - 同一 asset 上 from 出现次数与 to 出现次数之和为 hop 数的两倍（每 hop 两端各计 1）。

  func AssertPartialConserve(hops []HopSummary) error {
	  if len(hops) == 0 {
		  return nil
	   }
	  type end struct{ book, asset string }
	   fromN := map[end]int{}
	   toN := map[end]int{}
	  for i, h := range hops {
		  if strings.TrimSpace(h.From) == "" || strings.TrimSpace(h.To) == "" {
			  return fmt.Errorf("batchhopgate: hop %d missing book ends", i)
		  }
		  if strings.EqualFold(h.From, h.To) {
			   return fmt.Errorf("batchhopgate: hop %d same book %s", i, h.From)
		   }
		  if strings.TrimSpace(h.Amount) == "" {
			   return fmt.Errorf("batchhopgate: hop %d empty amount", i)
		   }
		  fromN[end{h.From, h.Asset}]++
		   toN[end{h.To, h.Asset}]++
	   }
	   totalEnds := 0
	   for _, n := range fromN {
		  totalEnds += n
	   }
	  for _, n := range toN {
		  totalEnds += n
	  }
	   if totalEnds != len(hops)*2 {
		   return fmt.Errorf("batchhopgate: end count %d != hops*2 %d", totalEnds, len(hops)*2)
	   }
	  return nil
   }

   func uniqueBooks(hops []HopSummary) []string {
	  seen := map[string]bool{}
	  out := make([]string, 0)
	  for _, h := range hops {
		   for _, b := range []string{h.From, h.To} {
			   if b == "" || seen[b] {
				  continue
			   }
			  seen[b] = true
			   out = append(out, b)
		  }
	  }
	   sort.Strings(out)
	  return out
   }

// RebuildHoldFromStore 从残留仓重建占用（worker 启动补偿）。
   func (e *PartialConserveEngine) RebuildHoldFromStore(ctx context.Context, store Store, limit int) (int, error) {
	   if e == nil || store == nil {
		  return 0, ErrUnavailable
	   }
	  n := 0
	  for _, st := range []ResidualStatus{ResidualOpen, ResidualProven, ResidualResuming} {
		  list,
			  err := store.ListResidualsByStatus(ctx, st, limit)
		  if err != nil {
			   return n, err
		  }
		   for _, c := range list {
			  e.PinHold(c)
			  n++
		   }
	  }
	   return n, nil
  }

// DiffHoldVsCase 核对占用镜像与残留案 hop 计数；不一致返回说明。
  func DiffHoldVsCase(h *OccupyHold, c *BatchResidualCase) string {
	  if h == nil || c == nil {
		  return "hold or case nil"
	  }
	  if h.ResidualCaseID != c.ID {
		  return "case id mismatch"
	  }
	   if h.HopCount != len(c.SucceededHops) {
		   return fmt.Sprintf("hop count hold=%d case=%d", h.HopCount, len(c.SucceededHops))
	  }
	   if h.Generation != c.Generation {
		   return fmt.Sprintf("generation hold=%d case=%d", h.Generation, c.Generation)
	   }
	   return ""
   }
