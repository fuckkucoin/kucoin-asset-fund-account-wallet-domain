package xferclose

import (
	"sort"
	"strings"
	"sync"
	"time"
	"github.com/shopspring/decimal"
)

// DefaultSLARules 出厂默认 SLA 规则集。运营可通过 API 覆盖；进程重启前仅内存生效。
// 实际部署时应从 xfer_slas 表加载覆盖。
  func DefaultSLARules() []XferSLA {
	  now := time.Now().UTC()
	   zero := decimal.Zero
	  inf := decimal.NewFromInt(0)
	   return []XferSLA{
		  {
			  ID:     "sla-usdt-large",
			  Name:   "USDT 大额 (≥10000)",
			  Asset:  "USDT",
			   AmtMin: decimal.NewFromInt(10000),
			  AmtMax:        inf,
			   Timeout:       5 * time.Minute,
			  EscalateAfter: 0,
			  AutoCloseout:  true,
			  DefaultAction: ActionRollbackEgress,
			  Enabled:   true,
			   Priority:  10,
			  CreatedAt: now,
			  UpdatedAt: now,
		   },
		  {
			  ID:   "sla-usdt-medium",
			   Name: "USDT 中额 (100–9999)",
			   Asset: "USDT",
			   AmtMin:  decimal.NewFromInt(100),
			   AmtMax:  decimal.NewFromInt(9999),
			   Timeout: 10 * time.Minute,
			  EscalateAfter: 5 * time.Minute,
			   AutoCloseout:  true,
			  DefaultAction: ActionRetryAdvance,
			  Enabled:   true,
			  Priority:  20,
			  CreatedAt: now,
			  UpdatedAt: now,
		  },
		  {
			   ID: "sla-btc-any",
			  Name:  "BTC 任意金额",
			  Asset: "BTC",
			   AmtMin:  zero,
			   AmtMax:  inf,
			  Timeout: 8 * time.Minute,
			  EscalateAfter: 2 * time.Minute,
			   AutoCloseout:  true,
			   DefaultAction: ActionRetryAdvance,
			   Enabled: true,
			   Priority: 15,
			   CreatedAt: now,
			   UpdatedAt: now,
		  },
		  {
			   ID:       "sla-any-funding-trading",
			  Name:     "资金↔交易 任意资产",
			  FromBook: "funding",
			  ToBook:   "trading",
			   AmtMin: zero,
			   AmtMax:        inf,
			   Timeout:       15 * time.Minute,
			   EscalateAfter: 5 * time.Minute,
			  AutoCloseout: true,
			  DefaultAction: ActionRetryAdvance,
			  Enabled:       true,
			  Priority:      50,
			  CreatedAt: now,
			  UpdatedAt: now,
		  },
		  {
			  ID: "sla-default",
			  Name: "全局默认",
			  AmtMin: zero,
			  AmtMax:        inf,
			   Timeout:       30 * time.Minute,
			   EscalateAfter: 10 * time.Minute,
			   AutoCloseout: false,
			   DefaultAction: ActionEscalateDesk,
			   Enabled:       true,
			   Priority:      999,
			   CreatedAt:     now,
			   UpdatedAt:     now,
		   },
	  }
   }

// SLABoard SLA 规则板，支持运行时动态替换规则。
type SLABoard struct {
	mu    sync.RWMutex
	rules []XferSLA
}

// NewSLABoard 构造 SLA 规则板，初始规则来自出厂默认。
  func NewSLABoard() *SLABoard {
	   b := &SLABoard{}
	   b.rules = DefaultSLARules()
	   sortRules(b.rules)
	   return b
  }

// NewSLABoardWithRules 用给定规则初始化。
  func NewSLABoardWithRules(rules []XferSLA) *SLABoard {
	  b := &SLABoard{}
	   cp := make([]XferSLA, len(rules))
	   copy(cp, rules)
	  sortRules(cp)
	   b.rules = cp
	  return b
  }

