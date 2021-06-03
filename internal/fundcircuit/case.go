package fundcircuit

import (
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fsm"
	"github.com/google/uuid"
)

// 案件状态机事件。
const (
	CaseEvReview = "review"
	CaseEvClear = "clear"
)

// CaseMachine 案件边表。CaseBook 仍是权威写入口，这里供校验与回归矩阵复用。

   func CaseMachine() *fsm.Machine[CaseState, string] {
	   return fsm.New([]fsm.Edge[CaseState, string]{
		  {From: CaseOpen, Event: CaseEvReview, To: CaseReview},
		  {From: CaseReview, Event: CaseEvClear, To: CaseCleared},
	  }, CaseCleared)
   }

// CaseBook 案件台。同一熔断键同时只有一个未结案件。
type CaseBook struct {
	mu   sync.Mutex
	rows map[string]*Case
	byKey map[string]string
	mach  *fsm.Machine[CaseState, string]
	clock func() time.Time
	onSave func(Case)
	opened int64
	closed int64
	denied int64
}

// NewCaseBook 建案件台。
  func NewCaseBook() *CaseBook {
	  return &CaseBook{
		   rows: map[string]*Case{}, byKey: map[string]string{}, mach: CaseMachine(),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
   func (b *CaseBook) WithClock(c func() time.Time) *CaseBook {
	  if b != nil && c != nil {
		  b.mu.Lock()
		   b.clock = c
		  b.mu.Unlock()
	  }
	  return b
  }

   // OnSave 落库回调，锁外执行。

   func (b *CaseBook) OnSave(fn func(Case)) *CaseBook {
	   if b != nil {
		   b.mu.Lock()
		  b.onSave = fn
		   b.mu.Unlock()
	  }
	  return b
  }

  func (b *CaseBook) now() time.Time {
	  if b == nil || b.clock == nil {
		   return time.Now().UTC()
	   }
	   return b.clock()
  }

// Open 开案。同键已有未结案件时返回原案件并刷新分数，不重复开单。
// 功能：跳闸即开案，把分数、分档、跨路径证据与来源键留档给值班。
// 重点：开案不需要双人；结案才需要。
// 风险点：未开案就强制闭合熔断会丢处置链路，Desk 侧统一先开案再走双控。

  func (b *CaseBook) Open(c Circuit, agg Aggregate, openedBy, reason string) (Case, error) {
	  if b == nil {
		   return Case{}, ErrStore
	  }
	   if err := GuardKey(c.Key); err != nil {
		  return Case{}, err
	  }
	   now := b.now()
	   reason = strings.TrimSpace(reason)
	   if reason == "" {
		  reason = c.Reason
	   }
	   b.mu.Lock()
	   if id, ok := b.byKey[c.Key]; ok {
		   if row, found := b.rows[id]; found && row.State != CaseCleared {
			  row.Score = agg.Score
			   if agg.Severity.Rank() > row.Severity.Rank() {
				   row.Severity = agg.Severity
			  }
			  row.UpdatedAt = now
			  out := *row
			   fn := b.onSave
			   b.mu.Unlock()
			   if fn != nil {
				   fn(out)
			  }
			  return out, nil
		  }
	  }
	   row := &Case{
		  ID: uuid.NewString(), CircuitKey: c.Key, Scope: c.Scope, UserID: c.UserID,
		   Chain: c.Chain,
		  Asset: c.Asset, State: CaseOpen, Score: agg.Score,
		  Severity: agg.Severity, Reason: reason, OpenedBy: strings.TrimSpace(openedBy),
		  CreatedAt: now, UpdatedAt: now,
	  }
	   row.Audit = append(row.Audit, CaseAudit{
		   Action: "open", Actor: row.OpenedBy, From: "", To: CaseOpen, Note: reason, At: now,
	   })
	   b.rows[row.ID] = row
	  b.byKey[c.Key] = row.ID
	   b.opened++
	  out := *row
	  fn := b.onSave
	   b.mu.Unlock()
	   if fn != nil {
		  fn(out)
	  }
	   return out, nil
   }

// Review 转入复核。reviewer 必填且不得是 system。

   func (b *CaseBook) Review(id, reviewer, note string) (Case, error) {
	   if b == nil {
		   return Case{}, ErrStore
	  }
	  reviewer = strings.TrimSpace(reviewer)
	   if reviewer == "" || strings.EqualFold(reviewer, "system") {
		  return Case{}, ErrActor
	   }
	   now := b.now()
	   b.mu.Lock()
	  row, ok := b.rows[strings.TrimSpace(id)]
	  if !ok {
		  b.mu.Unlock()
		  return Case{}, ErrCaseMissing
	   }
	  next, err := b.mach.Apply(row.State, CaseEvReview)
	   if err != nil {
		   out := *row
		   b.mu.Unlock()
		  return out, ErrCaseState
	   }
	  from := row.State
	   row.State = next
	  row.Reviewer = reviewer
	  row.UpdatedAt = now
	   row.Audit = append(row.Audit, CaseAudit{
		  Action: CaseEvReview, Actor: reviewer, From: from, To: next,
		  Note: strings.TrimSpace(note), At: now,
	   })
	   out := *row
	  fn := b.onSave
	   b.mu.Unlock()
	  if fn != nil {
		  fn(out)
	  }
	   return out, nil
   }

// NoteClearDenied 记一次被拒的结案尝试（通常是单人 Clear）。

  func (b *CaseBook) NoteClearDenied(id, actor, note string) (Case, error) {
	  if b == nil {
		   return Case{}, ErrStore
	   }
	  now := b.now()
	  b.mu.Lock()
	   row, ok := b.rows[strings.TrimSpace(id)]
	  if !ok {
		   b.mu.Unlock()
		   return Case{}, ErrCaseMissing
	  }
	   row.Audit = append(row.Audit, CaseAudit{
		  Action: "clear_denied", Actor: strings.TrimSpace(actor), From: row.State, To: row.State,
		   Note: strings.TrimSpace(note), At: now,
	  })
	  row.UpdatedAt = now
	   b.denied++
	   out := *row
	   fn := b.onSave
	   b.mu.Unlock()
	   if fn != nil {
		   fn(out)
	   }
	   return out, nil
  }

// BindClearRequest 绑定双控申请号，供结案时核对票数来源。
  func (b *CaseBook) BindClearRequest(id, reqID, actor string) (Case, error) {
	  if b == nil {
		   return Case{}, ErrStore
	  }
	  reqID = strings.TrimSpace(reqID)
	  if reqID == "" {
		  return Case{}, ErrDualControl
	   }
	  now := b.now()
	   b.mu.Lock()
	   row, ok := b.rows[strings.TrimSpace(id)]
	   if !ok {
		   b.mu.Unlock()
		  return Case{}, ErrCaseMissing
	   }
	   row.ClearReq = reqID
	  row.UpdatedAt = now
	  row.Audit = append(row.Audit, CaseAudit{
		   Action: "clear_request", Actor: strings.TrimSpace(actor), From: row.State, To: row.State,
		  Note: "dual control request " + reqID, At: now,
	  })
	  out := *row
	   fn := b.onSave
	   b.mu.Unlock()
	  if fn != nil {
		  fn(out)
	  }
	   return out, nil
   }

// Clear 结案。votes 必须是两个以上互异的操作人，否则拒绝并留痕。
// 功能：双人复核通过后关案，与熔断器闭合成对出现。
// 重点：状态须先进 review；票数不足或同一人两票一律 ErrDualControl。
// 风险点：结案只解除新写阻断，不解冻在途单；在途单仍按出金域既有规则处置。

   func (b *CaseBook) Clear(id string, votes []string, note string) (Case, error) {
	  if b == nil {
		  return Case{}, ErrStore
	   }
	  clean,
		   err := distinctActors(votes)
	   if err != nil {
		  return Case{}, err
	   }
	  now := b.now()
	   b.mu.Lock()
	   row,
		   ok := b.rows[strings.TrimSpace(id)]
	   if !ok {
		  b.mu.Unlock()
		   return Case{}, ErrCaseMissing
	   }
	  next,
		  aerr := b.mach.Apply(row.State, CaseEvClear)
	   if aerr != nil {
		  out := *row
		   b.mu.Unlock()
		  return out, ErrCaseState
	   }
	   from := row.State
	  row.State = next
	  row.ClearVotes = clean
	  row.UpdatedAt = now
	  row.Audit = append(row.Audit, CaseAudit{
		   Action: CaseEvClear, Actor: strings.Join(clean, ","), From: from, To: next,
		  Note: strings.TrimSpace(note), At: now,
	   })
	  delete(b.byKey, row.CircuitKey)
	  b.closed++
	   out := *row
	  fn := b.onSave
	  b.mu.Unlock()
	   if fn != nil {
		   fn(out)
	  }
	  return out, nil
   }

// distinctActors 校验票面：至少两名互异且非 system 的操作人。
   func distinctActors(votes []string) ([]string, error) {
	  seen := map[string]struct{}{}
	   out := make([]string, 0, len(votes))
	  for _, v := range votes {
		   a := strings.TrimSpace(v)
		  if a == "" || strings.EqualFold(a, "system") {
			   continue
		   }
		  low := strings.ToLower(a)
		  if _, ok := seen[low]; ok {
			   continue
		  }
		  seen[low] = struct{}{}
		  out = append(out, a)
	  }
	   if len(out) < 2 {
		   return nil, ErrDualControl
	  }
	   sort.Strings(out)
	   return out, nil
  }

// Get 读一个案件。
   func (b *CaseBook) Get(id string) (Case, error) {
	  if b == nil {
		   return Case{}, ErrStore
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   row, ok := b.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Case{}, ErrCaseMissing
	  }
	   return *row, nil
   }

// OpenFor 取某熔断键的未结案件。
  func (b *CaseBook) OpenFor(key string) (Case, bool) {
	   if b == nil {
		  return Case{}, false
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  id, ok := b.byKey[key]
	  if !ok {
		  return Case{}, false
	   }
	  row, found := b.rows[id]
	  if !found || row.State == CaseCleared {
		   return Case{}, false
	  }
	   return *row, true
  }

// List 案件清单，可按状态过滤。

   func (b *CaseBook) List(state CaseState, limit int) []Case {
	   if b == nil {
		  return nil
	   }
	   if limit <= 0 || limit > 500 {
		  limit = 50
	   }
	   b.mu.Lock()
	  out := make([]Case, 0, len(b.rows))
	  for _, row := range b.rows {
		  if state != "" && row.State != state {
			   continue
		  }
		  out = append(out, *row)
	  }
	   b.mu.Unlock()
	   sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	  if len(out) > limit {
		  out = out[:limit]
	  }
	   return out
   }

// OpenCount 未结案件数。

   func (b *CaseBook) OpenCount() int {
	  if b == nil {
		  return 0
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   return len(b.byKey)
   }

// Load 装回落库案件（Hydrate 用）。
   func (b *CaseBook) Load(c Case) error {
	  if b == nil {
		  return ErrStore
	  }
	   if err := GuardKey(c.CircuitKey); err != nil {
		   return err
	   }
	  if strings.TrimSpace(c.ID) == "" {
		   return ErrCaseMissing
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   row := c
	   b.rows[c.ID] = &row
	  if c.State != CaseCleared {
		  b.byKey[c.CircuitKey] = c.ID
	  }
	  return nil
  }

// CaseStats 案件台读面。
type CaseStats struct {
	Rows int `json:"rows"`
	Open int `json:"open"`
	Review  int   `json:"review"`
	Cleared int   `json:"cleared"`
	Opened  int64 `json:"opened_total"`
	Closed  int64 `json:"cleared_total"`
	ClearDenied int64 `json:"clear_denied_total"`
}

// Stats 案件统计。
  func (b *CaseBook) Stats() CaseStats {
	  if b == nil {
		  return CaseStats{}
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  out := CaseStats{Rows: len(b.rows),
		  Opened: b.opened, Closed: b.closed, ClearDenied: b.denied}
	   for _, row := range b.rows {
		   switch row.State {
		  case CaseOpen:
			  out.Open++
		   case CaseReview:
			   out.Review++
		  case CaseCleared:
			   out.Cleared++
		  }
	  }
	   return out
   }
