package dualctl

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

type State string

const (
	StateOpen State = "open"
	StateReady State = "ready"
	StateDenied State = "denied"
	StateExpired  State = "expired"
	StateRevoked  State = "revoked"
	StateExecuted State = "executed"
)

type Vote struct {
	Actor string    `json:"actor"`
	Allow bool      `json:"allow"`
	At    time.Time `json:"at"`
}

type Request struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Subject  string `json:"subject"`
	OpenedBy string `json:"opened_by"`
	Need     int    `json:"need"`
	Votes []Vote `json:"votes"`
	State State  `json:"state"`
	Note  string `json:"note,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	Fence     persist.Fence `json:"fence"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

var (
	ErrActor = fmt.Errorf("dual control requires a distinct actor")
	ErrNeed = fmt.Errorf("dual control need must be at least 2")
	ErrNotFound = fmt.Errorf("dual control request not found")
	ErrClosed   = fmt.Errorf("dual control request already closed")
	ErrNotReady = fmt.Errorf("dual control quorum not reached")
	ErrDuplicate = fmt.Errorf("actor already voted")
	ErrStale = persist.ErrStaleFence
)

type Book struct {
	mu  sync.Mutex
	ttl time.Duration
	clock func() time.Time
	rows  map[string]*Request
	fence *persist.Table
	sql   *persist.SQLDB
}

   func New(ttl time.Duration) *Book {
	  if ttl <= 0 {
		   ttl = 4 * time.Hour
	   }
	   return &Book{
		  ttl:   ttl,
		   clock: func() time.Time { return time.Now().UTC() },
		   rows: map[string]*Request{}, fence: persist.NewTable(),
	  }
   }

   func (b *Book) WithClock(c func() time.Time) *Book {
	  if b != nil && c != nil {
		   b.clock = c
	   }
	  return b
  }

  func actorOK(actor string) error {
	  actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return ErrActor
	   }
	   return nil
   }

// Open 开一条 N 票申请。need<2 拒，避免一人闭环。
   func (b *Book) Open(kind, actor, subject string, need int, note string) (*Request, error) {
	  if err := actorOK(actor); err != nil {
		   return nil, err
	  }
	   if need < 2 {
		   return nil, ErrNeed
	  }
	   now := b.clock()
	  r := &Request{
		  ID: uuid.NewString(), Kind: strings.TrimSpace(kind), Subject: strings.TrimSpace(subject),
		  OpenedBy: strings.TrimSpace(actor),
		  Need:     need, State: StateOpen, Note: note,
		  ExpiresAt: now.Add(b.ttl), CreatedAt: now, UpdatedAt: now,
	  }
	  f, err := b.fence.Bump(r.ID, persist.NewToken())
	   if err != nil {
		   return nil, err
	   }
	  r.Fence = f
	   if err := b.persistReq(r); err != nil {
		  return nil, err
	   }
	   b.mu.Lock()
	   b.rows[r.ID] = r
	   b.mu.Unlock()
	   cp := *r
	   return &cp, nil
   }

