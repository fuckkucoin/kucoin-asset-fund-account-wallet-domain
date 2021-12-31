package xferloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/recon"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	kindCase = "aa_xfer_loop"
	// kindHop 与母子 hop 相位行的存储键一致，工作台只读该相位，不改 saga 写入方。
	kindHop = "aa_hop_saga"
)

type Kind string

const (
	KindHalfHop Kind = "half_hop"
	KindInner   Kind = "inner_uncredited"
	KindCover Kind = "cover_stranded"
)

type State string

const (
	StateOpen       State = "open"
	StateRecovering State = "recovering"
	StateRecovered  State = "recovered"
	StateEscalated  State = "escalated"
)

var ErrNotFound = errors.New("xfer loop case not found")

// RecoverHop 续跑或补偿一条未完成的母子 hop。由资金进程注入，避免工作台反向依赖资金 HTTP。

type RecoverHop func(ctx context.Context, clientOid string) error

type Case struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	Ref  string `json:"ref"`
	UserID string `json:"user_id"`
	Asset string `json:"asset,omitempty"`
	Phase string `json:"phase,omitempty"`
	State State  `json:"state"`
	Attempts int `json:"attempts"`
	LastError  string `json:"last_error,omitempty"`
	TransferID string `json:"transfer_id,omitempty"`
	Idem       string `json:"idem,omitempty"`
	SeenAt    time.Time `json:"seen_at"`
	UpdatedAt time.Time `json:"updated_at"`
	DueAt time.Time `json:"due_at,omitempty"`
	Severity string `json:"severity,omitempty"`
	Policy    string `json:"policy,omitempty"`
	SLABreach bool   `json:"sla_breach,omitempty"`
	Edge      string `json:"edge,omitempty"`
	Assignee  string `json:"assignee,omitempty"`
}

type Report struct {
	Scanned   int `json:"scanned"`
	Recovered int `json:"recovered"`
	Escalated int `json:"escalated"`
	Open int `json:"open"`
}

type Book struct {
	led     *ledger.Service
	kv      persist.KV
	tickets *recon.Book
	recover RecoverHop
	minAge  time.Duration
	mu sync.Mutex
	rows     map[string]*Case
	hydrated bool
}

  func New(led *ledger.Service, kv persist.KV, tickets *recon.Book, recover RecoverHop) *Book {
	   if kv == nil {
		   kv = persist.NewMemKV()
	  }
	  return &Book{
		   led: led, kv: kv, tickets: tickets, recover: recover,
		  minAge: 14 * time.Second,
		  rows:   map[string]*Case{},
	  }
   }

var live struct {
	mu sync.Mutex
	b  *Book
}

// BindLive 让历史读面与运维查询看到同一本工作台。

   func BindLive(b *Book) {
	  live.mu.Lock()
	   live.b = b
	  live.mu.Unlock()
  }

   func Live() *Book {
	  live.mu.Lock()
	   defer live.mu.Unlock()
	  return live.b
   }

// Ensure 进程内只保留一本工作台，避免周期任务和读面各记各的。
  func Ensure(led *ledger.Service, kv persist.KV, tickets *recon.Book, recover RecoverHop) *Book {
	  live.mu.Lock()
	   defer live.mu.Unlock()
	  if live.b != nil {
		  return live.b
	   }
	   live.b = New(led, kv, tickets, recover)
	  return live.b
   }

   func (b *Book) List(limit int) []Case {
	   if b == nil {
		  return nil
	   }
	  _ = b.hydrate(context.Background())
	   if limit <= 0 || limit > 200 {
		   limit = 50
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]Case,
		  0, len(b.rows))
	   for _, c := range b.rows {
		  if c == nil || c.State == StateRecovered {
			  continue
		   }
		   out = append(out, *c)
		  if len(out) >= limit {
			  break
		   }
	  }
	  return out
  }

