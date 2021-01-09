// 功能：窗口内三类差额落入金工单并锁住该输出：金额不一致、已确认却未贷记、同一输出再次贷记。
// 窗口内三类差额落入金工单并锁住该输出：金额不一致、已确认却未贷记、同一输出再次贷记。
// 窗口外仍锁输出，避免过了窗口还能补一笔，但不再开新单。
// 同一输出、同一差额再次巡检时回放已有工单。

// 作者：Gunner
// 日期：2021.01.08

package deposit

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// PatrolIn 一笔观测。三件套用同一入口。
type PatrolIn struct {
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Asset string `json:"asset"`
	OutputIndex int    `json:"output_index"`
	Observed    string `json:"observed_amount"`
	Credited    string `json:"credited_amount"`
	Confirmed   bool   `json:"confirmed"`
	Confirms    int    `json:"confirmations"`
	Need int `json:"need"`
	CreditState string `json:"credit_state"`
	TransferID    string `json:"transfer_id"`
	SecondAttempt bool   `json:"second_attempt"`
	SecondAmount   string    `json:"second_amount"`
	SecondTransfer string    `json:"second_transfer"`
	At             time.Time `json:"at"`
}

// PatrolOut 巡检结论。Ticket 非空表示窗口内已落单。
type PatrolOut struct {
	Kind string `json:"kind,omitempty"`
	TicketID string `json:"ticket_id,omitempty"`
	Blocked  bool             `json:"blocked"`
	InWindow bool             `json:"in_window"`
	Event    DepositDiffEvent `json:"event"`
}

// sight 巡检内存行。
type sight struct {
	PatrolIn
	Kind string
	TicketID string
	Blocked  bool
}

// Patrol 簿。窗口默认 24 小时。

type Patrol struct {
	mu sync.Mutex
	rows map[string]*sight
	window time.Duration
	clock func() time.Time
	desk  *Desk
	slots *Slots
	actor string
}

   func NewPatrol() *Patrol {
	  return &Patrol{
		  rows:   map[string]*sight{},
		   window: 24 * time.Hour,
		   clock:  func() time.Time { return time.Now().UTC() },
		   actor: "deposit-patrol",
	   }
  }

   func (p *Patrol) BindDesk(d *Desk) {
	  if p != nil {
		  p.desk = d
	   }
  }

  func (p *Patrol) BindSlots(s *Slots) {
	  if p != nil {
		   p.slots = s
	   }
   }

   func (p *Patrol) SetWindow(w time.Duration) {
	   if p == nil || w <= 0 {
		  return
	  }
	   p.mu.Lock()
	  p.window = w
	  p.mu.Unlock()
   }

  func (p *Patrol) now() time.Time {
	   if p == nil || p.clock == nil {
		  return time.Now().UTC()
	   }
	   return p.clock()
   }

  func (p *Patrol) inWindow(at time.Time) bool {
	   if at.IsZero() {
		  return true
	  }
	   w := p.window
	  if w <= 0 {
		  w = 24 * time.Hour
	   }
	   now := p.now()
	  if at.After(now.Add(w)) {
		   return false
	   }
	  return now.Sub(at) <= w
  }

// Apply 分类三件套并在窗口内强制工单。任何差额都会锁输出，二次贷记失败。

   func (p *Patrol) Apply(in PatrolIn) (PatrolOut, error) {
	   if p == nil {
		   return PatrolOut{}, ErrPatrolInput
	   }
	   norm, err := normalizePatrol(in)
	   if err != nil {
		  return PatrolOut{}, err
	  }
	   kind := classify(norm)
	  inside := p.inWindow(norm.At)
	  if prev, ok := p.prior(norm.Chain, norm.TxHash, norm.OutputIndex); ok && prev.Kind == kind && kind != "" && prev.TicketID != "" {
		   out := PatrolOut{Kind: kind, TicketID: prev.TicketID, Blocked: true, InWindow: inside}
		   out.Event = p.event(norm, kind, prev.TicketID, true, inside)
		   return out, nil
	  }
	   out := PatrolOut{Kind: kind, InWindow: inside}
	   if kind == "" {
		  if p.slots != nil && positiveAmount(norm.Credited) && !norm.SecondAttempt {
			  if err := p.slots.Commit(OutputSlot{
				   Chain: norm.Chain, TxHash: norm.TxHash, OutputIndex: norm.OutputIndex,
				  UserID: norm.UserID, Asset: norm.Asset, Amount: norm.Credited,
				   TransferID: norm.TransferID, State: SlotCredited,
			   }); err != nil {
				   if lerr := p.slots.Lock(norm.Chain, norm.TxHash, KindDupOutput, norm.OutputIndex); lerr != nil {
					  return PatrolOut{}, lerr
				  }
				  p.remember(norm, KindDupOutput, "", true)
				   dup := PatrolOut{Kind: KindDupOutput, Blocked: true, InWindow: inside}
				   dup.Event = p.event(norm, KindDupOutput, "", true, inside)
				   return dup, err
			  }
		  }
		  p.remember(norm, "", "", false)
		  out.Event = p.event(norm, "", "", false, inside)
		  return out, nil
	   }
	   if p.slots != nil {
		  if err := p.slots.Lock(norm.Chain, norm.TxHash, kind, norm.OutputIndex); err != nil {
			  return PatrolOut{}, err
		   }
	  }
	   if !inside {
		  p.remember(norm, kind, "", true)
		   out.Blocked = true
		   out.Event = p.event(norm, kind, "", true, false)
		   return out, ErrOutsideWindow
	   }
	   if p.desk == nil {
		   out.Blocked = true
		  out.Event = p.event(norm, kind, "", true, true)
		  return out, ErrTicketMiss
	  }
	  if strings.TrimSpace(norm.UserID) == "" {
		   out.Blocked = true
		  out.Event = p.event(norm, kind, "", true, true)
		   return out, ErrUnattributed
	  }
	  ex, err := p.desk.OpenException(p.actor, norm.UserID, norm.Chain, norm.Asset, norm.TxHash, ticketKindOf(kind), noteOf(kind, norm))
	   if err != nil {
		   out.Blocked = true
		   out.Event = p.event(norm, kind, "", true, true)
		   return out, err
	   }
	   ticketID := ex.ID
	  p.remember(norm, kind, ticketID, true)
	  out.TicketID = ticketID
	  out.Blocked = true
	  out.Event = p.event(norm, kind, ticketID, true, true)
	  return out, nil
   }

