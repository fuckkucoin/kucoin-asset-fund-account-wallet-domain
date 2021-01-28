package recon

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/pkg/money"
)

// DepositDiff 入金观测差字段。与入金事件契约同名。

type DepositDiff struct {
	Event string `json:"event"`
	Kind  string `json:"kind"`
	Chain       string `json:"chain"`
	TxHash      string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
	Asset       string `json:"asset"`
	UserID      string `json:"user_id"`
	ObservedAmount string `json:"observed_amount"`
	CreditedAmount string `json:"credited_amount"`
	ObserveState string `json:"observe_state"`
	CreditState  string `json:"credit_state"`
	WindowStart string `json:"window_start"`
	WindowEnd    string `json:"window_end"`
	SecondCredit bool   `json:"second_credit"`
	Blocked       bool   `json:"blocked"`
	DepositTicket string `json:"deposit_ticket_id"`
	Phase string `json:"phase"`
	ReverseEntry string `json:"reverse_entry"`
	AvailBefore string `json:"avail_before"`
	AvailAfter  string `json:"avail_after"`
	Reason string `json:"reason"`
}

// AppendDepositDiff 把字段挂到已有工单。状态、栅栏、处理人都不动。
  func (b *Book) AppendDepositDiff(id, actor string, diff DepositDiff) (*Ticket, error) {
	  if b == nil {
		   return nil, ErrStore
	  }
	   if err := actorOK(actor); err != nil {
		  return nil, err
	  }
	   if err := validateDiff(diff); err != nil {
		  return nil, err
	   }
	  b.mu.Lock()
	  t, ok := b.rows[strings.TrimSpace(id)]
	  if !ok {
		  loaded, found, err := b.loadSQL(id)
		   if err != nil {
			   b.mu.Unlock()
			  return nil, err
		   }
		   if found {
			   b.rows[id] = loaded
			  t = loaded
			  ok = true
		   }
	  }
	   if !ok || t == nil {
		   b.mu.Unlock()
		   return nil, ErrNotFound
	   }
	  cp := diff
	  t.DepositDiff = &cp
	   if b.diffs == nil {
		   b.diffs = map[string]*DepositDiff{}
	   }
	  b.diffs[t.ID] = &cp
	  b.mu.Unlock()
	   if b.sqlOn() {
		   row := diffRow(t.ID, cp)
		   if err := sqlrepo.PutReconDepositDiff(context.Background(), b.sql, row); err != nil {
			   return nil, err
		   }
		   note := cp.Kind
		  if len(note) > 240 {
			  note = note[:240]
		  }
		   _ = sqlrepo.RecordTicketEvent(context.Background(), b.sql, "recon", t.ID, actor, "deposit_diff", string(t.State), string(t.State), note)
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   cur := b.rows[t.ID]
	  if cur == nil {
		  return nil, ErrNotFound
	  }
	   out := *cur
	   return &out, nil
  }

   func (b *Book) fillDiff(t *Ticket) {
	  if b == nil || t == nil || t.DepositDiff != nil {
		  return
	  }
	   if b.diffs != nil {
		   if d, ok := b.diffs[t.ID]; ok && d != nil {
			   cp := *d
			  t.DepositDiff = &cp
			   return
		  }
	   }
	  if !b.sqlOn() {
		   return
	  }
	   row, err := sqlrepo.GetReconDepositDiff(context.Background(), b.sql, t.ID)
	  if err != nil {
		  return
	   }
	  d := diffFromRow(row)
	  t.DepositDiff = &d
	  if b.diffs == nil {
		  b.diffs = map[string]*DepositDiff{}
	   }
	   b.diffs[t.ID] = &d
   }

  func validateDiff(d DepositDiff) error {
	   switch strings.TrimSpace(d.Kind) {
	  case "observe_ne_credit", "confirmed_unccredited", "dup_output_second_credit",
		   "early_rollback", "wrong_memo", "wrong_network", "unattributed":
	   default:
		   return ErrEmpty
	   }
	  if strings.TrimSpace(d.Chain) == "" || strings.TrimSpace(d.TxHash) == "" {
		  return ErrEmpty
	   }
	  if d.OutputIndex < 0 {
		   return ErrEmpty
	   }
	  for _, amt := range []string{d.ObservedAmount, d.CreditedAmount, d.AvailBefore, d.AvailAfter} {
		   if strings.TrimSpace(amt) == "" {
			   continue
		   }
		  if _, err := money.ParseBook(amt); err != nil {
			  return ErrEmpty
		   }
	  }
	  return nil
   }

   func diffRow(ticketID string, d DepositDiff) sqlrepo.ReconDiffRow {
	   return sqlrepo.ReconDiffRow{
		  TicketID: ticketID, Event: d.Event, Kind: d.Kind, Chain: d.Chain, TxHash: d.TxHash,
		  OutputIndex: d.OutputIndex, Asset: d.Asset, UserID: d.UserID,
		  ObservedAmount: d.ObservedAmount, CreditedAmount: d.CreditedAmount,
		  ObserveState: d.ObserveState, CreditState: d.CreditState,
		  WindowStart:  d.WindowStart,
		   WindowEnd:    d.WindowEnd,
		   SecondCredit: d.SecondCredit, Blocked: d.Blocked,
		   DepositTicket: d.DepositTicket, Phase: d.Phase, ReverseEntry: d.ReverseEntry,
		  AvailBefore: d.AvailBefore, AvailAfter: d.AvailAfter, Reason: clip(d.Reason, 255),
		   CreatedAt: time.Now().UTC(),
	   }
  }

  func diffFromRow(row sqlrepo.ReconDiffRow) DepositDiff {
	   return DepositDiff{
		   Event: row.Event, Kind: row.Kind, Chain: row.Chain, TxHash: row.TxHash,
		  OutputIndex: row.OutputIndex, Asset: row.Asset, UserID: row.UserID,
		   ObservedAmount: row.ObservedAmount, CreditedAmount: row.CreditedAmount,
		   ObserveState: row.ObserveState, CreditState: row.CreditState,
		   WindowStart: row.WindowStart, WindowEnd: row.WindowEnd,
		   SecondCredit: row.SecondCredit, Blocked: row.Blocked,
		  DepositTicket: row.DepositTicket, Phase: row.Phase, ReverseEntry: row.ReverseEntry,
		   AvailBefore: row.AvailBefore, AvailAfter: row.AvailAfter, Reason: row.Reason,
	   }
   }

   func clip(s string, n int) string {
	  s = strings.TrimSpace(s)
	   if len(s) <= n {
		   return s
	   }
	   return s[:n]
   }
