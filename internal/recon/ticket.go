package recon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

type State string

const (
	StateOpen     State = "open"
	StateAcked    State = "acked"
	StateResolved State = "resolved"
	StateRejected State = "rejected"
)

type Ticket struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Book  string `json:"book,omitempty"`
	Asset string `json:"asset,omitempty"`
	Note  string `json:"note"`
	State State `json:"state"`
	OpenedBy string `json:"opened_by"`
	Assignee string `json:"assignee,omitempty"`
	AckedBy  string `json:"acked_by,omitempty"`
	ClosedBy string        `json:"closed_by,omitempty"`
	Fence    persist.Fence `json:"fence"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// DepositDiff 入金观测差事件。只追加字段，不改变工单状态机。
	DepositDiff *DepositDiff `json:"deposit_diff,omitempty"`
}

type Mismatch struct {
	UserID string
	Book   string
	Asset  string
	Note   string
}

var (
	ErrNotFound = fmt.Errorf("reconcile ticket not found")
	ErrClosed   = fmt.Errorf("reconcile ticket already closed")
	ErrActor = fmt.Errorf("reconcile actor required")
	ErrEmpty = fmt.Errorf("reconcile note required")
	ErrStore = fmt.Errorf("reconcile ticket store unavailable")
	ErrAssign = fmt.Errorf("reconcile assignee required")
)

type Book struct {
	mu sync.Mutex
	clock func() time.Time
	rows  map[string]*Ticket
	fence *persist.Table
	sql   *persist.SQLDB
	diffs map[string]*DepositDiff
}

  func New() *Book {
	   return &Book{
		  clock: func() time.Time { return time.Now().UTC() },
		  rows: map[string]*Ticket{},
		  fence: persist.NewTable(),
	  }
  }

  func actorOK(actor string) error {
	  if strings.TrimSpace(actor) == "" || strings.EqualFold(actor, "system") {
		  return ErrActor
	   }
	   return nil
   }

// Open 记一条差异。note 空拒。同一用户可有多张开单。

  func (b *Book) Open(actor, userID, book, asset, note string) (*Ticket, error) {
	  if err := actorOK(actor); err != nil {
		   return nil, err
	   }
	   note = strings.TrimSpace(note)
	   if note == "" {
		   return nil, ErrEmpty
	  }
	   now := b.clock()
	  t := &Ticket{
		   ID: uuid.NewString(), UserID: strings.TrimSpace(userID), Book: book, Asset: asset,
		   Note:  note,
		   State: StateOpen, OpenedBy: strings.TrimSpace(actor),
		   CreatedAt: now, UpdatedAt: now,
	   }
	   f, err := b.fence.Bump(t.ID, persist.NewToken())
	   if err != nil {
		  return nil, err
	  }
	  t.Fence = f
	   if err := b.persistRow(t, t.OpenedBy, "open", "", string(StateOpen)); err != nil {
		   return nil, err
	   }
	   b.mu.Lock()
	  b.rows[t.ID] = t
	  b.mu.Unlock()
	  metrics.ReconTickets.WithLabelValues("open").Inc()
	  b.publishBacklog()
	   ingest.Try(context.Background(), ingest.ReconFact(t.UserID, t.Asset, t.ID, t.OpenedBy, t.Note, now))
	   cp := *t
	  return &cp, nil
  }

   func (b *Book) OpenFrom(actor string, mm []Mismatch) ([]Ticket, error) {
	  out := make([]Ticket, 0, len(mm))
	   for _, m := range mm {
		  got, err := b.Open(actor, m.UserID, m.Book, m.Asset, m.Note)
		   if err != nil {
			   return out, err
		  }
		   out = append(out, *got)
	  }
	   return out, nil
   }

   func (b *Book) Ack(id, actor string, epoch int64, token string) (*Ticket, error) {
	   return b.transit(id, actor, epoch, token, StateAcked)
  }

// Assign 把未关工单派给处理人。开单人可派，结案仍走 Resolve。

   func (b *Book) Assign(id, actor, assignee string, epoch int64, token string) (*Ticket, error) {
	   if err := actorOK(actor); err != nil {
		  return nil, err
	   }
	   assignee = strings.TrimSpace(assignee)
	   if err := actorOK(assignee); err != nil {
		   return nil, ErrAssign
	   }
	  got, err := b.Ack(id, actor, epoch, token)
	  if err != nil && !errors.Is(err, ErrClosed) {
		   return got, err
	  }
	  if got == nil {
		  return nil, ErrNotFound
	   }
	   if got.State == StateResolved || got.State == StateRejected {
		  return got, ErrClosed
	  }
	   b.mu.Lock()
	   cur, ok := b.rows[got.ID]
	  if !ok {
		  b.mu.Unlock()
		   return got, ErrNotFound
	   }
	   prev := cur.Assignee
	  cur.Assignee = assignee
	  cur.UpdatedAt = b.clock()
	   if err := b.persistRow(cur, actor, "assign", string(cur.State), string(cur.State)); err != nil {
		   cur.Assignee = prev
		  b.mu.Unlock()
		  return nil, err
	   }
	  cp := *cur
	   b.mu.Unlock()
	   metrics.ReconTickets.WithLabelValues("assign").Inc()
	   b.publishBacklog()
	  return &cp, nil
  }

   func (b *Book) Resolve(id, actor string, epoch int64, token string) (*Ticket, error) {
	  return b.transit(id, actor, epoch, token, StateResolved)
  }

   func (b *Book) Reject(id, actor string, epoch int64, token string) (*Ticket, error) {
	   return b.transit(id,
		  actor, epoch, token, StateRejected)
  }

   func (b *Book) transit(id, actor string, epoch int64, token string, to State) (*Ticket, error) {
	   if err := actorOK(actor); err != nil {
		  return nil, err
	   }
	   if err := b.fence.Check(id, epoch, token); err != nil {
		  return nil, err
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  t, ok := b.rows[id]
	   if !ok {
		   loaded, found, err := b.loadSQL(id)
		  if err != nil {
			   return nil, err
		   }
		  if found {
			   b.rows[id] = loaded
			  t = loaded
			  ok = true
		   }
	   }
	   if !ok {
		  return nil, ErrNotFound
	   }
	   if t.State == StateResolved || t.State == StateRejected {
		   cp := *t
		  return &cp, ErrClosed
	   }
	  if to == StateAcked && t.State != StateOpen {
		   cp := *t
		   return &cp, ErrClosed
	  }
	  if (to == StateResolved || to == StateRejected) && t.State != StateOpen && t.State != StateAcked {
		  cp := *t
		   return &cp, ErrClosed
	  }
	   f, err := b.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return nil, err
	  }
	  from := string(t.State)
	  t.State = to
	  t.Fence = f
	   t.UpdatedAt = b.clock()
	  if to == StateAcked {
		  t.AckedBy = strings.TrimSpace(actor)
	  } else {
		  t.ClosedBy = strings.TrimSpace(actor)
	   }
	  if err := b.persistRow(t, actor, string(to), from, string(to)); err != nil {
		   return nil, err
	  }
	  metrics.ReconTickets.WithLabelValues(string(to)).Inc()
	  metrics.ReconBacklog.Set(b.backlogLocked())
	  cp := *t
	   return &cp, nil
   }

   func (b *Book) Get(id string) (*Ticket, error) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   t, ok := b.rows[id]
	   if !ok {
		  loaded,
			  found, err := b.loadSQL(id)
		  if err != nil {
			  return nil, err
		  }
		  if found {
			  b.rows[id] = loaded
			   t = loaded
			  ok = true
		  }
	   }
	   if !ok {
		   return nil, ErrNotFound
	   }
	   b.fillDiff(t)
	   cp := *t
	   return &cp, nil
   }

  func (b *Book) List(userID string, limit int) []Ticket {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	  }
	   out := make([]Ticket, 0, limit)
	  for _, t := range b.rows {
		  if userID != "" && t.UserID != userID {
			  continue
		   }
		  out = append(out, *t)
		   if len(out) >= limit {
			   break
		  }
	  }
	   return out
  }

  func (b *Book) OpenCount(userID string) int {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   return b.openCountLocked(userID)
   }

   func (b *Book) openCountLocked(userID string) int {
	  n := 0
	   for _, t := range b.rows {
		   if t.State != StateOpen && t.State != StateAcked {
			  continue
		   }
		  if userID != "" && t.UserID != userID {
			   continue
		   }
		  n++
	  }
	   return n
  }

  func (b *Book) backlogLocked() float64 {
	  return float64(b.openCountLocked(""))
  }

  func (b *Book) publishBacklog() {
	  if b == nil {
		   return
	  }
	  metrics.ReconBacklog.Set(float64(b.OpenCount("")))
  }