// TryCredit 窗口内已落单或已贷记的输出不得再成功贷记。
   func (p *Patrol) TryCredit(chain, hash, transferID, amount string, output int) error {
	   if p == nil {
		  return ErrSecondCredit
	   }
	  if p.slots != nil {
		   if err := p.slots.Allow(chain, hash, "", output); err != nil {
			  return err
		   }
	   }
	  k := slotKey(chain, hash, output)
	   p.mu.Lock()
	   cur, ok := p.rows[k]
	   p.mu.Unlock()
	  if !ok {
		  return nil
	   }
	  if cur.Blocked || cur.Kind != "" {
		   return ErrSecondCredit
	  }
	   if cur.TransferID != "" && transferID != "" && cur.TransferID != transferID && cur.Credited != "" {
		  return ErrSecondCredit
	   }
	   if cur.Credited != "" && amount != "" && !sameAmt(cur.Credited, amount) && transferID != cur.TransferID {
		   return ErrSecondCredit
	   }
	  return nil
   }

// NoteBalanced 观察线程成功贷记后记下守恒行，避免巡检把成功单再开成差额。
  func (p *Patrol) NoteBalanced(in PatrolIn) {
	  if p == nil {
		  return
	   }
	  norm, err := normalizePatrol(in)
	   if err != nil {
		   return
	   }
	  p.remember(norm, "", "", false)
  }

   func (p *Patrol) prior(chain, hash string, output int) (sight, bool) {
	   if p == nil {
		  return sight{}, false
	   }
	  p.mu.Lock()
	  defer p.mu.Unlock()
	  cur, ok := p.rows[slotKey(chain, hash, output)]
	   if !ok || cur == nil {
		   return sight{}, false
	   }
	   return *cur, true
  }

   func (p *Patrol) remember(in PatrolIn, kind, ticket string, blocked bool) {
	   k := slotKey(in.Chain, in.TxHash, in.OutputIndex)
	   p.mu.Lock()
	   defer p.mu.Unlock()
	  p.rows[k] = &sight{PatrolIn: in, Kind: kind, TicketID: ticket, Blocked: blocked}
  }

   func (p *Patrol) event(in PatrolIn, kind, ticket string, blocked, inside bool) DepositDiffEvent {
	   start := p.now().Add(-p.window)
	   end := p.now()
	  ev := DepositDiffEvent{
		   Event: EventDepositDiff, Kind: kind, Chain: in.Chain, TxHash: in.TxHash,
		  OutputIndex: in.OutputIndex, Asset: in.Asset, UserID: in.UserID,
		   ObservedAmount: in.Observed, CreditedAmount: in.Credited,
		  ObserveState: observeState(in), CreditState: creditState(in),
		   WindowStart: start.Format(time.RFC3339), WindowEnd: end.Format(time.RFC3339),
		   SecondCredit: kind == KindDupOutput || in.SecondAttempt,
		   Blocked: blocked, DepositTicket: ticket,
		   Phase: phaseOf(in, kind), Reason: noteOf(kind, in),
	  }
	  if !inside {
		  ev.Reason = "outside window"
	   }
	  return NormalizeEvent(ev)
   }

