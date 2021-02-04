package amldesk

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

type Book struct {
	mu sync.Mutex
	rows   map[string]*Record
	byUser map[string][]string
	byMove map[string]string
	ev     map[string][]Evidence
	tr     map[string]Payload
	tl    map[string][]Event
	fence *persist.Table
	clock func() time.Time
}

   func NewBook() *Book {
	  return &Book{
		   rows: map[string]*Record{}, byUser: map[string][]string{}, byMove: map[string]string{},
		  ev: map[string][]Evidence{}, tr: map[string]Payload{}, tl: map[string][]Event{},
		  fence: persist.NewTable(),
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (b *Book) WithClock(c func() time.Time) *Book {
	   if b != nil && c != nil {
		  b.clock = c
	  }
	   return b
   }

  func (b *Book) Open(userID, movementID, kind, reason, actor string) (Record, error) {
	   if b == nil {
		  return Record{}, ErrUnavailable
	   }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		  return Record{}, err
	  }
	  act, err := actorOK(actor)
	  if err != nil {
		   return Record{}, ErrActor
	   }
	  reason, err = validate.Reason(reason)
	  if err != nil {
		  return Record{}, err
	  }
	  k, err := ParseKind(kind)
	   if err != nil {
		   return Record{}, err
	   }
	  if movementID != "" {
		  if _, err := validate.MovementID(movementID); err != nil {
			  return Record{}, err
		  }
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  if movementID != "" {
		  if id, ok := b.byMove[movementID]; ok {
			  if cur := b.rows[id]; cur != nil && cur.Open() {
				  return *cur, nil
			  }
		   }
	   }
	   rec := Record{
		   ID: uuid.NewString(), UserID: uid, MovementID: strings.TrimSpace(movementID),
		   Kind:   k,
		  Reason: reason, Status: StatusOpen, OpenedBy: act, OpenedAt: b.clock(),
	   }
	  f,
		  err := b.fence.Bump(rec.ID, persist.NewToken())
	   if err != nil {
		  return Record{}, err
	   }
	  rec.Fence = f
	   cp := rec
	   b.rows[rec.ID] = &cp
	   b.byUser[uid] = append(b.byUser[uid], rec.ID)
	  if rec.MovementID != "" {
		  b.byMove[rec.MovementID] = rec.ID
	   }
	  b.noteLocked(rec.ID, act, "opened", string(k)+" "+reason)
	  return rec, nil
  }

   func (b *Book) Get(id string) (Record, error) {
	  if b == nil {
		  return Record{}, ErrUnavailable
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  r, ok := b.rows[strings.TrimSpace(id)]
	  if !ok {
		   return Record{}, ErrNotFound
	   }
	   return *r, nil
   }

   func (b *Book) ByMovement(id string) (Record, error) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   cid, ok := b.byMove[strings.TrimSpace(id)]
	  if !ok {
		   return Record{}, ErrNotFound
	  }
	  r := b.rows[cid]
	  if r == nil {
		   return Record{}, ErrNotFound
	   }
	  return *r, nil
  }

  func (b *Book) List(userID string, limit int) []Record {
	  if b == nil {
		  return nil
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if limit <= 0 {
		   limit = 50
	   }
	   ids := b.byUser[strings.TrimSpace(userID)]
	   if userID == "" {
		  out := make([]Record, 0, limit)
		  for _, r := range b.rows {
			  out = append(out, *r)
			   if len(out) >= limit {
				  break
			   }
		   }
		   return out
	  }
	   out := make([]Record, 0, len(ids))
	   for _, id := range ids {
		   if r := b.rows[id]; r != nil {
			   out = append(out, *r)
			  if len(out) >= limit {
				   break
			  }
		  }
	  }
	   return out
  }

  func (b *Book) OpenQueue(limit int) []Record {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	   }
	  out := make([]Record, 0, limit)
	  for _, r := range b.rows {
		  if r.Open() {
			  out = append(out, *r)
			  if len(out) >= limit {
				  break
			   }
		   }
	  }
	   return out
   }

   func (b *Book) mark(id, actor string, to Status, epoch int64, token, note string) (Record, error) {
	   act, err := actorOK(actor)
	   if err != nil {
		  return Record{}, ErrActor
	   }
	  if err := b.fence.Check(id, epoch, token); err != nil {
		  return Record{}, err
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  r, ok := b.rows[id]
	  if !ok {
		  return Record{}, ErrNotFound
	   }
	  if r.Status == StatusClosed || r.Status == StatusRejected {
		  return *r, ErrClosed
	  }
	  f,
		   err := b.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return Record{}, err
	   }
	   r.Status = to
	  r.Note = strings.TrimSpace(note)
	   r.Fence = f
	  if to == StatusClosed || to == StatusRejected {
		  r.ClosedBy = act
		  r.ClosedAt = b.clock()
	  }
	  b.noteLocked(id,
		  act, string(to), note)
	  return *r, nil
   }