// Vote 记一票。deny 立刻关单。allow 达到 need 才进 ready。申请人可以投，但不自动算一票。

  func (b *Book) Vote(id, actor string, allow bool, epoch int64, token string) (*Request, error) {
	  if err := actorOK(actor); err != nil {
		   return nil, err
	  }
	  if err := b.fence.Check(id, epoch, token); err != nil {
		  return nil, err
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  r, ok := b.rows[id]
	   if !ok {
		   if loaded, found := b.loadSQL(id); found {
			  b.rows[id] = loaded
			  r = loaded
			   ok = true
		  }
	  }
	   if !ok {
		   return nil, ErrNotFound
	  }
	  if r.State != StateOpen {
		   cp := *r
		   return &cp, ErrClosed
	   }
	   actor = strings.TrimSpace(actor)
	   for _, v := range r.Votes {
		  if strings.EqualFold(v.Actor, actor) {
			   return nil, ErrDuplicate
		   }
	  }
	  now := b.clock()
	  if !now.Before(r.ExpiresAt) {
		   r.State = StateExpired
		  r.UpdatedAt = now
		   cp := *r
		   return &cp, ErrClosed
	  }
	  r.Votes = append(r.Votes, Vote{Actor: actor, Allow: allow, At: now})
	  if !allow {
		  r.State = StateDenied
	  } else if allowCount(r) >= r.Need {
		  r.State = StateReady
	   }
	  f, err := b.fence.Bump(id, persist.NewToken())
	   if err != nil {
		   return nil, err
	   }
	  r.Fence = f
	   r.UpdatedAt = now
	  if err := b.persistReq(r); err != nil {
		  return nil, err
	  }
	  cp := *r
	   return &cp, nil
   }

   func allowCount(r *Request) int {
	   n := 0
	   for _, v := range r.Votes {
		   if v.Allow {
			  n++
		  }
	  }
	  return n
  }

   func (b *Book) Get(id string) (*Request, error) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   r, ok := b.rows[id]
	  if !ok {
		  if loaded, found := b.loadSQL(id); found {
			   b.rows[id] = loaded
			  r = loaded
			  ok = true
		  }
	  }
	   if !ok {
		   return nil, ErrNotFound
	  }
	   cp := *r
	  return &cp, nil
  }

// Execute 必须 ready。执行人不得是唯一投票人（至少再换一个人碰过单）。

   func (b *Book) Execute(id, actor string, epoch int64, token string) (*Request, error) {
	  if err := actorOK(actor); err != nil {
		  return nil, err
	   }
	   if err := b.fence.Check(id, epoch, token); err != nil {
		   return nil, err
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  r, ok := b.rows[id]
	  if !ok {
		   return nil, ErrNotFound
	  }
	   if r.State != StateReady {
		  cp := *r
		  return &cp, ErrNotReady
	  }
	   if onlyVoter(r, actor) {
		  return nil, ErrActor
	   }
	   f, err := b.fence.Bump(id, persist.NewToken())
	   if err != nil {
		  return nil, err
	  }
	  r.State = StateExecuted
	   r.Fence = f
	  r.UpdatedAt = b.clock()
	   if err := b.persistReq(r); err != nil {
		  return nil, err
	  }
	   cp := *r
	   return &cp, nil
  }

   func onlyVoter(r *Request, actor string) bool {
	   if len(r.Votes) != 1 {
		  return false
	   }
	  return strings.EqualFold(r.Votes[0].Actor, actor)
  }

  func (b *Book) Revoke(id, actor string) (*Request, error) {
	   if err := actorOK(actor); err != nil {
		   return nil, err
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  r, ok := b.rows[id]
	   if !ok {
		   return nil, ErrNotFound
	   }
	  if r.State == StateExecuted || r.State == StateDenied || r.State == StateExpired {
		   cp := *r
		   return &cp, ErrClosed
	  }
	  f, err := b.fence.Bump(id, persist.NewToken())
	  if err != nil {
		   return nil, err
	   }
	  r.State = StateRevoked
	   r.Fence = f
	  r.UpdatedAt = b.clock()
	   if err := b.persistReq(r); err != nil {
		  return nil, err
	  }
	  cp := *r
	  return &cp, nil
   }

   func (b *Book) ExpireDue() []Request {
	  now := b.clock()
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Request, 0)
	  for _, r := range b.rows {
		   if r.State != StateOpen && r.State != StateReady {
			  continue
		   }
		   if now.Before(r.ExpiresAt) {
			   continue
		   }
		  r.State = StateExpired
		  r.UpdatedAt = now
		   out = append(out, *r)
	  }
	   return out
  }

   func (b *Book) List(limit int) []Request {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	  }
	   out := make([]Request, 0, limit)
	   for _, r := range b.rows {
		   out = append(out, *r)
		  if len(out) >= limit {
			  break
		  }
	   }
	  return out
   }

  func (b *Book) ReadyCount() int {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   n := 0
	   for _, r := range b.rows {
		   if r.State == StateReady {
			   n++
		   }
	   }
	   return n
   }
