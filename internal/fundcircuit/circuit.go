package fundcircuit

import (
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fsm"
	"account-assets.kucoin.net/funding/internal/persist"
)

// 熔断状态机事件。

const (
	EvTrip      = "trip"
	EvCool      = "cool"
	EvProbePass = "probe_pass"
	EvProbeFail = "probe_fail"
	EvForceOpen = "force_open"
	EvForceClose = "force_close"
)

// Machine 熔断边表。服务侧的 Table 仍是权威写入口，这里供校验与回归矩阵复用。
   func Machine() *fsm.Machine[CircuitState, string] {
	   return fsm.New([]fsm.Edge[CircuitState, string]{
		  {From: StateClosed, Event: EvTrip, To: StateOpen},
		   {From: StateClosed, Event: EvForceOpen, To: StateOpen},
		  {From: StateOpen, Event: EvTrip, To: StateOpen},
		   {From: StateOpen, Event: EvCool, To: StateHalfOpen},
		   {From: StateOpen,
			  Event: EvForceOpen, To: StateOpen},
		   {From: StateOpen, Event: EvForceClose, To: StateClosed},
		   {From: StateHalfOpen, Event: EvProbePass, To: StateClosed},
		  {From: StateHalfOpen, Event: EvProbeFail, To: StateOpen},
		   {From: StateHalfOpen, Event: EvTrip, To: StateOpen},
		  {From: StateHalfOpen, Event: EvForceOpen, To: StateOpen},
		  {From: StateHalfOpen, Event: EvForceClose, To: StateClosed},
	   })
  }

// Transition 一次状态跃迁留痕，供 worker 回报与审计。

type Transition struct {
	Key   string `json:"key"`
	Event string `json:"event"`
	From CircuitState `json:"from"`
	To CircuitState `json:"to"`
	Reason string `json:"reason,omitempty"`
	Actor  string `json:"actor,omitempty"`
	Score  int    `json:"score"`
	At time.Time `json:"at"`
}

// Table 熔断器表。写入串行化在表锁内，持久化回调在锁外执行。

type Table struct {
	mu   sync.Mutex
	rows map[string]*Circuit
	fence  *persist.Table
	clock  func() time.Time
	mach   *fsm.Machine[CircuitState, string]
	onSave func(Circuit)
	trips  int64
	closes int64
	probes int64
}

