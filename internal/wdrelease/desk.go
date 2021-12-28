package wdrelease

import (
	"encoding/json"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/approvestore"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// Desk 出金多级放行台。与托管归集审批不是同一本账。
type Desk struct {
	mu       sync.Mutex
	clock    func() time.Time
	book     *PolicyBook
	activeID string
	pinned   bool
	tickets map[string]*Ticket
	byIdem  map[string]string
	grants map[string]*Grant
	audits map[string][]Audit
	holds   *holdBook
	keyUser map[string]string
	keyDest map[string]string
	priv    map[string]int64
	privSent map[string]int64
	driftSet map[string]bool
	driftNew int
	kv       persist.KV
	sql      *persist.SQLDB
	refs *approvestore.Store
	halted error
	seenAsset  map[string]struct{}
	idemFence  map[string]persist.Fence
	grantFence map[string]persist.Fence
	auditFence map[string]persist.Fence
	hydrated bool
}

  func New() *Desk {
	   book, err := newBook(builtinVersions())
	  if err != nil {
		  panic(err)
	  }
	   d := &Desk{
		  clock:   func() time.Time { return time.Now().UTC() },
		  book:    book,
		  tickets: map[string]*Ticket{},
		  byIdem:  map[string]string{},
		  grants:  map[string]*Grant{},
		  audits: map[string][]Audit{},
		   holds:   newHoldBook(),
		   keyUser: map[string]string{},
		   keyDest: map[string]string{},
		   priv:     map[string]int64{},
		  privSent: map[string]int64{},
		   driftSet:   map[string]bool{},
		  seenAsset:  map[string]struct{}{},
		  idemFence:  map[string]persist.Fence{},
		  grantFence: map[string]persist.Fence{},
		  auditFence: map[string]persist.Fence{},
	  }
	   d.activeID = book.EffectiveID(d.clock())
	  return d
   }

   func (d *Desk) BindKV(kv persist.KV) *Desk {
	   if d == nil || kv == nil {
		  return d
	  }
	   d.mu.Lock()
	  d.kv = kv
	  d.mu.Unlock()
	  return d
  }

   func (d *Desk) BindSQL(db *persist.SQLDB) *Desk {
	  if d == nil || db == nil {
		   return d
	  }
	  d.mu.Lock()
	  d.sql = db
	   d.mu.Unlock()
	   return d
  }

   func (d *Desk) BindRefs(st *approvestore.Store) *Desk {
	  if d == nil || st == nil {
		   return d
	   }
	  d.mu.Lock()
	  d.refs = st
	   d.mu.Unlock()
	   return d
  }

   func (d *Desk) now() time.Time {
	  if d != nil && d.clock != nil {
		   return d.clock().UTC()
	  }
	   return time.Now().UTC()
   }

// SetNow 固定时钟，并在未手工发布时按生效日重选代次。

  func (d *Desk) SetNow(t time.Time) {
	   if d == nil {
		  return
	  }
	   t = t.UTC()
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  d.clock = func() time.Time { return t }
	   if !d.pinned && d.book != nil {
		   d.activeID = d.book.EffectiveID(t)
	  }
  }

  func (d *Desk) ActiveVersion() string {
	  if d == nil {
		  return ""
	   }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   return d.activeID
   }

// Publish 把已安装的代次设为当前代次。已开票据保持原代次，只记漂移。

  func (d *Desk) Publish(id string) error {
	   if d == nil {
		  return ErrUnavailable
	   }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   if _, ok := d.book.Get(id); !ok {
		  return WithCode(ErrVersion, CodeVersion)
	   }
	  d.activeID = id
	   d.pinned = true
	  d.scanDrift()
	  d.emitLocked()
	   return nil
  }

// Install 装入一张额外的代次表，供夹具覆盖单段上限。不自动切到该代次。

  func (d *Desk) Install(v Version) error {
	   if d == nil {
		   return ErrUnavailable
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  return d.book.install(v)
   }

  func (d *Desk) Version(id string) (Version, error) {
	   if d == nil || d.book == nil {
		   return Version{}, ErrUnavailable
	   }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   if id == "" {
		  id = d.activeID
	   }
	  v, ok := d.book.Get(id)
	   if !ok {
		   return Version{}, WithCode(ErrVersion, CodeVersion)
	  }
	  return v, nil
  }

  func (d *Desk) Get(id string) (Ticket, error) {
	   if d == nil {
		   return Ticket{}, ErrUnavailable
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   t := d.tickets[id]
	   if t == nil {
		  return Ticket{}, WithCode(ErrNoTicket, CodeNoTicket)
	   }
	   return cloneTicket(t), nil
   }

  func (d *Desk) Audits(id string) ([]Audit, error) {
	  if d == nil {
		  return nil, ErrUnavailable
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   rows := d.audits[id]
	   if rows == nil {
		  return nil, WithCode(ErrNoTicket, CodeNoTicket)
	  }
	  out := append([]Audit(nil),
		   rows...)
	   return out, nil
   }

   func cloneTicket(t *Ticket) Ticket {
	   if t == nil {
		  return Ticket{}
	  }
	   b, err := json.Marshal(t)
	  if err != nil {
		   return Ticket{}
	  }
	  var out Ticket
	   if err := json.Unmarshal(b, &out); err != nil {
		   return Ticket{}
	  }
	  return out
   }

  func (d *Desk) freshID() string {
	  return uuid.NewString()
  }
