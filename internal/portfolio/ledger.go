package portfolio

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// BookLedger 科目子账。funding/trading/earn/margin/futures 分开记账，合计才对用户。

type BookLedger struct {
	mu      sync.Mutex
	rows    map[string]*Position
	journal map[string]*HopRec
	fence   *persist.Table
	clock   func() time.Time
}

   func NewBookLedger() *BookLedger {
	   return &BookLedger{
		  rows: map[string]*Position{}, journal: map[string]*HopRec{},
		  fence: persist.NewTable(),
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (l *BookLedger) WithClock(c func() time.Time) *BookLedger {
	   if l != nil && c != nil {
		   l.clock = c
	  }
	   return l
   }

   func (l *BookLedger) Get(userID string, book Book, asset string) (Position, bool) {
	   if l == nil {
		  return Position{}, false
	   }
	  l.mu.Lock()
	  defer l.mu.Unlock()
	  p, ok := l.rows[bookKey(userID, book, asset)]
	   if !ok {
		  return Position{}, false
	  }
	  return *p, true
   }

  func (l *BookLedger) MustGet(userID string, book Book, asset string) Position {
	   p,
		  ok := l.Get(userID, book, asset)
	   if ok {
		  return p
	   }
	  return Position{UserID: userID, Book: book, Asset: strings.ToUpper(asset), Available: "0", Frozen: "0"}
  }

  func (l *BookLedger) List(userID string) []Position {
	   if l == nil {
		  return nil
	   }
	   l.mu.Lock()
	   defer l.mu.Unlock()
	  out := make([]Position, 0)
	  for _, p := range l.rows {
		   if userID != "" && p.UserID != userID {
			   continue
		   }
		   out = append(out, *p)
	  }
	   return out
  }

// Seed 写入已落库余额。金额走 ParseBook，允许 0。

   func (l *BookLedger) Seed(userID, book, asset, avail, frozen string) (Position, error) {
	   if l == nil {
		   return Position{}, ErrUnavailable
	  }
	   p, err := parsePos(userID, book, asset, avail, frozen)
	   if err != nil {
		  return Position{}, err
	  }
	  p.UpdatedAt = l.clock()
	   l.mu.Lock()
	  cp := p
	  l.rows[bookKey(p.UserID, p.Book, p.Asset)] = &cp
	   l.mu.Unlock()
	  return p, nil
  }

  func (l *BookLedger) Credit(userID string, book Book, asset, amount string) (Position, error) {
	   return l.mutate(userID, book, asset, amount, true, false)
   }

  func (l *BookLedger) Debit(userID string, book Book, asset, amount string) (Position, error) {
	   return l.mutate(userID,
		  book, asset, amount, false, false)
  }

   func (l *BookLedger) Freeze(userID string, book Book, asset, amount string) (Position, error) {
	   return l.mutate(userID,
		   book, asset, amount, false, true)
   }

   func (l *BookLedger) Unfreeze(userID string, book Book, asset, amount string) (Position, error) {
	  if l == nil {
		   return Position{}, ErrUnavailable
	   }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return Position{}, ErrUser
	  }
	  as, err := validate.Asset(asset)
	  if err != nil {
		   return Position{}, err
	   }
	  amt, err := money.Parse(amount)
	  if err != nil {
		   return Position{}, err
	   }
	   l.mu.Lock()
	   defer l.mu.Unlock()
	   k := bookKey(uid, book, as)
	  p := l.rows[k]
	   if p == nil {
		   return Position{}, ErrNotFound
	   }
	   fr,
		  err := money.ParseBook(orZ(p.Frozen))
	  if err != nil {
		  return Position{}, err
	   }
	  av, err := money.ParseBook(orZ(p.Available))
	  if err != nil {
		   return Position{}, err
	  }
	   nfr, err := fr.Sub(amt)
	  if err != nil {
		  return Position{}, err
	  }
	   nav, err := av.Add(amt)
	   if err != nil {
		   return Position{}, err
	   }
	  p.Frozen = nfr.String()
	  p.Available = nav.String()
	   p.UpdatedAt = l.clock()
	   return *p, nil
  }

  func (l *BookLedger) mutate(userID string, book Book, asset, amount string, credit, freeze bool) (Position, error) {
	  if l == nil {
		   return Position{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return Position{}, ErrUser
	  }
	  if _, err := ParseBook(string(book)); err != nil {
		  return Position{}, err
	   }
	   as, err := validate.Asset(asset)
	  if err != nil {
		   return Position{}, err
	  }
	  amt, err := money.Parse(amount)
	   if err != nil {
		   return Position{}, err
	  }
	   l.mu.Lock()
	  defer l.mu.Unlock()
	   return l.mutateLocked(uid, book, as, amt, credit, freeze)
  }

   func (l *BookLedger) mutateLocked(uid string, book Book, as string, amt money.Amount, credit, freeze bool) (Position, error) {
	   k := bookKey(uid, book, as)
	  p := l.rows[k]
	   if p == nil {
		   p = &Position{UserID: uid, Book: book, Asset: as, Available: "0", Frozen: "0"}
		  l.rows[k] = p
	  }
	   if p.Locked && !credit {
		  return *p, ErrLocked
	   }
	   if p.Reserved && !credit && !freeze {
		   return *p, ErrReserved
	   }
	  av,
		  err := money.ParseBook(orZ(p.Available))
	  if err != nil {
		  return Position{}, err
	   }
	  fr, err := money.ParseBook(orZ(p.Frozen))
	  if err != nil {
		   return Position{}, err
	  }
	   switch {
	   case freeze:
		   nav, err := av.Sub(amt)
		   if err != nil {
			  return Position{}, err
		   }
		   nfr, err := fr.Add(amt)
		  if err != nil {
			   return Position{}, err
		  }
		   p.Available = nav.String()
		   p.Frozen = nfr.String()
	   case credit:
		  nav, err := av.Add(amt)
		  if err != nil {
			  return Position{}, err
		  }
		   p.Available = nav.String()
	   default:
		   nav, err := av.Sub(amt)
		  if err != nil {
			   return Position{}, err
		  }
		  p.Available = nav.String()
	  }
	   p.UpdatedAt = l.clock()
	   return *p, nil
   }

   func (l *BookLedger) SetLock(userID string, book Book, asset string, locked bool) error {
	  return l.flag(userID, book, asset, locked, false)
  }

   func (l *BookLedger) SetReserve(userID string, book Book, asset string, reserved bool) error {
	   return l.flag(userID, book, asset, false, reserved)
  }

   func (l *BookLedger) flag(userID string, book Book, asset string, locked, reserveSet bool) error {
	   if l == nil {
		  return ErrUnavailable
	   }
	  l.mu.Lock()
	   defer l.mu.Unlock()
	  k := bookKey(userID, book, asset)
	  p := l.rows[k]
	   if p == nil {
		  return ErrNotFound
	  }
	   if reserveSet {
		  p.Reserved = true
	   } else if locked {
		  p.Locked = true
	   } else {
		  p.Locked = false
		   p.Reserved = false
	  }
	  p.UpdatedAt = l.clock()
	   return nil
   }

  func (l *BookLedger) TotalAsset(userID, asset string) (money.Amount, error) {
	   sum := money.Zero()
	   for _, p := range l.List(userID) {
		   if !strings.EqualFold(p.Asset, asset) {
			   continue
		   }
		   t, err := p.Total()
		  if err != nil {
			   return money.Zero(), err
		  }
		  next, err := sum.Add(t)
		  if err != nil {
			   return money.Zero(), err
		   }
		  sum = next
	   }
	   return sum, nil
  }

  func (l *BookLedger) Snapshot(userID string) (Snap, error) {
	  rows := l.List(userID)
	  out := Snap{UserID: userID,
		   Positions: rows, At: time.Now().UTC()}
	   by := map[string]money.Amount{}
	  for _, p := range rows {
		   t, err := p.Total()
		  if err != nil {
			  return Snap{}, err
		  }
		   cur := by[p.Asset]
		  next, err := cur.Add(t)
		   if err != nil {
			   return Snap{}, err
		  }
		  by[p.Asset] = next
	  }
	  out.Totals = make([]AssetTotal, 0, len(by))
	  for asset, amt := range by {
		   out.Totals = append(out.Totals, AssetTotal{Asset: asset, Amount: amt.String()})
	   }
	   return out, nil
   }

type Snap struct {
	UserID string `json:"user_id"`
	Positions []Position `json:"positions"`
	Totals []AssetTotal `json:"totals"`
	At     time.Time    `json:"at"`
}

type AssetTotal struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

  func ConserveSnaps(before, after Snap, asset string) error {
	  var l, r money.Amount
	  var err error
	  for _, t := range before.Totals {
		   if t.Asset == asset {
			  l, err = money.ParseBook(orZ(t.Amount))
			  if err != nil {
				   return err
			  }
		  }
	  }
	   for _, t := range after.Totals {
		   if t.Asset == asset {
			   r, err = money.ParseBook(orZ(t.Amount))
			   if err != nil {
				  return err
			  }
		   }
	   }
	  if err := money.Conserve(l, r); err != nil {
		  return fmt.Errorf("%w: %v", ErrConserve, err)
	  }
	   return nil
   }