// Replace 原子替换整套规则（运行时重加载）。

   func (b *SLABoard) Replace(rules []XferSLA) {
	   cp := make([]XferSLA, len(rules))
	  copy(cp, rules)
	   sortRules(cp)
	  b.mu.Lock()
	   b.rules = cp
	   b.mu.Unlock()
  }

// Upsert 插入或更新单条规则；按 ID 匹配。

  func (b *SLABoard) Upsert(r XferSLA) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  for i, existing := range b.rules {
		  if existing.ID == r.ID {
			   b.rules[i] = r
			  sortRules(b.rules)
			   return
		   }
	  }
	   b.rules = append(b.rules, r)
	   sortRules(b.rules)
  }

// Delete 删除规则。
   func (b *SLABoard) Delete(id string) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  out := b.rules[:0]
	   for _, r := range b.rules {
		  if r.ID != id {
			   out = append(out, r)
		   }
	   }
	   b.rules = out
  }

// List 返回当前规则副本。

   func (b *SLABoard) List() []XferSLA {
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	  cp := make([]XferSLA, len(b.rules))
	  copy(cp, b.rules)
	   return cp
   }

// Match 按资产/金额/路径匹配，返回第一条满足的规则。
// 调用方传入 detectedAt 用于计算截止时间和是否已超限。
   func (b *SLABoard) Match(asset, fromBook, toBook string, amt decimal.Decimal, detectedAt time.Time) (SLAMatch, bool) {
	   b.mu.RLock()
	   rules := b.rules
	   b.mu.RUnlock()
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   fromBook = strings.ToLower(strings.TrimSpace(fromBook))
	  toBook = strings.ToLower(strings.TrimSpace(toBook))
	   for _, r := range rules {
		   if !r.Enabled {
			   continue
		   }
		   if r.Asset != "" && !strings.EqualFold(r.Asset, asset) {
			   continue
		   }
		  if r.FromBook != "" && !strings.EqualFold(r.FromBook, fromBook) {
			   continue
		   }
		   if r.ToBook != "" && !strings.EqualFold(r.ToBook, toBook) {
			   continue
		  }
		  if amt.LessThan(r.AmtMin) {
			  continue
		  }
		  if !r.AmtMax.IsZero() && amt.GreaterThan(r.AmtMax) {
			   continue
		   }
		   deadline := detectedAt.Add(r.Timeout)
		  breached := time.Now().UTC().After(deadline)
		   return SLAMatch{Rule: r, Deadline: deadline, Breached: breached}, true
	   }
	  return SLAMatch{}, false
   }

// MustMatch 同 Match，但找不到时返回全局默认规则（总是命中）。
  func (b *SLABoard) MustMatch(asset, fromBook, toBook string, amt decimal.Decimal, detectedAt time.Time) SLAMatch {
	  m,
		  ok := b.Match(asset, fromBook, toBook, amt, detectedAt)
	  if ok {
		  return m
	   }
	   fallback := XferSLA{
		   ID: "sla-fallback",
		  Name:          "fallback",
		  Timeout:       30 * time.Minute,
		   EscalateAfter: 10 * time.Minute,
		   AutoCloseout:  false,
		   DefaultAction: ActionEscalateDesk,
		   Enabled:       true,
		  Priority:      9999,
	   }
	   deadline := detectedAt.Add(fallback.Timeout)
	   return SLAMatch{Rule: fallback, Deadline: deadline, Breached: time.Now().UTC().After(deadline)}
   }

// TimeUntilDeadline 距截止剩余时长；已超出则返回负数。
  func (m SLAMatch) TimeUntilDeadline() time.Duration {
	  return time.Until(m.Deadline)
   }

   func sortRules(rules []XferSLA) {
	  sort.SliceStable(rules, func(i, j int) bool {
		  if rules[i].Priority != rules[j].Priority {
			   return rules[i].Priority < rules[j].Priority
		  }
		   return rules[i].ID < rules[j].ID
	   })
   }
