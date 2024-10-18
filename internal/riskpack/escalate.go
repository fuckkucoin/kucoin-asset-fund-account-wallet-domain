package riskpack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

type Status string

const (
	StatusQueued Status = "queued"
	StatusAssigned Status = "assigned"
	StatusApproved Status = "approved"
	StatusDenied Status = "denied"
	StatusExpired Status = "expired"
)

type Item struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Reason string `json:"reason"`
	Asset  string `json:"asset,omitempty"`
	Amount string `json:"amount,omitempty"`
	Dest   string `json:"dest,omitempty"`
	Status Status `json:"status"`
	Actor    string    `json:"actor,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	Fence persist.Fence `json:"fence"`
}

type Queue struct {
	mu sync.Mutex
	ttl   time.Duration
	clock func() time.Time
	rows  map[string]*Item
	fence *persist.Table
	sql *persist.SQLDB
}

   func NewQueue(ttl time.Duration) *Queue {
	   if ttl <= 0 {
		  ttl = 4 * time.Hour
	   }
	  return &Queue{
		  ttl:   ttl,
		   clock: func() time.Time { return time.Now().UTC() },
		  rows: map[string]*Item{}, fence: persist.NewTable(),
	  }
  }

   func (q *Queue) WithClock(c func() time.Time) *Queue {
	   if q != nil && c != nil {
		   q.clock = c
	  }
	   return q
  }

  func (q *Queue) Enqueue(userID, reason, asset, amount, dest string) (Item, error) {
	  if q == nil {
		   return Item{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return Item{}, err
	  }
	  reason, err = validate.Reason(reason)
	   if err != nil {
		  return Item{}, err
	   }
	  it := Item{
		   ID: uuid.NewString(), UserID: uid, Reason: reason, Asset: asset,
		   Amount: amount,
		  Dest: dest, Status: StatusQueued, OpenedAt: q.clock(),
	  }
	   snap := q.fence.Snapshot()
	   f, err := q.fence.Bump(it.ID, persist.NewToken())
	   if err != nil {
		  return Item{}, err
	   }
	  it.Fence = f
	   q.mu.Lock()
	   q.rows[it.ID] = &it
	   q.mu.Unlock()
	   if err := q.persistItem(it); err != nil {
		  q.mu.Lock()
		  delete(q.rows, it.ID)
		   q.mu.Unlock()
		   q.fence.Restore(snap)
		  return Item{}, err
	   }
	  return it, nil
  }

   func (q *Queue) Assign(id, actor string, epoch int64, token string) (Item, error) {
	   return q.transit(id, actor, epoch, token, StatusAssigned)
   }

  func (q *Queue) Approve(id, actor string, epoch int64, token string) (Item, error) {
	   return q.transit(id,
		  actor, epoch, token, StatusApproved)
  }

   func (q *Queue) Deny(id, actor string, epoch int64, token string) (Item, error) {
	   return q.transit(id, actor, epoch, token, StatusDenied)
   }

  func (q *Queue) transit(id, actor string, epoch int64, token string, to Status) (Item, error) {
	   if _, err := validate.Actor(actor); err != nil {
		   return Item{}, ErrActor
	   }
	   if err := q.fence.Check(id, epoch, token); err != nil {
		  return Item{}, err
	   }
	  q.mu.Lock()
	   it, ok := q.rows[id]
	  if !ok {
		  q.mu.Unlock()
		  return Item{}, ErrNotFound
	  }
	   if it.Status == StatusApproved || it.Status == StatusDenied || it.Status == StatusExpired {
		  out := *it
		   q.mu.Unlock()
		  return out, ErrClosed
	   }
	  if to == StatusAssigned && it.Status != StatusQueued {
		   out := *it
		   q.mu.Unlock()
		   return out, ErrClosed
	  }
	   if (to == StatusApproved || to == StatusDenied) && it.Status != StatusQueued && it.Status != StatusAssigned {
		   out := *it
		   q.mu.Unlock()
		   return out, ErrClosed
	  }
	   prevStatus, prevActor, prevFence := it.Status, it.Actor, it.Fence
	   snap := q.fence.Snapshot()
	   f,
		  err := q.fence.Bump(id, persist.NewToken())
	  if err != nil {
		  q.mu.Unlock()
		  return Item{}, err
	   }
	   it.Status = to
	   it.Actor = strings.TrimSpace(actor)
	   it.Fence = f
	   out := *it
	  q.mu.Unlock()
	   if err := q.persistItem(out); err != nil {
		   q.mu.Lock()
		   it.Status = prevStatus
		   it.Actor = prevActor
		  it.Fence = prevFence
		  q.mu.Unlock()
		   q.fence.Restore(snap)
		   return Item{}, err
	   }
	   return out, nil
   }

   func (q *Queue) Get(id string) (Item, error) {
	   if q != nil && q.sqlOn() {
		  row,
			   err := sqlrepo.NewRiskPack(q.sql).GetItem(context.Background(), id)
		  if err == nil {
			  return Item{
				   ID: row.ID,
				  UserID: row.UserID, Reason: row.Reason, Asset: row.Asset, Amount: row.Amount,
				   Dest: row.Dest, Status: Status(row.Status), Actor: row.Actor, OpenedAt: row.OpenedAt, Fence: row.Fence,
			  }, nil
		   }
		  if !errors.Is(err, sqlrepo.ErrNotFound) {
			  return Item{}, ErrUnavailable
		   }
	  }
	  q.mu.Lock()
	   defer q.mu.Unlock()
	   it, ok := q.rows[id]
	   if !ok {
		  return Item{}, ErrNotFound
	  }
	   return *it, nil
   }

   func (q *Queue) Pending(limit int) []Item {
	   q.Expire()
	  if q != nil && q.sqlOn() {
		   got, err := sqlrepo.NewRiskPack(q.sql).ListPending(context.Background(), limit)
		   if err == nil {
			  if limit <= 0 {
				   limit = 50
			  }
			   out := make([]Item, 0, len(got))
			  for _, row := range got {
				  out = append(out, Item{
					  ID:     row.ID,
					  UserID: row.UserID, Reason: row.Reason, Asset: row.Asset, Amount: row.Amount,
					   Dest: row.Dest, Status: Status(row.Status), Actor: row.Actor, OpenedAt: row.OpenedAt, Fence: row.Fence,
				  })
				  if len(out) >= limit {
					   break
				   }
			   }
			   return out
		   }
	   }
	  q.mu.Lock()
	   defer q.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	  }
	  out := make([]Item, 0, limit)
	  for _, it := range q.rows {
		   if it.Status != StatusQueued && it.Status != StatusAssigned {
			  continue
		   }
		   out = append(out, *it)
		   if len(out) >= limit {
			  break
		  }
	   }
	  return out
   }

// Expire 超时只关单，不得当放行。
  func (q *Queue) Expire() int {
	  if q == nil {
		   return 0
	  }
	   now := q.clock()
	   q.mu.Lock()
	  defer q.mu.Unlock()
	  n := 0
	   for _, it := range q.rows {
		  if it.Status != StatusQueued && it.Status != StatusAssigned {
			   continue
		  }
		   if now.Sub(it.OpenedAt) < q.ttl {
			   continue
		  }
		   it.Status = StatusExpired
		   n++
		  _ = q.persistItem(*it)
	   }
	  return n
   }

   func (q *Queue) IsApproved(id string) bool {
	   it, err := q.Get(id)
	  return err == nil && it.Status == StatusApproved
   }

   func (q *Queue) String() string {
	  if q == nil {
		   return "escalate<nil>"
	   }
	   return fmt.Sprintf("escalate pending=%d", len(q.Pending(200)))
   }
