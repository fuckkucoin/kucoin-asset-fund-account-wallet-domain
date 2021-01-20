// @FileName: ticket.go
// @Summary: 充值入账域之工单票据。
// @Author: Kian
// @Created: 2021.01.10

package deposit

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

type TicketKind string

const (
	TicketHashDrift TicketKind = "hash_drift"
	TicketAmtDrift  TicketKind = "amount_drift"
	TicketMemoMiss  TicketKind = "memo_miss"
	TicketReorgHold TicketKind = "reorg_hold"
	// 观测额与贷记额不一致。窗口内必须落单，未结不得再次贷记。
	TicketObserveGap TicketKind = "observe_gap"
	// 确认数已够但没有贷记。
	TicketConfirmMiss TicketKind = "confirm_miss"
	// 同一 (chain, tx, output) 第二次贷记。
	TicketDupCredit TicketKind = "dup_credit"
	// 提前入账后重组或确认回退。
	TicketEarlyBack TicketKind = "early_rollback"
	// 错漏备注，只能走回收工单。
	TicketWrongMemo TicketKind = "wrong_memo"
	// 错网，禁止观察线程静默贷记。
	TicketWrongNet TicketKind = "wrong_network"
	// 无法归因到用户。
	TicketUnattrib TicketKind = "unattributed"
)

type TicketState string

const (
	TicketOpen     TicketState = "open"
	TicketAssigned TicketState = "assigned"
	TicketClosed   TicketState = "closed"
)

