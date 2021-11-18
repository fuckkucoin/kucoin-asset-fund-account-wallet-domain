package reviewsla

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// Ticket SLA 观测票。BreachGen 在首次 Scan 到违约时签发，后续幂等。

type Ticket struct {
	CaseID     string `json:"case_id"`
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	OpenedAt time.Time `json:"opened_at"`
	Deadline time.Time `json:"deadline"`
	Breached bool      `json:"breached"`
	Closed   bool      `json:"closed"`
	State string `json:"state"`
	// BreachGen 违约世代令牌；非空表示已签发 breach_gen，需 breachclosegate 建档挡写。
	BreachGen string `json:"breach_gen,omitempty"`
	// BreachedAt 首次标为 Breached 的时刻。
	BreachedAt time.Time `json:"breached_at,omitempty"`
}

type Book struct {
	mu    sync.Mutex
	ttl   time.Duration
	clock func() time.Time
	rows  map[string]*Ticket
}

   func New(ttl time.Duration) *Book {
	  if ttl <= 0 {
		   ttl = 4 * time.Hour
	  }
	   return &Book{ttl: ttl, clock: func() time.Time { return time.Now().UTC() }, rows: map[string]*Ticket{}}
  }

   func (b *Book) WithClock(c func() time.Time) *Book {
	   if b != nil && c != nil {
		   b.clock = c
	  }
	   return b
   }

   // Track 立案时挂上截止。同一案重复 Track 不改 Deadline。
  func (b *Book) Track(c cases.Case) Ticket {
	   if b == nil {
		   return Ticket{}
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   if cur, ok := b.rows[c.ID]; ok {
		  cp := *cur
		   return cp
	   }
	  now := b.clock()
	  t := &Ticket{
		  CaseID: c.ID, MovementID: c.MovementID, UserID: c.UserID,
		   OpenedAt: now, Deadline: now.Add(b.ttl), State: string(c.State),
	  }
	   b.rows[c.ID] = t
	   cp := *t
	   return cp
   }

   func (b *Book) Close(caseID string, state string) {
	   if b == nil {
		  return
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if t, ok := b.rows[caseID]; ok {
		   t.Closed = true
		  t.State = state
	  }
  }

// Scan 把过了 Deadline 仍未关的票标成违约，并为每张新违约票签发 breach_gen 令牌。
// 返回本拍新违约（含 BreachGen 字段），供 breachclosegate Worker 创建 BreachLease
// 并驱动 cases.Expire；Scan 本身不调用 Expire，不调用 Decide(allow)。

  func (b *Book) Scan() []Ticket {
	   if b == nil {
		   return nil
	   }
	   now := b.clock()
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  out := make([]Ticket, 0)
	  for _, t := range b.rows {
		  if t.Closed || t.Breached {
			   continue
		   }
		   if now.Before(t.Deadline) {
			  continue
		   }
		  t.Breached = true
		  t.State = "breached"
		   // 签发 breach_gen（幂等：已有则复用）。
		   issueBreachGenLocked(t, now)
		  out = append(out, *t)
		   metrics.ReviewSLABreaches.Inc()
	  }
	  return out
   }

// ScanRefs 与 Scan 语义相同，但返回 BreachLeaseRef 切片，方便 breachclosegate 直接消费。
// 只返回本拍新违约的引用；已 Breached 的不重复返回。

  func (b *Book) ScanRefs() []BreachLeaseRef {
	  tickets := b.Scan()
	  refs := make([]BreachLeaseRef, 0, len(tickets))
	   for i := range tickets {
		   if tickets[i].BreachGen != "" {
			  refs = append(refs,
				   toLeaseRef(&tickets[i]))
		  }
	   }
	   return refs
  }

// GetRef 返回指定 caseID 的 BreachLeaseRef（若已违约且有 breach_gen）。
  func (b *Book) GetRef(caseID string) (BreachLeaseRef, bool) {
	   if b == nil {
		   return BreachLeaseRef{}, false
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  t, ok := b.rows[caseID]
	   if !ok || !t.Breached || t.BreachGen == "" {
		   return BreachLeaseRef{}, false
	   }
	  return toLeaseRef(t), true
  }

   func (b *Book) Get(caseID string) (Ticket, bool) {
	  if b == nil {
		  return Ticket{}, false
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  t, ok := b.rows[caseID]
	  if !ok {
		   return Ticket{}, false
	   }
	   return *t, true
  }

   func (b *Book) OpenCount() int {
	   if b == nil {
		   return 0
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   n := 0
	   for _, t := range b.rows {
		  if !t.Closed && !t.Breached {
			  n++
		   }
	   }
	  return n
   }

  func (b *Book) List(limit int) []Ticket {
	  if b == nil {
		  return nil
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  if limit <= 0 {
		   limit = 50
	   }
	   out := make([]Ticket, 0, limit)
	  for _, t := range b.rows {
		   out = append(out, *t)
		  if len(out) >= limit {
			  break
		  }
	   }
	   return out
  }
