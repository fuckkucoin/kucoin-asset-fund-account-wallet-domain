// 出金提现域之工单票据。

// Author: Cason
// Date: 2020.11.03

package withdraw

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
	TicketStuck     TicketKind = "stuck"
	TicketReplace   TicketKind = "replace"
	TicketAllowMiss TicketKind = "allowlist_miss"
	TicketSignHold  TicketKind = "sign_hold"
	TicketConserve TicketKind = "conserve"
	TicketHalfFreeze TicketKind = "half_freeze"
	TicketLiveHash TicketKind = "live_hash"
)

type TicketState string

const (
	TicketOpen     TicketState = "open"
	TicketAssigned TicketState = "assigned"
	TicketClosed TicketState = "closed"
)

type Exception struct {
	ID string `json:"id"`
	Kind       TicketKind `json:"kind"`
	UserID     string     `json:"user_id"`
	TransferID string     `json:"transfer_id,omitempty"`
	Note       string     `json:"note"`
	State TicketState `json:"state"`
	OpenedBy string        `json:"opened_by"`
	Assignee string        `json:"assignee,omitempty"`
	ClosedBy string        `json:"closed_by,omitempty"`
	Fence    persist.Fence `json:"fence"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	ErrTicketActor = fmt.Errorf("withdraw ticket actor invalid")
	ErrTicketNote = fmt.Errorf("withdraw ticket note required")
	ErrTicketKind = fmt.Errorf("withdraw ticket kind invalid")
	ErrTicketMiss   = fmt.Errorf("withdraw ticket not found")
	ErrTicketClosed = fmt.Errorf("withdraw ticket already closed")
	ErrTicketAssign = fmt.Errorf("withdraw ticket requires a distinct closer")
)

type Desk struct {
	mu    sync.Mutex
	rows  map[string]*Exception
	fence *persist.Table
	clock func() time.Time
	sql *persist.SQLDB
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

   func parseWDKind(s string) (TicketKind, error) {
	  switch TicketKind(strings.TrimSpace(s)) {
	  case TicketStuck, TicketReplace, TicketAllowMiss, TicketSignHold, TicketConserve, TicketHalfFreeze, TicketLiveHash:
		   return TicketKind(strings.TrimSpace(s)), nil
	  default:
		  return "", ErrTicketKind
	   }
  }

// OpenException 出金卡单或白名单缺失落单。未结案不得当已广播成功。

   func (d *Desk) OpenException(actor, userID, transferID, kind, note string) (Exception, error) {
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
	  k, err := parseWDKind(kind)
	  if err != nil {
		  return Exception{}, err
	  }
	  note = strings.TrimSpace(note)
	   if note == "" {
		   return Exception{}, ErrTicketNote
	  }
	  now := d.clock()
	  row := &Exception{
		  ID: uuid.NewString(), Kind: k, UserID: uid, TransferID: strings.TrimSpace(transferID),
		  Note: note, State: TicketOpen, OpenedBy: strings.TrimSpace(actor), CreatedAt: now, UpdatedAt: now,
	   }
	  f, err := d.fence.Bump(row.ID, persist.NewToken())
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
	  return *row, nil
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
		  return *row, ErrTicketClosed
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
	   return *row, nil
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
		  return *row, ErrTicketClosed
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
	   return *row, nil
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
