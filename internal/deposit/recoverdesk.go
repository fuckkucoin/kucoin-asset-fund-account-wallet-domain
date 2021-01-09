// 错备注、错网、无法归因的回收台。
// 观察线程见到未批准的回收单必须停写，不能按请求用户把这笔钱记进钱包。
// 批准要由另一名处理人完成，开单人不能自批，也不能派给自己。

// Author: Blaise
// Date: 2021.01.08

package deposit

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

const (
	RecoverOpen = "open"
	RecoverAssigned = "assigned"
	RecoverApproved = "approved"
	RecoverRejected = "rejected"
)

// Recovery 一笔不能在观察线程入账的链上款。
type Recovery struct {
	ID string `json:"id"`
	Kind   string `json:"kind"`
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Network     string `json:"network,omitempty"`
	Asset       string `json:"asset"`
	TxHash      string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
	Memo        string `json:"memo,omitempty"`
	Tag string `json:"tag,omitempty"`
	Note string `json:"note"`
	State    string `json:"state"`
	OpenedBy string `json:"opened_by"`
	Assignee string `json:"assignee,omitempty"`
	ApprovedBy string    `json:"approved_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RecoverDesk 回收台。批准前观察线程不得把这笔贷记进钱包。

type RecoverDesk struct {
	mu sync.Mutex
	rows map[string]*Recovery
	byOut map[string]string
	clock func() time.Time
}

   func NewRecoverDesk() *RecoverDesk {
	  return &RecoverDesk{
		  rows:  map[string]*Recovery{},
		  byOut: map[string]string{},
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// Open 落一张回收单。同一输出的未结单回放，不另开。

   func (d *RecoverDesk) Open(actor string, rec Recovery) (Recovery, error) {
	  if d == nil {
		   return Recovery{}, ErrRecoveryRequired
	  }
	   actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return Recovery{}, ErrTicketActor
	  }
	   rec.Kind = strings.TrimSpace(rec.Kind)
	  switch rec.Kind {
	   case KindWrongMemoEvent, KindWrongNetEvent, KindUnattribEvent, string(TicketMemoMiss):
	   default:
		   return Recovery{}, ErrTicketKind
	  }
	  var err error
	   rec.Chain, err = validate.Chain(rec.Chain)
	   if err != nil {
		   return Recovery{}, err
	   }
	   rec.Chain = CanonicalChain(rec.Chain)
	  rec.TxHash,
		   err = validate.TxHash(rec.TxHash)
	   if err != nil {
		  return Recovery{}, err
	   }
	  if strings.TrimSpace(rec.Asset) != "" {
		  rec.Asset, err = validate.Asset(rec.Asset)
		  if err != nil {
			   return Recovery{}, err
		   }
	   }
	   if strings.TrimSpace(rec.UserID) == "" {
		   rec.UserID = "unassigned"
	   } else if rec.UserID != "unassigned" {
		  rec.UserID, err = validate.UserID(rec.UserID)
		  if err != nil {
			  return Recovery{}, err
		  }
	   }
	   rec.Note = strings.TrimSpace(rec.Note)
	   if rec.Note == "" {
		   return Recovery{}, ErrTicketNote
	  }
	   k := slotKey(rec.Chain, rec.TxHash, rec.OutputIndex)
	   d.mu.Lock()
	  defer d.mu.Unlock()
	  if id, ok := d.byOut[k]; ok {
		  if cur := d.rows[id]; cur != nil && cur.State != RecoverRejected {
			   return *cur, nil
		  }
	   }
	   now := d.clock()
	   rec.ID = uuid.NewString()
	   rec.State = RecoverOpen
	   rec.OpenedBy = actor
	   rec.CreatedAt = now
	  rec.UpdatedAt = now
	  cp := rec
	  d.rows[rec.ID] = &cp
	   d.byOut[k] = rec.ID
	   return cp, nil
  }

// Assign 派给另一名处理人。开单人不能派给自己。
   func (d *RecoverDesk) Assign(id, actor, assignee string) (Recovery, error) {
	   if d == nil {
		   return Recovery{}, ErrTicketMiss
	   }
	   actor = strings.TrimSpace(actor)
	   assignee = strings.TrimSpace(assignee)
	  if actor == "" || strings.EqualFold(actor, "system") || assignee == "" || strings.EqualFold(assignee, "system") {
		  return Recovery{}, ErrTicketActor
	   }
	  if strings.EqualFold(actor, assignee) {
		  return Recovery{}, ErrTicketAssign
	   }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   row, ok := d.rows[strings.TrimSpace(id)]
	  if !ok {
		   return Recovery{}, ErrTicketMiss
	  }
	  if strings.EqualFold(row.OpenedBy, actor) {
		   return Recovery{}, ErrTicketAssign
	   }
	  if row.State == RecoverApproved || row.State == RecoverRejected {
		  return *row, ErrTicketClosed
	   }
	   row.State = RecoverAssigned
	  row.Assignee = assignee
	   row.UpdatedAt = d.clock()
	  return *row, nil
  }

// Approve 处理人批准后，才允许专用入账入口。观察线程本身仍不读这张单来静默贷记。

  func (d *RecoverDesk) Approve(id, actor string) (Recovery, error) {
	  if d == nil {
		   return Recovery{}, ErrTicketMiss
	   }
	  actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return Recovery{}, ErrTicketActor
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   row, ok := d.rows[strings.TrimSpace(id)]
	   if !ok {
		   return Recovery{}, ErrTicketMiss
	  }
	  if row.Assignee == "" || strings.EqualFold(row.Assignee, actor) || strings.EqualFold(row.OpenedBy, actor) {
		   return Recovery{},
			  ErrTicketAssign
	   }
	  if row.State == RecoverApproved {
		   return *row, nil
	   }
	   if row.State == RecoverRejected {
		   return *row, ErrTicketClosed
	   }
	  row.State = RecoverApproved
	  row.ApprovedBy = actor
	   row.UpdatedAt = d.clock()
	   return *row, nil
  }

// BlocksObserve 该输出有未批准回收单时，观察线程必须停在工单，不得贷记。

  func (d *RecoverDesk) BlocksObserve(chain, hash string, output int) error {
	  if d == nil {
		  return ErrRecoveryRequired
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	  id, ok := d.byOut[slotKey(chain, hash, output)]
	  if !ok {
		   return nil
	   }
	  row := d.rows[id]
	   if row == nil {
		   return nil
	   }
	   if row.State == RecoverApproved || row.State == RecoverRejected {
		  return nil
	   }
	  return ErrRecoveryRequired
  }

// Approved 批准后的回收单。观察线程不调用本方法。
   func (d *RecoverDesk) Approved(chain, hash string, output int) (Recovery, bool) {
	   if d == nil {
		   return Recovery{}, false
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  id, ok := d.byOut[slotKey(chain, hash, output)]
	  if !ok {
		  return Recovery{}, false
	   }
	  row := d.rows[id]
	  if row == nil || row.State != RecoverApproved {
		   return Recovery{}, false
	  }
	   return *row, true
  }

// List 按用户列回收单。user 空则全部，上限 limit。

  func (d *RecoverDesk) List(user string, limit int) []Recovery {
	   if d == nil {
		  return nil
	   }
	  if limit <= 0 {
		  limit = 50
	   }
	   user = strings.TrimSpace(user)
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  out := make([]Recovery,
		  0, limit)
	  for _, row := range d.rows {
		   if user != "" && row.UserID != user && user != "unassigned" {
			   continue
		   }
		  out = append(out, *row)
		   if len(out) >= limit {
			  break
		  }
	   }
	   return out
   }

// Get 按单号读取回收单。
  func (d *RecoverDesk) Get(id string) (Recovery, error) {
	  if d == nil {
		  return Recovery{}, ErrTicketMiss
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   row, ok := d.rows[strings.TrimSpace(id)]
	   if !ok {
		  return Recovery{}, ErrTicketMiss
	  }
	   return *row, nil
   }