type Exception struct {
	ID   string     `json:"id"`
	Kind TicketKind `json:"kind"`
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Asset string `json:"asset,omitempty"`
	Hash  string `json:"hash"`
	Note  string `json:"note"`
	State TicketState `json:"state"`
	OpenedBy string `json:"opened_by"`
	Assignee string `json:"assignee,omitempty"`
	ClosedBy string `json:"closed_by,omitempty"`
	Fence     persist.Fence `json:"fence"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	ErrTicketActor = fmt.Errorf("deposit ticket actor invalid")
	ErrTicketNote  = fmt.Errorf("deposit ticket note required")
	ErrTicketKind  = fmt.Errorf("deposit ticket kind invalid")
	ErrTicketMiss = fmt.Errorf("deposit ticket not found")
	ErrTicketClosed = fmt.Errorf("deposit ticket already closed")
	ErrTicketAssign = fmt.Errorf("deposit ticket opener cannot assign self")
)

type Desk struct {
	mu   sync.Mutex
	rows map[string]*Exception
	fence *persist.Table
	clock      func() time.Time
	sql        *persist.SQLDB
	hydrateErr string
	hydratedN  int
	hydratedAt time.Time
}

  func NewDesk() *Desk {
	   return &Desk{rows: map[string]*Exception{}, fence: persist.NewTable(), clock: func() time.Time { return time.Now().UTC() }}
   }

   func ticketActor(actor string) error {
	  if strings.TrimSpace(actor) == "" || strings.EqualFold(actor, "system") {
		  return ErrTicketActor
	   }
	  return nil
  }

   func parseTicketKind(s string) (TicketKind, error) {
	   switch TicketKind(strings.TrimSpace(s)) {
	  case TicketHashDrift, TicketAmtDrift, TicketMemoMiss, TicketReorgHold,
		   TicketObserveGap, TicketConfirmMiss, TicketDupCredit, TicketEarlyBack,
		   TicketWrongMemo, TicketWrongNet, TicketUnattrib:
		  return TicketKind(strings.TrimSpace(s)), nil
	  default:
		   return "", ErrTicketKind
	  }
   }

// OpenException 观察额与单据不一致时落单。未结案不得当已入账。asset 可空（兼容旧单）。

   func (d *Desk) OpenException(actor, userID, chain, asset, hash, kind, note string) (Exception, error) {
	   if d == nil {
		  return Exception{}, ErrTicketMiss
	   }
	  if err := ticketActor(actor); err != nil {
		   return Exception{}, err
	   }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		  return Exception{}, err
	   }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		   return Exception{}, err
	  }
	   as := strings.ToUpper(strings.TrimSpace(asset))
	   if as != "" {
		   as, err = validate.Asset(as)
		   if err != nil {
			  return Exception{}, err
		  }
	   }
	   k, err := parseTicketKind(kind)
	   if err != nil {
		  return Exception{}, err
	  }
	   note = strings.TrimSpace(note)
	   if note == "" {
		  return Exception{}, ErrTicketNote
	  }
	   now := d.clock()
	  row := &Exception{
		  ID: uuid.NewString(), Kind: k, UserID: uid, Chain: ch, Asset: as, Hash: strings.TrimSpace(hash),
		   Note: note, State: TicketOpen, OpenedBy: strings.TrimSpace(actor), CreatedAt: now, UpdatedAt: now,
	  }
	   f,
		  err := d.fence.Bump(row.ID, persist.NewToken())
	   if err != nil {
		  return Exception{}, err
	   }
	  row.Fence = f
	   if err := d.persistRow(row, row.OpenedBy, "open", "", string(TicketOpen)); err != nil {
		  return Exception{}, err
	  }
	   d.mu.Lock()
	   d.rows[row.ID] = row
	   d.mu.Unlock()
	  return row.copy(), nil
   }

   func (d *Desk) Assign(id, actor string, epoch int64, token string) (Exception, error) {
	   if err := ticketActor(actor); err != nil {
		  return Exception{}, err
	  }
	  if err := d.fence.Check(id, epoch, token); err != nil {
		   return Exception{}, err
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   row, ok := d.rows[id]
	  if !ok {
		   if loaded, found := d.loadSQL(id); found {
			   d.rows[id] = loaded
			   row = loaded
			  ok = true
		   }
	  }
	   if !ok {
		  return Exception{}, ErrTicketMiss
	  }
	   if row.State == TicketClosed {
		   return row.copy(), ErrTicketClosed
	   }
	  if strings.EqualFold(row.OpenedBy, strings.TrimSpace(actor)) {
		   return Exception{}, ErrTicketAssign
	   }
	  f, err := d.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return Exception{}, err
	  }
	   from := string(row.State)
	  row.State = TicketAssigned
	   row.Assignee = strings.TrimSpace(actor)
	   row.Fence = f
	   row.UpdatedAt = d.clock()
	   if err := d.persistRow(row, actor, "assign", from, string(TicketAssigned)); err != nil {
		   return Exception{}, err
	  }
	   return row.copy(), nil
  }

  func (d *Desk) Close(id, actor string, epoch int64, token string) (Exception, error) {
	   if err := ticketActor(actor); err != nil {
		   return Exception{}, err
	   }
	   if err := d.fence.Check(id, epoch, token); err != nil {
		   return Exception{}, err
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   row, ok := d.rows[id]
	   if !ok {
		   if loaded, found := d.loadSQL(id); found {
			  d.rows[id] = loaded
			   row = loaded
			   ok = true
		   }
	   }
	   if !ok {
		  return Exception{}, ErrTicketMiss
	  }
	   if row.State == TicketClosed {
		  return row.copy(), ErrTicketClosed
	   }
	  if row.Assignee == "" || strings.EqualFold(row.Assignee, strings.TrimSpace(actor)) {
		   return Exception{}, ErrTicketAssign
	  }
	  f, err := d.fence.Bump(id, persist.NewToken())
	  if err != nil {
		  return Exception{}, err
	   }
	  from := string(row.State)
	   row.State = TicketClosed
	  row.ClosedBy = strings.TrimSpace(actor)
	   row.Fence = f
	  row.UpdatedAt = d.clock()
	  if err := d.persistRow(row, actor, "close", from, string(TicketClosed)); err != nil {
		  return Exception{}, err
	   }
	  return row.copy(), nil
  }

  func (d *Desk) OpenCount(userID string) int {
	   if d == nil {
		  return 0
	   }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   n := 0
	  for _, r := range d.rows {
		  if r.State == TicketClosed {
			  continue
		   }
		  if userID != "" && r.UserID != userID {
			  continue
		  }
		  n++
	   }
	  return n
  }

  func (e Exception) copy() Exception { return e }