// Retry 人工把已升级的单拉回再试一次。已恢复的不再动。

   func (b *Book) Retry(ctx context.Context, id string) (Case, error) {
	  if b == nil {
		   return Case{}, ErrNotFound
	  }
	   _ = b.hydrate(ctx)
	   b.mu.Lock()
	   c := b.rows[strings.TrimSpace(id)]
	  if c == nil {
		  b.mu.Unlock()
		   return Case{}, ErrNotFound
	   }
	   if c.State == StateRecovered {
		  cp := *c
		  b.mu.Unlock()
		   return cp, nil
	   }
	   c.State = StateOpen
	  c.Attempts = 0
	   c.LastError = ""
	  c.UpdatedAt = time.Now().UTC()
	  reopenSLA(c, c.UpdatedAt)
	  cp := *c
	   b.mu.Unlock()
	  _ = b.save(ctx, &cp)
	   rep := Report{}
	  b.drive(ctx, &cp, &rep)
	  return cp, nil
   }

// Tick 扫描并收口。年轻单据只更新读面。
  func (b *Book) Tick(ctx context.Context) (Report, error) {
	  var rep Report
	   if b == nil || b.led == nil || b.led.Store() == nil {
		  return rep, nil
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	   if err := b.hydrate(ctx); err != nil {
		   return rep, err
	  }
	  b.scanHops(ctx, &rep)
	  b.scanInner(ctx, &rep)
	  b.scanCovers(ctx, &rep)
	  b.mu.Lock()
	   for _, c := range b.rows {
		  if c != nil && c.State != StateRecovered {
			  rep.Open++
		   }
	  }
	  b.mu.Unlock()
	   return rep, nil
   }

  func (b *Book) scanHops(ctx context.Context, rep *Report) {
	  if b.kv == nil {
		  return
	   }
	   after := ""
	   seen := 0
	   for seen < 200 {
		   page, err := persist.PageKind(ctx, b.kv, kindHop, persist.Cursor{After: after, Limit: 50})
		   if err != nil {
			   return
		   }
		  for _, row := range page.Rows {
			  seen++
			   var hop struct {
				  ClientOid string `json:"client_oid"`
				  Phase     string `json:"phase"`
				   CompPhase string `json:"comp_phase"`
				  UserID   string `json:"user_id"`
				  Currency string `json:"currency"`
			   }
			  if json.Unmarshal(row.Body, &hop) != nil || hop.ClientOid == "" {
				  continue
			  }
			   if hop.Phase == "done" || hop.Phase == "aborted" {
				  b.markRecovered(ctx, KindHalfHop, hop.ClientOid)
				  continue
			  }
			   rep.Scanned++
			  phase := hop.Phase
			   if hop.CompPhase != "" {
				   phase = hop.Phase + ":" + hop.CompPhase
			  }
			  c := b.upsert(ctx, KindHalfHop, hop.ClientOid, hop.UserID, hop.Currency, phase, "", hop.ClientOid)
			   edge := classifyHop(c.Phase)
			   c.Edge = edge.Code
			  b.publish(c)
			   if c.State == StateEscalated {
				  continue
			   }
			  if edge.Action == actionRecovered {
				  b.markRecovered(ctx, KindHalfHop, hop.ClientOid)
				   continue
			  }
			  if edge.Action == actionEscalate {
				  b.escalate(ctx, c, edge.Reason, rep)
				  continue
			  }
			   if !b.ripe(c) || edge.Action == actionWait {
				  continue
			   }
			  if slaEscalate(c) {
				  b.escalate(ctx, c, "sla breach", rep)
				  continue
			  }
			   b.drive(ctx, c, rep)
		   }
		   if page.Done || page.Next == "" {
			  break
		   }
		  after = page.Next
	  }
   }

   func (b *Book) scanInner(ctx context.Context, rep *Report) {
	  states := []ledger.State{
		   ledger.StateInitiated, ledger.StateSpotFrozen, ledger.StateWalletLocked,
		   ledger.StateConfirmed, ledger.StateFailed, ledger.StateChainSubmitted,
		  ledger.StateHold, ledger.StateCredited,
	   }
	  for _, st := range states {
		  list, err := b.led.Store().ListByState(ctx, st, 200)
		  if err != nil {
			  continue
		  }
		   for _, tr := range list {
			  if tr == nil || tr.Direction != ledger.SpotSend {
				   continue
			  }
			   rep.Scanned++
			   gap := ""
			   if tr.State == ledger.StateCredited {
				   j, err := b.led.Store().ListJournal(ctx, tr.ID)
				   if err != nil {
					  continue
				   }
				  reason, ok := counterpartyGap(j, tr.Counterparty(), tr.Amount)
				  if ok {
					  b.markRecovered(ctx, KindInner, tr.ID)
					  continue
				  }
				  gap = reason
			  }
			   c := b.upsert(ctx, KindInner, tr.ID, tr.UserID, tr.Asset, string(tr.State), tr.ID, tr.IdempotencyKey)
			   edge := classifyInner(tr)
			   if edge.Code != "" {
				  c.Edge = edge.Code
			  }
			   if edge.Action == actionEscalate {
				   c.Phase = edge.Code
				   b.publish(c)
				   if c.State != StateEscalated {
					   b.escalate(ctx, c, edge.Reason, rep)
				   }
				  continue
			  }
			   if gap != "" {
				   c.Phase = gapPhase(gap)
				   b.publish(c)
				   if c.State != StateEscalated {
					  b.escalate(ctx, c, gap, rep)
				  }
				   continue
			   }
			   if tr.CreatedAt.IsZero() || time.Since(tr.CreatedAt) < b.minAge {
				   continue
			  }
			   if c.State == StateEscalated {
				   b.publish(c)
				  continue
			   }
			   if slaEscalate(c) {
				  b.publish(c)
				   b.escalate(ctx, c, "sla breach", rep)
				  continue
			   }
			   b.publish(c)
			  b.drive(ctx, c, rep)
		  }
	   }
  }

   func (b *Book) scanCovers(ctx context.Context, rep *Report) {
	   list, err := b.led.Store().ListByState(ctx, ledger.StateCredited, 200)
	  if err != nil {
		   return
	   }
	  var stranded []*ledger.Transfer
	   for _, tr := range list {
		  if tr == nil || !ledger.IsAutoCoverKey(tr.IdempotencyKey) {
			  continue
		   }
		   if b.coverStranded(ctx, tr) {
			  stranded = append(stranded, tr)
			  rep.Scanned++
		   } else {
			  b.markRecovered(ctx, KindCover, tr.ID)
		  }
	   }
	   if len(stranded) == 0 {
		   return
	   }
	   _,
		   _ = b.led.SweepStrandedCovers(ctx, 200)
	   for _, tr := range stranded {
		   c := b.upsert(ctx, KindCover, tr.ID, tr.UserID, tr.Asset, "credited", tr.ID, tr.IdempotencyKey)
		  c.Edge = b.coverEdge(ctx, tr).Code
		   if !b.coverStranded(ctx, tr) {
			  b.finish(ctx, c, rep)
			   continue
		  }
		  if edge := b.coverEdge(ctx, tr); edge.Action == actionWait {
			  c.Edge = edge.Code
			  b.publish(c)
			   continue
		   }
		   if tr.CreatedAt.IsZero() || time.Since(tr.CreatedAt) < b.minAge {
			  continue
		   }
		   if c.State == StateEscalated {
			  b.publish(c)
			   continue
		   }
		  if slaEscalate(c) {
			  b.publish(c)
			  b.escalate(ctx, c, "sla breach", rep)
			  continue
		  }
		   b.publish(c)
		   c.Attempts++
		   c.State = StateRecovering
		   c.LastError = "cover still stranded after sweep"
		  c.UpdatedAt = time.Now().UTC()
		  if c.Attempts >= policyOf(c.Kind).MaxAttempts {
			  b.escalate(ctx,
				   c, c.LastError, rep)
			  continue
		   }
		  _ = b.save(ctx, c)
		   metrics.XferLoop.WithLabelValues(string(c.Kind), "retry").Inc()
	   }
   }

  func (b *Book) coverStranded(ctx context.Context, tr *ledger.Transfer) bool {
	  if tr == nil || !ledger.IsAutoCoverKey(tr.IdempotencyKey) || tr.State != ledger.StateCredited {
		   return false
	  }
	  parent := ledger.CoverParentKey(tr.IdempotencyKey)
	  main, err := b.led.ByIdempotency(ctx, parent)
	   if err == nil && main != nil && main.State != ledger.StateFailed && main.State != ledger.StateRolledBack {
		   return false
	  }
	  undo, uerr := b.led.ByIdempotency(ctx, tr.IdempotencyKey+":undo")
	  if uerr == nil && undo != nil && undo.State == ledger.StateCredited {
		  return false
	   }
	   return true
   }

  func (b *Book) drive(ctx context.Context, c *Case, rep *Report) {
	   if c == nil || c.State == StateEscalated || c.State == StateRecovered {
		   return
	   }
	  c.State = StateRecovering
	   c.Attempts++
	   c.UpdatedAt = time.Now().UTC()
	  var err error
	  switch c.Kind {
	  case KindHalfHop:
		   if b.recover == nil {
			   err = errors.New("hop recover unavailable")
		   } else {
			   err = b.recover(ctx, c.Ref)
		   }
		   if err == nil && b.hopOpen(ctx, c.Ref) {
			  err = errors.New("hop still open")
		  }
	  case KindInner:
		  err = b.resumeInner(ctx, c)
	   default:
		   err = errors.New("unsupported loop kind")
	   }
	   if err == nil {
		   b.finish(ctx, c, rep)
		   return
	   }
	  c.LastError = err.Error()
	   if c.Attempts >= policyOf(c.Kind).MaxAttempts || slaEscalate(c) {
		   reason := err.Error()
		   if c.SLABreach && c.Attempts < policyOf(c.Kind).MaxAttempts {
			   reason = "sla breach: " + reason
		   }
		   b.escalate(ctx, c, reason, rep)
		   return
	   }
	   c.State = StateOpen
	   _ = b.save(ctx, c)
	  b.publish(c)
	  metrics.XferLoop.WithLabelValues(string(c.Kind), "retry").Inc()
   }

  func (b *Book) resumeInner(ctx context.Context, c *Case) error {
	  tr,
		  err := b.led.Get(ctx, c.TransferID)
	   if err != nil {
		  return err
	   }
	  if tr.State == ledger.StateCredited {
		  j, jerr := b.led.Store().ListJournal(ctx, tr.ID)
		   if jerr != nil {
			  return jerr
		  }
		   reason,
			   ok := counterpartyGap(j, tr.Counterparty(), tr.Amount)
		   if !ok {
			  return errors.New(reason)
		  }
		   return nil
	  }
	   _, err = b.led.Create(ctx, ledger.CreateReq{
		  UserID: tr.UserID, Direction: tr.Direction, Asset: tr.Asset, Amount: tr.Amount,
		   ToAddress: tr.Counterparty(),
		  IdempotencyKey: tr.IdempotencyKey,
	  })
	   return err
  }

  func (b *Book) hopOpen(ctx context.Context, oid string) bool {
	  if b.kv == nil {
		  return true
	  }
	   row, err := b.kv.Get(ctx, kindHop, oid)
	   if err != nil {
		  return true
	   }
	  var hop struct {
		   Phase string `json:"phase"`
	   }
	   if json.Unmarshal(row.Body, &hop) != nil {
		  return true
	  }
	   return hop.Phase != "done" && hop.Phase != "aborted"
  }

  func (b *Book) finish(ctx context.Context, c *Case, rep *Report) {
	  c.State = StateRecovered
	   c.LastError = ""
	   c.UpdatedAt = time.Now().UTC()
	  _ = b.save(ctx, c)
	   b.clearPub(c)
	   if rep != nil {
		   rep.Recovered++
	  }
	   metrics.XferLoop.WithLabelValues(string(c.Kind), "recovered").Inc()
   }

  func (b *Book) escalate(ctx context.Context, c *Case, reason string, rep *Report) {
	   c.State = StateEscalated
	   c.LastError = reason
	  c.UpdatedAt = time.Now().UTC()
	   _ = b.save(ctx, c)
	  b.publish(c)
	  if rep != nil {
		   rep.Escalated++
	   }
	  metrics.XferLoop.WithLabelValues(string(c.Kind), "escalated").Inc()
	  if b.tickets == nil {
		   return
	   }
	  note := "xferloop:" + string(c.Kind) + ":" + c.Ref
	   if reason != "" {
		  note += " " + reason
	   }
	   if hasOpenTicket(b.tickets, c.UserID, note) {
		  return
	  }
	  user := c.UserID
	   if strings.TrimSpace(user) == "" {
		  user = "platform"
	  }
	   _, _ = b.tickets.Open("xfer-loop", user, string(c.Kind), c.Asset, note)
  }

   func (b *Book) ripe(c *Case) bool {
	   if c == nil {
		  return false
	  }
	   if c.SeenAt.IsZero() {
		   return false
	  }
	  return time.Since(c.SeenAt) >= b.minAge
  }

  func (b *Book) upsert(ctx context.Context, kind Kind, ref, user, asset, phase, transferID, idem string) *Case {
	  key := string(kind) + "|" + ref
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   if cur := b.rows[key]; cur != nil {
		   cur.Phase = phase
		   if transferID != "" {
			  cur.TransferID = transferID
		   }
		   if idem != "" {
			   cur.Idem = idem
		  }
		  applyPolicy(cur, time.Now().UTC())
		  return cur
	   }
	   now := time.Now().UTC()
	   c := &Case{
		  ID: uuid.NewString(), Kind: kind, Ref: ref, UserID: user, Asset: asset,
		   Phase: phase, State: StateOpen, TransferID: transferID, Idem: idem,
		  SeenAt: now, UpdatedAt: now,
	  }
	  applyPolicy(c, now)
	   b.rows[key] = c
	   b.mu.Unlock()
	   _ = b.save(ctx, c)
	   b.mu.Lock()
	   return c
  }

   func (b *Book) markRecovered(ctx context.Context, kind Kind, ref string) {
	   key := string(kind) + "|" + ref
	  b.mu.Lock()
	   c := b.rows[key]
	   b.mu.Unlock()
	  if c == nil || c.State == StateRecovered {
		   if c != nil {
			  b.clearPub(c)
		  }
		   return
	   }
	  b.finish(ctx, c, nil)
  }

  func (b *Book) publish(c *Case) {
	  if c == nil {
		  return
	   }
	  st := railOf(c)
	   for _, ref := range refsOf(c) {
		   ledger.PublishLoop(ref, st)
	   }
   }

   func (b *Book) clearPub(c *Case) {
	   for _, ref := range refsOf(c) {
		   ledger.ClearLoop(ref)
	  }
  }

   func refsOf(c *Case) []string {
	   if c == nil {
		  return nil
	  }
	   out := []string{c.Ref, c.ID}
	   if c.TransferID != "" {
		  out = append(out, c.TransferID)
	   }
	  if c.Idem != "" {
		   out = append(out, c.Idem)
	  }
	   if c.Kind == KindHalfHop && c.Ref != "" {
		   out = append(out, c.Ref+":ledger-out")
	  }
	   return out
  }

   func railOf(c *Case) string {
	   st := mapUserStatus(c)
	   if st.Code == "" {
		   return ledger.RailRecovering
	   }
	  return st.Code
  }

   func (b *Book) coverEdge(ctx context.Context, tr *ledger.Transfer) Edge {
	   if b == nil || b.led == nil || tr == nil {
		   return Edge{Action: actionSkip, Code: "not_cover"}
	   }
	  parentKey := ledger.CoverParentKey(tr.IdempotencyKey)
	  parent, perr := b.led.ByIdempotency(ctx, parentKey)
	   undo, uerr := b.led.ByIdempotency(ctx, tr.IdempotencyKey+":undo")
	   return classifyCover(CoverView{
		  Cover: tr, Parent: parent, ParentMissing: perr != nil || parent == nil,
		   Undo: undo, UndoMissing: uerr != nil || undo == nil,
		  Now: time.Now().UTC(),
		  MinAge: b.minAge,
	   })
   }

// counterpartyGap 核对对手贷记是否与转出金额一致。
// 少记、多记、完全没有贷记都返回原因；金额一致才算收口。已入账后不得再补一笔。

   func counterpartyGap(j []ledger.JournalEntry, dest string, amt decimal.Decimal) (string, bool) {
	   dest = strings.TrimSpace(dest)
	   if dest == "" {
		  return "counterparty missing", false
	   }
	  got := decimal.Zero
	  for _, e := range j {
		   if e.UserID == dest && e.Memo == "credit_available" && e.Credit.IsPositive() {
			  got = got.Add(e.Credit)
		   }
	   }
	  switch {
	   case got.Equal(amt):
		  return "", true
	   case got.GreaterThan(amt):
		   return "counterparty credit exceeds transfer", false
	   case got.IsPositive():
		   return "counterparty credit short", false
	  default:
		   return "counterparty credit journal missing", false
	  }
   }

  func gapPhase(reason string) string {
	   switch reason {
	  case "counterparty credit short":
		  return "counterparty_short"
	  case "counterparty credit exceeds transfer":
		   return "counterparty_over"
	   default:
		  return "counterparty_missing"
	   }
  }

  func hasOpenTicket(tickets *recon.Book, user, note string) bool {
	   if tickets == nil {
		   return false
	   }
	  prefix := note
	  if i := strings.Index(note, " "); i > 0 {
		  prefix = note[:i]
	   }
	   for _, t := range tickets.List(user, 200) {
		  if t.State != recon.StateOpen && t.State != recon.StateAcked {
			  continue
		   }
		  if strings.Contains(t.Note, prefix) {
			  return true
		   }
	  }
	  return false
  }

  func (b *Book) save(ctx context.Context, c *Case) error {
	  if b == nil || b.kv == nil || c == nil {
		   return nil
	   }
	   body, err := json.Marshal(c)
	   if err != nil {
		   return err
	   }
	   key := string(c.Kind) + "|" + c.Ref
	   return b.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, ferr := b.kv.Get(ctx, kindCase, key)
		   exp := persist.Fence{}
		   if ferr == nil {
			  exp = cur.Fence
		  } else if !errors.Is(ferr, persist.ErrNotFound) {
			  return ferr
		   }
		   return b.kv.Put(ctx, persist.Row{Kind: kindCase, Key: key, Body: body}, exp)
	  })
   }

  func (b *Book) hydrate(ctx context.Context) error {
	  if b == nil {
		  return nil
	   }
	  b.mu.Lock()
	  if b.hydrated {
		   b.mu.Unlock()
		   return nil
	  }
	   b.mu.Unlock()
	   if b.kv == nil {
		   b.mu.Lock()
		   b.hydrated = true
		   b.mu.Unlock()
		   return nil
	  }
	  after := ""
	   loaded := map[string]*Case{}
	   for {
		  page, err := persist.PageKind(ctx, b.kv, kindCase, persist.Cursor{After: after, Limit: 100})
		   if err != nil {
			   return err
		   }
		  for _, row := range page.Rows {
			  var c Case
			  if json.Unmarshal(row.Body, &c) != nil || c.Ref == "" {
				   continue
			  }
			   cp := c
			  loaded[string(c.Kind)+"|"+c.Ref] = &cp
			   if cp.State != StateRecovered {
				  b.publish(&cp)
			   }
		  }
		  if page.Done || page.Next == "" {
			  break
		   }
		  after = page.Next
	  }
	   b.mu.Lock()
	  if !b.hydrated {
		  for k, c := range loaded {
			  if _, ok := b.rows[k]; !ok {
				   b.rows[k] = c
			   }
		  }
		   b.hydrated = true
	   }
	   b.mu.Unlock()
	  return nil
  }