// Get 读最近一次巡检行。
  func (p *Patrol) Get(chain, hash string, output int) (PatrolOut, bool) {
	   if p == nil {
		   return PatrolOut{}, false
	  }
	  p.mu.Lock()
	  defer p.mu.Unlock()
	   cur, ok := p.rows[slotKey(chain, hash, output)]
	  if !ok {
		  return PatrolOut{}, false
	   }
	  return PatrolOut{Kind: cur.Kind,
		  TicketID: cur.TicketID, Blocked: cur.Blocked, InWindow: p.inWindow(cur.At)}, true
   }

  func normalizePatrol(in PatrolIn) (PatrolIn, error) {
	  var err error
	   in.Chain, err = validate.Chain(in.Chain)
	  if err != nil {
		  return PatrolIn{},
			   ErrPatrolInput
	  }
	  in.Chain = CanonicalChain(in.Chain)
	   in.TxHash, err = validate.TxHash(in.TxHash)
	   if err != nil {
		   return PatrolIn{}, ErrPatrolInput
	  }
	  if strings.TrimSpace(in.Asset) != "" {
		   in.Asset, err = validate.Asset(in.Asset)
		  if err != nil {
			   return PatrolIn{}, ErrPatrolInput
		  }
	  }
	  if strings.TrimSpace(in.UserID) != "" {
		  in.UserID, err = validate.UserID(in.UserID)
		  if err != nil {
			   return PatrolIn{}, ErrPatrolInput
		   }
	   }
	  if in.OutputIndex < 0 {
		  return PatrolIn{}, ErrPatrolInput
	  }
	   if in.Observed != "" {
		   if _, err := money.ParseBook(in.Observed); err != nil {
			   return PatrolIn{}, ErrPatrolInput
		   }
	   }
	   if in.Credited != "" {
		   if _, err := money.ParseBook(in.Credited); err != nil {
			   return PatrolIn{}, ErrPatrolInput
		   }
	  }
	   if in.At.IsZero() {
		  in.At = time.Now().UTC()
	   }
	  if in.Need <= 0 {
		  in.Need = DepthFor(in.Chain)
	  }
	   return in, nil
   }

// classify 先看是不是第二笔，再看金额是否对得上，最后看确认了却没有贷记。
// 已有贷记金额时不再报「未贷记」，避免成功单被巡检再开一张缺额单。
   func positiveAmount(raw string) bool {
	  raw = strings.TrimSpace(raw)
	   if raw == "" {
		  return false
	  }
	   amt, err := money.ParseBook(raw)
	   if err != nil {
		   return false
	   }
	   return !amt.IsZero()
  }

   func classify(in PatrolIn) string {
	  if in.SecondAttempt {
		  return KindDupOutput
	   }
	   hasCredit := positiveAmount(in.Credited)
	  if hasCredit && in.Observed != "" && !sameAmt(in.Observed, in.Credited) {
		  return KindObserveNE
	  }
	   if in.Confirmed && !hasCredit {
		  return KindConfirmedMiss
	  }
	  return ""
   }

  func sameAmt(a, b string) bool {
	   aa, err1 := money.ParseBook(a)
	   bb, err2 := money.ParseBook(b)
	   if err1 != nil || err2 != nil {
		   return strings.TrimSpace(a) == strings.TrimSpace(b)
	  }
	   return aa.Equal(bb)
   }

   func ticketKindOf(kind string) string {
	   switch kind {
	   case KindObserveNE:
		  return string(TicketObserveGap)
	   case KindConfirmedMiss:
		  return string(TicketConfirmMiss)
	   case KindDupOutput:
		  return string(TicketDupCredit)
	  case KindEarlyRollback:
		   return string(TicketEarlyBack)
	  default:
		   return string(TicketAmtDrift)
	   }
   }

   func noteOf(kind string, in PatrolIn) string {
	   switch kind {
	   case KindObserveNE:
		   return "observe " + in.Observed + " credit " + in.Credited
	   case KindConfirmedMiss:
		  return "confirmed without credit"
	  case KindDupOutput:
		  return "duplicate output second credit"
	   case "":
		  return ""
	   default:
		  return kind
	   }
   }

  func observeState(in PatrolIn) string {
	   if in.Confirmed {
		   return "confirmed"
	   }
	   if in.Confirms > 0 {
		  return "observed"
	  }
	  return "observed"
  }

   func creditState(in PatrolIn) string {
	  if strings.TrimSpace(in.CreditState) != "" {
		   return in.CreditState
	   }
	  if in.Credited == "" {
		  return "none"
	  }
	   return "credited"
  }

   func phaseOf(in PatrolIn, kind string) string {
	  if kind == KindConfirmedMiss {
		  return PhaseNone
	  }
	  if kind != "" {
		   return PhaseHeld
	   }
	  if in.CreditState == PhaseConfirming {
		  return PhaseConfirming
	   }
	   if in.Credited != "" {
		   return PhaseCredited
	  }
	  return PhaseNone
  }
