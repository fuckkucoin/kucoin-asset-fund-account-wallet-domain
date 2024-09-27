package fundcircuit

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// DualKind 双控申请类别。
const DualKind = "fundcircuit_clear"

// DeskEvent 运营台动作留痕。

type DeskEvent struct {
	Action string `json:"action"`
	Actor  string `json:"actor"`
	Key    string `json:"key,omitempty"`
	Scope  Scope  `json:"scope,omitempty"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
	ReqID string `json:"req_id,omitempty"`
	CaseID  string   `json:"case_id,omitempty"`
	Allowed bool     `json:"allowed"`
	Votes   []string `json:"votes,omitempty"`
	At time.Time `json:"at"`
}

// Desk 运营台。双控账本缺失时 Clear 一律拒绝（失败即关）。

type Desk struct {
	mu    sync.Mutex
	dual  *dualctl.Book
	table *Table
	cases *CaseBook
	need int
	events []DeskEvent
	clock func() time.Time
	onEvt func(DeskEvent)
}

// NewDesk 建运营台。need 小于 2 时按 2 处理，避免一人闭环。

  func NewDesk(need int) *Desk {
	  if need < 2 {
		  need = 2
	   }
	   return &Desk{
		  need: need, dual: dualctl.New(4 * time.Hour), events: make([]DeskEvent, 0, 32),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// Bind 挂熔断表与案件台。
   func (d *Desk) Bind(t *Table, cases *CaseBook) *Desk {
	  if d != nil {
		   d.mu.Lock()
		  d.table = t
		   d.cases = cases
		  d.mu.Unlock()
	   }
	   return d
   }

// BindDual 换双控账本（复用统一账本，便于与其它双控动作同源审计）。
  func (d *Desk) BindDual(b *dualctl.Book) *Desk {
	   if d != nil && b != nil {
		  d.mu.Lock()
		  d.dual = b
		  d.mu.Unlock()
	   }
	   return d
   }

// WithClock 注入时钟。
   func (d *Desk) WithClock(c func() time.Time) *Desk {
	   if d != nil && c != nil {
		  d.mu.Lock()
		   d.clock = c
		   d.mu.Unlock()
		  if d.dual != nil {
			  d.dual.WithClock(c)
		   }
	   }
	   return d
  }

  // OnEvent 审计回调，锁外执行。

  func (d *Desk) OnEvent(fn func(DeskEvent)) *Desk {
	   if d != nil {
		  d.mu.Lock()
		  d.onEvt = fn
		  d.mu.Unlock()
	   }
	  return d
  }

   func (d *Desk) now() time.Time {
	   if d == nil || d.clock == nil {
		  return time.Now().UTC()
	   }
	  return d.clock()
   }

   func (d *Desk) append(ev DeskEvent) {
	  if d == nil {
		   return
	  }
	   if ev.At.IsZero() {
		  ev.At = d.now()
	  }
	  d.mu.Lock()
	   d.events = append(d.events, ev)
	   if len(d.events) > 500 {
		  d.events = append([]DeskEvent{}, d.events[len(d.events)-500:]...)
	   }
	   fn := d.onEvt
	   d.mu.Unlock()
	   if fn != nil {
		  fn(ev)
	   }
  }

   func actorOK(actor string) (string, error) {
	  actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		   return "", ErrActor
	  }
	   return actor, nil
   }

// ForceOpen 人工强制跳闸。方向是收紧，单人即可，但须带原因并留痕。
// 功能：事故期先挡住某用户 / 资产 / 链 / 全局的新写，等值班核实后再走双控解除。
// 重点：强开的熔断器不会自动进半开，必须双人 Clear。
// 风险点：强开全局会挡住三条入口的全部新写；在途单仍不解冻，须按各域既有规则处置。

   func (d *Desk) ForceOpen(ctx context.Context, actor string, scope Scope, user, chain, asset, reason string) (Circuit, error) {
	  _ = ctx
	   if d == nil || d.table == nil {
		  return Circuit{}, ErrBus
	  }
	  who, err := actorOK(actor)
	   if err != nil {
		  return Circuit{}, err
	  }
	   reason = strings.TrimSpace(reason)
	  if reason == "" {
		   return Circuit{}, ErrReason
	  }
	   if !scope.Valid() {
		   return Circuit{}, ErrScope
	  }
	   key, err := ScopeKey(scope, user, chain, asset)
	  if err != nil {
		  return Circuit{}, err
	  }
	   pol := DefaultPolicy()
	  agg := Aggregate{
		   Key: key, Scope: scope, UserID: strings.TrimSpace(user),
		   Chain: strings.ToUpper(strings.TrimSpace(chain)), Asset: strings.ToUpper(strings.TrimSpace(asset)),
		  Score: pol.TripScore, Severity: SevTrip, At: d.now(),
	   }
	  c, _, err := d.table.Trip(agg, pol, "desk force open: "+reason, who, true)
	  if err != nil {
		  return Circuit{}, err
	   }
	  caseID := ""
	  if d.cases != nil {
		  if cs, cerr := d.cases.Open(c, agg, who, "desk force open: "+reason); cerr == nil {
			   caseID = cs.ID
			  d.table.BindCase(c.Key, cs.ID)
			  c.CaseID = cs.ID
		   }
	  }
	   d.append(DeskEvent{
		  Action: "force_open", Actor: who, Key: key, Scope: scope, Target: c.Target(),
		   Reason: reason,
		  CaseID: caseID, Allowed: true,
	  })
	  return c, nil
  }

// ForceClear 单人解熔断入口：恒拒绝。
// 功能：把误用的单人解除挡在门外，并在案件上留一条 clear_denied。
// 重点：返回 ErrDualControl；调用方须走 OpenClear→VoteClear→ExecuteClear。
// 风险点：单人解除会让刷量方立刻恢复三条入口的新写，故不设 force 旁路。

   func (d *Desk) ForceClear(ctx context.Context, actor, key, reason string) error {
	   _ = ctx
	   if d == nil {
		   return ErrBus
	  }
	   who, err := actorOK(actor)
	  if err != nil {
		   return err
	  }
	   if err := GuardKey(key); err != nil {
		   return err
	  }
	   if d.cases != nil {
		  if cs, ok := d.cases.OpenFor(key); ok {
			  _, _ = d.cases.NoteClearDenied(cs.ID, who, "single-desk clear rejected: "+strings.TrimSpace(reason))
		   }
	  }
	   d.append(DeskEvent{
		   Action: "clear_denied", Actor: who, Key: key, Reason: strings.TrimSpace(reason), Allowed: false,
	  })
	  metrics.FundCircuitClearDenied.WithLabelValues("single_desk").Inc()
	  return ErrDualControl
  }

// OpenClear 开一条解熔断双控申请。申请人自己不自动计票。
   func (d *Desk) OpenClear(ctx context.Context, actor, key, reason string) (*dualctl.Request, error) {
	  _ = ctx
	   if d == nil || d.dual == nil {
		   return nil, ErrDualControl
	  }
	  who, err := actorOK(actor)
	   if err != nil {
		  return nil, err
	  }
	   if err := GuardKey(key); err != nil {
		   return nil, err
	   }
	  reason = strings.TrimSpace(reason)
	  if reason == "" {
		   return nil, ErrReason
	   }
	  if d.table != nil {
		  c, gerr := d.table.Get(key)
		  if gerr != nil {
			   return nil, gerr
		   }
		  if !c.Blocking() {
			  return nil, ErrCircuitState
		  }
	  }
	  req, err := d.dual.Open(DualKind, who, key, d.need, reason)
	   if err != nil {
		   return nil, err
	  }
	   caseID := ""
	   if d.cases != nil {
		   if cs, ok := d.cases.OpenFor(key); ok {
			  if _, rerr := d.cases.Review(cs.ID, who, reason); rerr != nil && rerr != ErrCaseState {
				   return nil, rerr
			   }
			  if _, berr := d.cases.BindClearRequest(cs.ID, req.ID, who); berr != nil {
				   return nil, berr
			   }
			   caseID = cs.ID
		   }
	  }
	   d.append(DeskEvent{
		  Action: "clear_request", Actor: who, Key: key, Reason: reason,
		  ReqID: req.ID, CaseID: caseID, Allowed: true,
	   })
	   return req, nil
  }

// VoteClear 记一票。同一人不得投两次；deny 立刻关单。
  func (d *Desk) VoteClear(ctx context.Context, reqID, actor string, allow bool, epoch int64, token string) (*dualctl.Request, error) {
	   _ = ctx
	  if d == nil || d.dual == nil {
		   return nil, ErrDualControl
	   }
	   who, err := actorOK(actor)
	  if err != nil {
		  return nil, err
	   }
	   req, err := d.dual.Vote(strings.TrimSpace(reqID), who, allow, epoch, token)
	  if err != nil {
		   return req, err
	   }
	   d.append(DeskEvent{
		  Action: "clear_vote", Actor: who, Key: req.Subject, ReqID: req.ID,
		   Reason: string(req.State), Allowed: allow,
	  })
	  return req, nil
  }

// ExecuteClear 双人票齐后执行解熔断。
// 功能：闭合熔断器、结案并落审计；此后该作用域的新写恢复。
// 重点：票数不足、同一人两票、执行人是唯一投票人都拒绝；结案不解冻在途单。
// 风险点：执行前须确认在途筛查/广播单已按出金域规则处置，解除阻断不等于解冻。

  func (d *Desk) ExecuteClear(ctx context.Context, reqID, actor string, epoch int64, token string) (Circuit, Case, error) {
	   _ = ctx
	   if d == nil || d.dual == nil {
		  return Circuit{}, Case{}, ErrDualControl
	  }
	  who, err := actorOK(actor)
	   if err != nil {
		  return Circuit{}, Case{}, err
	   }
	  req, err := d.dual.Execute(strings.TrimSpace(reqID), who, epoch, token)
	   if err != nil {
		   if req != nil {
			  d.append(DeskEvent{
				   Action: "clear_exec_denied", Actor: who, Key: req.Subject, ReqID: req.ID,
				  Reason: err.Error(), Allowed: false,
			  })
		  }
		   metrics.FundCircuitClearDenied.WithLabelValues("quorum").Inc()
		  return Circuit{}, Case{}, err
	   }
	   votes := make([]string, 0, len(req.Votes)+1)
	   for _, v := range req.Votes {
		   if v.Allow {
			   votes = append(votes, v.Actor)
		   }
	   }
	  clean,
		   verr := distinctActors(votes)
	  if verr != nil {
		   d.append(DeskEvent{
			   Action: "clear_exec_denied", Actor: who, Key: req.Subject, ReqID: req.ID,
			  Reason: verr.Error(), Allowed: false,
		  })
		  metrics.FundCircuitClearDenied.WithLabelValues("votes").Inc()
		  return Circuit{}, Case{}, verr
	  }
	   if d.table == nil {
		   return Circuit{}, Case{}, ErrBus
	  }
	   note := "dual control clear " + req.ID + " votes " + strings.Join(clean, ",")
	   c, _, err := d.table.ForceClose(req.Subject, who, note)
	  if err != nil {
		  return Circuit{}, Case{}, err
	  }
	   var cs Case
	  if d.cases != nil {
		   if open, ok := d.cases.OpenFor(req.Subject); ok {
			  if open.State == CaseOpen {
				  if _, rerr := d.cases.Review(open.ID, clean[0], "auto review before dual clear"); rerr != nil {
					  return c, Case{}, rerr
				  }
			   }
			  cleared, cerr := d.cases.Clear(open.ID, clean, note)
			   if cerr != nil {
				   return c, cleared, cerr
			   }
			  cs = cleared
		   }
	   }
	  d.append(DeskEvent{
		   Action: "clear_executed", Actor: who, Key: req.Subject, ReqID: req.ID,
		  CaseID: cs.ID, Reason: note, Votes: clean, Allowed: true,
	   })
	   return c, cs, nil
  }

// PendingClears 未执行的解熔断申请。
   func (d *Desk) PendingClears(limit int) []dualctl.Request {
	   if d == nil || d.dual == nil {
		  return nil
	   }
	   all := d.dual.List(limit)
	  out := make([]dualctl.Request, 0, len(all))
	   for _, r := range all {
		   if r.Kind != DualKind {
			   continue
		   }
		  if r.State == dualctl.StateOpen || r.State == dualctl.StateReady {
			   out = append(out, r)
		   }
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	   return out
   }

// Audit 最近审计事件。

  func (d *Desk) Audit(limit int) []DeskEvent {
	   if d == nil {
		  return nil
	   }
	   if limit <= 0 || limit > 500 {
		   limit = 50
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  if len(d.events) <= limit {
		   return append([]DeskEvent{}, d.events...)
	   }
	  return append([]DeskEvent{}, d.events[len(d.events)-limit:]...)
  }

// Need 当前双控票数要求。
   func (d *Desk) Need() int {
	  if d == nil {
		   return 2
	   }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	  return d.need
   }

// DualBook 暴露双控账本，供运营读面查询票面。
  func (d *Desk) DualBook() *dualctl.Book {
	   if d == nil {
		   return nil
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   return d.dual
  }