// NewTable 建熔断器表。
   func NewTable() *Table {
	  return &Table{
		   rows: map[string]*Circuit{}, fence: persist.NewTable(), mach: Machine(),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

   func (t *Table) WithClock(c func() time.Time) *Table {
	  if t != nil && c != nil {
		   t.mu.Lock()
		   t.clock = c
		  t.mu.Unlock()
	   }
	   return t
   }

   // OnSave 落库回调。回调在锁外执行，失败由调用方记指标，不回滚内存态。
  func (t *Table) OnSave(fn func(Circuit)) *Table {
	  if t != nil {
		  t.mu.Lock()
		   t.onSave = fn
		   t.mu.Unlock()
	   }
	   return t
   }

  func (t *Table) now() time.Time {
	   if t == nil || t.clock == nil {
		  return time.Now().UTC()
	   }
	  return t.clock()
  }

// blank 生成一个 closed 空壳，使未跳闸的键也能参与决策。
  func blank(key string, now time.Time) Circuit {
	  scope, user, chain, asset, err := ParseKey(key)
	   if err != nil {
		  scope = ScopeGlobal
	  }
	   return Circuit{
		  Key: key, Scope: scope, UserID: user, Chain: chain, Asset: asset,
		   State: StateClosed, Severity: SevNone, UpdatedAt: now.UTC(),
	  }
   }

// Snapshot 读一个键的熔断器。不存在时返回 closed 空壳，便于闸门直接求值。

   func (t *Table) Snapshot(key string) Circuit {
	  now := t.now()
	  if t == nil {
		  return blank(key, now)
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	   row, ok := t.rows[key]
	   if !ok {
		   return blank(key, now)
	  }
	  return *row
  }

// Get 读一个键；不存在返回 ErrCircuitMissing。

   func (t *Table) Get(key string) (Circuit, error) {
	  if t == nil {
		  return Circuit{}, ErrStore
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	   row, ok := t.rows[key]
	   if !ok {
		  return Circuit{}, ErrCircuitMissing
	  }
	  return *row, nil
  }

// upsert 在锁内取出或建行。调用方负责在锁内改字段。
   func (t *Table) upsert(key string, now time.Time) *Circuit {
	   row, ok := t.rows[key]
	   if ok {
		  return row
	   }
	  c := blank(key, now)
	  t.rows[key] = &c
	  return &c
  }

   func (t *Table) bump(c *Circuit) {
	  if t == nil || c == nil || t.fence == nil {
		   return
	   }
	  f, err := t.fence.Bump(c.Key, persist.NewToken())
	   if err != nil {
		  return
	   }
	   c.Fence = f
   }

// Note 记录聚合结果但不跳闸。watch/warn 档只更新读面分数与案件依据。
// 零分且尚无熔断行时不建行，避免每轮聚合把全部作用域键写成空行。
  func (t *Table) Note(agg Aggregate, pol BlockPolicy) Circuit {
	   if t == nil {
		   return Circuit{}
	   }
	   now := t.now()
	  t.mu.Lock()
	   if _, exists := t.rows[agg.Key]; !exists && agg.Score == 0 {
		   t.mu.Unlock()
		  return blank(agg.Key, now)
	   }
	   row := t.upsert(agg.Key, now)
	  row.Score = agg.Score
	   row.Severity = agg.Severity
	  row.CrossPath = agg.CrossPath
	  row.PolicyVer = pol.Version
	   row.UpdatedAt = now
	  t.bump(row)
	   out := *row
	   fn := t.onSave
	  t.mu.Unlock()
	  if fn != nil {
		   fn(out)
	   }
	   return out
  }

// Trip 跳闸。closed→open；已 open 只刷新分数与冷却，不重复计次。
// 功能：跨入口聚合达到跳闸档后挡住该作用域的新写。
// 重点：跳闸不触碰在途冻结，也不调用任何解冻路径。
// 风险点：actor 为空时按 system 记，读面无法追人，运营强开须带 actor。
   func (t *Table) Trip(agg Aggregate, pol BlockPolicy, reason, actor string, forced bool) (Circuit, Transition, error) {
	  if t == nil {
		   return Circuit{}, Transition{}, ErrStore
	   }
	   if err := GuardKey(agg.Key); err != nil {
		   return Circuit{},
			  Transition{}, err
	   }
	   now := t.now()
	   reason = strings.TrimSpace(reason)
	   if reason == "" {
		   reason = "fund abuse score " + severityLabel(agg.Severity)
	   }
	   t.mu.Lock()
	  row := t.upsert(agg.Key, now)
	  from := row.State
	  next, err := t.mach.Apply(from, tripEvent(forced))
	  if err != nil {
		   t.mu.Unlock()
		  return Circuit{}, Transition{}, ErrCircuitState
	  }
	  row.State = next
	  row.Score = agg.Score
	   if agg.Severity.Rank() > row.Severity.Rank() || from != StateOpen {
		   row.Severity = agg.Severity
	   }
	   row.CrossPath = agg.CrossPath
	   row.Reason = reason
	   row.PolicyVer = pol.Version
	  row.Forced = forced
	  if actor != "" {
		   row.Actor = strings.TrimSpace(actor)
	   }
	  if from != StateOpen {
		   row.Trips++
		   t.trips++
		  row.OpenedAt = now
	  }
	  row.ProbesIssued = 0
	   row.HalfOpenAt = time.Time{}
	   row.ClosedAt = time.Time{}
	   row.NextAdvanceAt = now.Add(pol.CooldownFor(row.Trips))
	   row.UpdatedAt = now
	   t.bump(row)
	   out := *row
	  fn := t.onSave
	  t.mu.Unlock()
	  if fn != nil {
		  fn(out)
	   }
	  tr := Transition{
		  Key: out.Key, Event: tripEvent(forced), From: from, To: out.State,
		   Reason: reason, Actor: out.Actor, Score: out.Score, At: now,
	  }
	   return out, tr, nil
  }

   func tripEvent(forced bool) string {
	   if forced {
		  return EvForceOpen
	   }
	  return EvTrip
   }

  func severityLabel(s Severity) string {
	   if s == "" {
		  return SevNone.String()
	   }
	  return s.String()
  }

// Cool 把冷却到期的 open 推进到 half_open。返回本轮跃迁清单。
// 功能：冷却期满后允许一枚探针试写，探针结论决定闭合还是重开。
// 重点：强制开启（forced）的熔断器不自动进半开，必须双人 Clear。
// 风险点：时钟回拨会让冷却判定提前，值班以 next_advance_at 读面为准。
   func (t *Table) Cool(pol BlockPolicy) []Transition {
	  if t == nil {
		   return nil
	  }
	   now := t.now()
	  t.mu.Lock()
	   out := make([]Transition, 0, 4)
	   saved := make([]Circuit, 0, 4)
	  for _, row := range t.rows {
		  if row.State != StateOpen || row.Forced {
			  continue
		   }
		  if row.NextAdvanceAt.IsZero() || now.Before(row.NextAdvanceAt) {
			  continue
		  }
		   next, err := t.mach.Apply(row.State, EvCool)
		   if err != nil {
			  continue
		   }
		  from := row.State
		  row.State = next
		  row.HalfOpenAt = now
		  row.ProbesIssued = 0
		  row.NextAdvanceAt = now.Add(pol.withDefaults().ProbeTTL)
		  row.UpdatedAt = now
		   t.bump(row)
		  out = append(out, Transition{
			   Key: row.Key, Event: EvCool, From: from, To: next,
			   Reason: "cooldown elapsed", Score: row.Score, At: now,
		   })
		   saved = append(saved, *row)
	  }
	  fn := t.onSave
	  t.mu.Unlock()
	   if fn != nil {
		   for _, c := range saved {
			  fn(c)
		   }
	  }
	  sort.Slice(out,
		  func(i, j int) bool { return out[i].Key < out[j].Key })
	  return out
  }

// MarkProbe 记一次探针签发。半开态外调用返回 ErrHalfOpenOnly。
  func (t *Table) MarkProbe(key string, pol BlockPolicy) (Circuit, error) {
	   if t == nil {
		   return Circuit{}, ErrStore
	   }
	  now := t.now()
	  t.mu.Lock()
	   row, ok := t.rows[key]
	   if !ok {
		   t.mu.Unlock()
		   return Circuit{}, ErrCircuitMissing
	   }
	   if row.State != StateHalfOpen {
		   t.mu.Unlock()
		  return *row, ErrHalfOpenOnly
	   }
	   budget := pol.withDefaults().HalfOpenProbes
	   if row.ProbesIssued >= budget {
		   t.mu.Unlock()
		   return *row, ErrProbeBudget
	  }
	  row.ProbesIssued++
	   row.UpdatedAt = now
	   t.probes++
	   t.bump(row)
	   out := *row
	  fn := t.onSave
	  t.mu.Unlock()
	   if fn != nil {
		   fn(out)
	  }
	   return out, nil
   }

// ProbePassed 探针成功：half_open→closed，分数与原因清零，但保留跳闸次数用于退避。
  func (t *Table) ProbePassed(key, actor, note string) (Circuit, Transition, error) {
	  return t.settle(key, EvProbePass, actor, note, BlockPolicy{})
  }

// ProbeFailed 探针失败：half_open→open，并按跳闸次数加长冷却。
  func (t *Table) ProbeFailed(key, actor, note string, pol BlockPolicy) (Circuit, Transition, error) {
	  return t.settle(key, EvProbeFail, actor, note, pol)
  }

// ForceClose 人工闭合（双人 Clear 执行后调用）。单人路径不得直达此处。
   func (t *Table) ForceClose(key, actor, note string) (Circuit, Transition, error) {
	  return t.settle(key, EvForceClose, actor, note, BlockPolicy{})
   }

   func (t *Table) settle(key, event, actor, note string, pol BlockPolicy) (Circuit, Transition, error) {
	  if t == nil {
		   return Circuit{}, Transition{}, ErrStore
	   }
	   now := t.now()
	   t.mu.Lock()
	  row, ok := t.rows[key]
	  if !ok {
		   t.mu.Unlock()
		   return Circuit{}, Transition{}, ErrCircuitMissing
	  }
	   from := row.State
	  next, err := t.mach.Apply(from, event)
	  if err != nil {
		   t.mu.Unlock()
		  return *row, Transition{}, ErrCircuitState
	   }
	   row.State = next
	  row.UpdatedAt = now
	   row.Actor = strings.TrimSpace(actor)
	   switch next {
	  case StateClosed:
		   row.ClosedAt = now
		  row.NextAdvanceAt = time.Time{}
		  row.HalfOpenAt = time.Time{}
		  row.ProbesIssued = 0
		  row.Forced = false
		   row.Severity = SevNone
		  row.Score = 0
		   row.Reason = strings.TrimSpace(note)
		  t.closes++
	  case StateOpen:
		   row.OpenedAt = now
		  row.HalfOpenAt = time.Time{}
		  row.ProbesIssued = 0
		   row.Trips++
		  t.trips++
		   if strings.TrimSpace(note) != "" {
			  row.Reason = strings.TrimSpace(note)
		   }
		  row.NextAdvanceAt = now.Add(pol.CooldownFor(row.Trips))
	  }
	  t.bump(row)
	   out := *row
	  fn := t.onSave
	   t.mu.Unlock()
	  if fn != nil {
		  fn(out)
	   }
	   tr := Transition{
		  Key: out.Key, Event: event, From: from, To: out.State,
		  Reason: strings.TrimSpace(note), Actor: out.Actor, Score: out.Score, At: now,
	   }
	   return out, tr, nil
  }

// BindCase 把案件号挂到熔断器上，便于读面串起案件与阻断。
  func (t *Table) BindCase(key, caseID string) {
	  if t == nil || strings.TrimSpace(caseID) == "" {
		  return
	   }
	  t.mu.Lock()
	   row, ok := t.rows[key]
	   if ok {
		   row.CaseID = strings.TrimSpace(caseID)
		   row.UpdatedAt = t.now()
	  }
	  var out Circuit
	   if ok {
		   out = *row
	  }
	   fn := t.onSave
	  t.mu.Unlock()
	  if ok && fn != nil {
		  fn(out)
	  }
  }

// Load 装回已落库熔断器（Hydrate 用）。键位非 fc: 一律拒绝。

  func (t *Table) Load(c Circuit) error {
	  if t == nil {
		  return ErrStore
	  }
	  if err := GuardKey(c.Key); err != nil {
		   return err
	   }
	   if !c.State.Valid() {
		   return ErrCircuitState
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	   row := c
	  t.rows[c.Key] = &row
	  return nil
  }

// List 全部熔断器，按键排序。
   func (t *Table) List(limit int) []Circuit {
	   if t == nil {
		  return nil
	   }
	   if limit <= 0 || limit > 1000 {
		   limit = 200
	  }
	   t.mu.Lock()
	  out := make([]Circuit, 0, len(t.rows))
	   for _, row := range t.rows {
		  out = append(out, *row)
	   }
	   t.mu.Unlock()
	   sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	   if len(out) > limit {
		   out = out[:limit]
	   }
	   return out
  }

// Active 仅返回拦截态（open / half_open）熔断器。
  func (t *Table) Active() []Circuit {
	   all := t.List(1000)
	  out := make([]Circuit, 0, len(all))
	  for _, c := range all {
		  if c.Blocking() {
			  out = append(out, c)
		   }
	  }
	   return out
   }

// TableStats 熔断表读面。

type TableStats struct {
	Rows int `json:"rows"`
	Open int `json:"open"`
	HalfOpen int `json:"half_open"`
	Closed int `json:"closed"`
	Trips  int64 `json:"trips"`
	Closes int64 `json:"closes"`
	Probes int64 `json:"probes_issued"`
}

// Stats 熔断表统计。
  func (t *Table) Stats() TableStats {
	   if t == nil {
		  return TableStats{}
	  }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	   out := TableStats{Rows: len(t.rows), Trips: t.trips, Closes: t.closes, Probes: t.probes}
	   for _, row := range t.rows {
		   switch row.State {
		  case StateOpen:
			   out.Open++
		   case StateHalfOpen:
			   out.HalfOpen++
		   default:
			  out.Closed++
		  }
	  }
	   return out
   }
