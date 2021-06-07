package fundcircuit

import (
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// ProbeDesk 探针台。同一熔断键同时只允许一枚 pending 探针。
type ProbeDesk struct {
	mu sync.Mutex
	rows     map[string]*Probe
	inflight map[string]string
	clock    func() time.Time
	onSave func(Probe)
	issued int64
	passed  int64
	failed  int64
	expired int64
}

// NewProbeDesk 建探针台。

   func NewProbeDesk() *ProbeDesk {
	  return &ProbeDesk{
		   rows: map[string]*Probe{}, inflight: map[string]string{},
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
   func (d *ProbeDesk) WithClock(c func() time.Time) *ProbeDesk {
	   if d != nil && c != nil {
		   d.mu.Lock()
		  d.clock = c
		  d.mu.Unlock()
	  }
	  return d
   }

  // OnSave 落库回调，锁外执行。

  func (d *ProbeDesk) OnSave(fn func(Probe)) *ProbeDesk {
	   if d != nil {
		   d.mu.Lock()
		  d.onSave = fn
		  d.mu.Unlock()
	  }
	   return d
  }

   func (d *ProbeDesk) now() time.Time {
	  if d == nil || d.clock == nil {
		   return time.Now().UTC()
	   }
	  return d.clock()
  }

// Issue 签发一枚探针。
// 功能：半开期放行一笔试写；试写结论由调用方回报 Settle。
// 重点：同一键已有 pending 探针时返回 ErrProbeBusy，不得并发放两笔试写。
// 风险点：探针过期未回报视为失败，熔断器会重开；值班须按 ProbeTTL 安排试写窗口。
   func (d *ProbeDesk) Issue(c Circuit, path Path, actor string, ttl time.Duration) (Probe, error) {
	   if d == nil {
		   return Probe{}, ErrStore
	  }
	   if err := GuardKey(c.Key); err != nil {
		   return Probe{}, err
	   }
	  if c.State != StateHalfOpen {
		   return Probe{}, ErrHalfOpenOnly
	  }
	   if !path.Valid() {
		  return Probe{}, ErrPath
	  }
	  if ttl <= 0 {
		   ttl = DefaultProbeTTL
	  }
	   now := d.now()
	   d.mu.Lock()
	   if id, ok := d.inflight[c.Key]; ok {
		   row, found := d.rows[id]
		  if found && row.Pending() && now.Before(row.ExpiresAt) {
			  d.mu.Unlock()
			   return *row, ErrProbeBusy
		   }
		  if found && row.Pending() {
			  row.State = ProbeExpired
			   row.DecidedAt = now
			  d.expired++
		  }
		   delete(d.inflight, c.Key)
	  }
	   p := &Probe{
		   ID: uuid.NewString(), CircuitKey: c.Key, Path: path, State: ProbePending,
		   Token: persist.NewToken(), Actor: strings.TrimSpace(actor), Round: c.Trips,
		  IssuedAt: now, ExpiresAt: now.Add(ttl),
	   }
	   d.rows[p.ID] = p
	  d.inflight[c.Key] = p.ID
	  d.issued++
	  out := *p
	   fn := d.onSave
	  d.mu.Unlock()
	  if fn != nil {
		   fn(out)
	  }
	  return out, nil
  }

// InFlight 取某键在途探针。
  func (d *ProbeDesk) InFlight(key string) (Probe, bool) {
	   if d == nil {
		  return Probe{}, false
	   }
	  now := d.now()
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   id, ok := d.inflight[key]
	   if !ok {
		  return Probe{}, false
	  }
	  row, found := d.rows[id]
	  if !found || !row.Pending() || !now.Before(row.ExpiresAt) {
		   return Probe{}, false
	   }
	  return *row, true
  }

// Get 读一枚探针。
   func (d *ProbeDesk) Get(id string) (Probe, error) {
	   if d == nil {
		  return Probe{}, ErrStore
	  }
	   d.mu.Lock()
	  defer d.mu.Unlock()
	   row,
		   ok := d.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Probe{}, ErrProbeMissing
	  }
	   return *row, nil
   }

// Settle 回报探针结论。令牌不符一律拒绝，避免旧轮结果覆盖新一轮。

   func (d *ProbeDesk) Settle(id, token string, pass bool, actor, note string) (Probe, error) {
	  if d == nil {
		   return Probe{}, ErrStore
	  }
	   id = strings.TrimSpace(id)
	   now := d.now()
	  d.mu.Lock()
	  row, ok := d.rows[id]
	  if !ok {
		  d.mu.Unlock()
		  return Probe{}, ErrProbeMissing
	  }
	   if strings.TrimSpace(token) != "" && token != row.Token {
		  d.mu.Unlock()
		  return *row, ErrProbeToken
	  }
	  if !row.Pending() {
		  d.mu.Unlock()
		   return *row, ErrProbeState
	  }
	  if !now.Before(row.ExpiresAt) {
		   row.State = ProbeExpired
		  row.DecidedAt = now
		  d.expired++
		   delete(d.inflight, row.CircuitKey)
		  out := *row
		   d.mu.Unlock()
		   return out, ErrProbeState
	   }
	  if pass {
		   row.State = ProbePassed
		  d.passed++
	  } else {
		   row.State = ProbeFailed
		  d.failed++
	  }
	  row.DecidedAt = now
	  row.Note = strings.TrimSpace(note)
	   if a := strings.TrimSpace(actor); a != "" {
		  row.Actor = a
	  }
	   delete(d.inflight, row.CircuitKey)
	   out := *row
	   fn := d.onSave
	  d.mu.Unlock()
	   if fn != nil {
		  fn(out)
	   }
	  return out, nil
   }

// Expire 把过期未回报的探针标为 expired，返回被摘除的键清单。
// 熔断器侧据此重开：探针没结论等同试写失败，不得默认闭合。
   func (d *ProbeDesk) Expire() []Probe {
	  if d == nil {
		   return nil
	   }
	  now := d.now()
	  d.mu.Lock()
	   out := make([]Probe, 0, 4)
	   for key, id := range d.inflight {
		   row, ok := d.rows[id]
		  if !ok {
			   delete(d.inflight, key)
			   continue
		   }
		  if !row.Pending() || now.Before(row.ExpiresAt) {
			  continue
		  }
		  row.State = ProbeExpired
		  row.DecidedAt = now
		   d.expired++
		  delete(d.inflight, key)
		  out = append(out, *row)
	  }
	   fn := d.onSave
	   d.mu.Unlock()
	   if fn != nil {
		   for _, p := range out {
			  fn(p)
		  }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CircuitKey < out[j].CircuitKey })
	  return out
   }

// List 探针清单，最近签发在前。

  func (d *ProbeDesk) List(limit int) []Probe {
	  if d == nil {
		  return nil
	  }
	  if limit <= 0 || limit > 500 {
		   limit = 50
	   }
	  d.mu.Lock()
	  out := make([]Probe, 0, len(d.rows))
	   for _, row := range d.rows {
		  out = append(out, *row)
	   }
	   d.mu.Unlock()
	   sort.Slice(out, func(i, j int) bool { return out[i].IssuedAt.After(out[j].IssuedAt) })
	   if len(out) > limit {
		  out = out[:limit]
	   }
	  return out
   }

// Load 装回落库探针（Hydrate 用）。
  func (d *ProbeDesk) Load(p Probe) error {
	  if d == nil {
		  return ErrStore
	  }
	   if err := GuardKey(p.CircuitKey); err != nil {
		  return err
	   }
	  if strings.TrimSpace(p.ID) == "" {
		   return ErrProbeMissing
	  }
	   d.mu.Lock()
	  defer d.mu.Unlock()
	   row := p
	  d.rows[p.ID] = &row
	   if p.Pending() {
		  d.inflight[p.CircuitKey] = p.ID
	  }
	  return nil
   }

// ProbeStats 探针台读面。
type ProbeStats struct {
	Rows int `json:"rows"`
	InFlight int   `json:"in_flight"`
	Issued   int64 `json:"issued"`
	Passed   int64 `json:"passed"`
	Failed   int64 `json:"failed"`
	Expired int64 `json:"expired"`
}

// Stats 探针台统计。
  func (d *ProbeDesk) Stats() ProbeStats {
	  if d == nil {
		  return ProbeStats{}
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	  return ProbeStats{
		  Rows: len(d.rows), InFlight: len(d.inflight),
		  Issued: d.issued,
		   Passed: d.passed, Failed: d.failed, Expired: d.expired,
	  }
  }