// RailKey 按幂等键或 clientOid 取收口状态，供母子 hop 读面。
   func RailKey(ref string) (string, bool) {
	  if st, ok := ledger.LoopRailKey(ref); ok {
		  return st, true
	   }
	  b := Live()
	  if b == nil || strings.TrimSpace(ref) == "" {
		   return "", false
	   }
	   _ = b.hydrate(context.Background())
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  for _, c := range b.rows {
		  if c == nil || c.State == StateRecovered {
			  continue
		  }
		  if c.Ref == ref || c.Idem == ref || c.TransferID == ref {
			  return railOf(c), true
		  }
	  }
	  return "", false
   }

// Rail 历史接口用的收口状态。内存提示先，没有则看工作台案件。

  func Rail(tr *ledger.Transfer) (string, bool) {
	   if st, ok := ledger.LoopRail(tr); ok {
		  return st, true
	   }
	  b := Live()
	  if b == nil || tr == nil {
		  return "", false
	   }
	   _ = b.hydrate(context.Background())
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   for _, c := range b.rows {
		  if c == nil || c.State == StateRecovered {
			  continue
		   }
		  if c.TransferID == tr.ID || c.Ref == tr.ID || c.Idem == tr.IdempotencyKey || c.Ref == tr.IdempotencyKey {
			   st := railOf(c)
			  if st == "" {
				  continue
			   }
			   if tr.State == ledger.StateCredited || tr.State == ledger.StateRolledBack {
				   if st != ledger.RailCoverStranded && st != ledger.RailCounterparty && st != ledger.RailEscalated {
					  continue
				   }
			  }
			   return st, true
		   }
	  }
	  return "", false
   }
