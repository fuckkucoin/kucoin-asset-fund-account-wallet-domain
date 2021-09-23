package marginhook

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/liqgengate"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Book struct {
	mu      sync.Mutex
	holds   map[string]*Hold
	borrows map[string]*Borrow
	byUser map[string][]string
	byIdem map[string]string
}

   func NewBook() *Book {
	  return &Book{
		   holds: map[string]*Hold{}, borrows: map[string]*Borrow{},
		   byUser: map[string][]string{}, byIdem: map[string]string{},
	  }
  }

   func (b *Book) PutHold(h Hold) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   cp := h
	   b.holds[h.ID] = &cp
	   b.byUser[h.UserID] = appendUnique(b.byUser[h.UserID], h.ID)
   }

  func (b *Book) Hold(id string) (Hold, bool) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  h, ok := b.holds[id]
	  if !ok {
		  return Hold{}, false
	   }
	   return *h, true
  }

  func (b *Book) PutBorrow(r Borrow) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   cp := r
	  b.borrows[r.ID] = &cp
	   if r.Idempotency != "" {
		  b.byIdem[r.Idempotency] = r.ID
	   }
   }

  func (b *Book) Borrow(id string) (Borrow, bool) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   r, ok := b.borrows[id]
	   if !ok {
		  return Borrow{}, false
	   }
	  return *r, true
  }

  func (b *Book) ByIdem(key string) (Borrow, bool) {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   id, ok := b.byIdem[key]
	  if !ok {
		   return Borrow{}, false
	  }
	   r := b.borrows[id]
	  if r == nil {
		  return Borrow{}, false
	   }
	  return *r, true
   }

   func (b *Book) UserHolds(userID string) []Hold {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]Hold, 0)
	   for _, id := range b.byUser[userID] {
		  if h := b.holds[id]; h != nil {
			  out = append(out, *h)
		   }
	   }
	   return out
   }

  func appendUnique(ids []string, id string) []string {
	  for _, x := range ids {
		   if x == id {
			   return ids
		   }
	   }
	  return append(ids, id)
   }

type FreezeFunc func(userID string, book account.Book, frozen bool) error

type Service struct {
	book       *Book
	freeze     FreezeFunc
	defaultCap int
	clock      func() time.Time
	liq *liqgengate.Engine
}

   func New() *Service {
	  return &Service{book: NewBook(), defaultCap: 6500, clock: func() time.Time { return time.Now().UTC() }}
  }

  func (s *Service) WithFreeze(fn FreezeFunc) *Service {
	  if s != nil {
		  s.freeze = fn
		  if s.liq != nil {
			   s.liq.WithFreeze(liqgengate.FreezeHook(fn))
		   }
	   }
	  return s
  }

// LiqEngine 只读暴露世代引擎。

   func (s *Service) LiqEngine() *liqgengate.Engine {
	  if s == nil {
		  return nil
	  }
	  return s.liq
   }

   func (s *Service) Book() *Book { return s.book }

type OpenCollateralReq struct {
	UserID string
	Asset string
	Amount string
	CapBPS int
}

// OpenCollateral 记下抵押冻结意图。真正冻可用仍走账户科目闸；这里不改账本余额。
  func (s *Service) OpenCollateral(req OpenCollateralReq) (Hold, error) {
	   if s == nil {
		  return Hold{}, ErrUnavailable
	   }
	   uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return Hold{}, err
	   }
	   asset, err := validate.Asset(req.Asset)
	  if err != nil {
		   return Hold{}, err
	   }
	   amt, err := validate.Amount(req.Amount)
	  if err != nil {
		  return Hold{}, err
	   }
	   if _, err := money.Parse(amt); err != nil {
		  return Hold{}, err
	  }
	  cap := req.CapBPS
	  if cap == 0 {
		  cap = s.defaultCap
	  }
	   if cap <= 0 || cap > 10000 {
		   return Hold{}, ErrLTV
	  }
	   h := Hold{
		  ID: uuid.NewString(), UserID: uid, Kind: KindCollateral,
		   Asset: asset, Amount: amt, CapBPS: cap, Status: StatusOpen, OpenedAt: s.clock(),
	  }
	  s.book.PutHold(h)
	  return h, nil
  }

type BorrowReq struct {
	UserID       string
	CollateralID string
	Asset        string
	Amount       string
	IdempotencyKey string
}

// OpenBorrow 核对 LTV 后再记借款。超 cap 失败即关，不得先记后补抵押。
  func (s *Service) OpenBorrow(req BorrowReq) (Borrow, error) {
	   if s == nil {
		   return Borrow{}, ErrUnavailable
	   }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return Borrow{}, err
	   }
	  key, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		  return Borrow{}, err
	  }
	  if prev, ok := s.book.ByIdem(key); ok {
		   return prev, nil
	   }
	  h,
		  ok := s.book.Hold(strings.TrimSpace(req.CollateralID))
	   if !ok || h.UserID != uid || h.Status != StatusOpen {
		  return Borrow{}, ErrNotFound
	   }
	   asset, err := validate.Asset(req.Asset)
	  if err != nil {
		   return Borrow{}, err
	   }
	   amt, err := validate.Amount(req.Amount)
	   if err != nil {
		   return Borrow{}, err
	  }
	   if err := CheckLTV(amt, h.Amount, h.CapBPS); err != nil {
		   return Borrow{}, err
	   }
	  ltv, err := LTV(amt, h.Amount)
	   if err != nil {
		   return Borrow{}, err
	  }
	  r := Borrow{
		  ID: uuid.NewString(), UserID: uid, Asset: asset, Amount: amt,
		  Collateral: h.ID, LTVBPS: ltv, Status: StatusOpen, OpenedAt: s.clock(), Idempotency: key,
	  }
	  h.BorrowID = r.ID
	  s.book.PutHold(h)
	  s.book.PutBorrow(r)
	   return r, nil
   }

// FreezeForLTV 超阈值时冻结杠杆科目。先推进 liq_gen 再调 freeze hook。
// hook 失败则借款仍在 Frozen/half_open 语义下，调用方不得当已强平释放。
// LTV 是否超 cap 仍走 CheckLTV，本函数不改写公式。
   func (s *Service) FreezeForLTV(userID, borrowID string) (Hold, error) {
	  r, ok := s.book.Borrow(borrowID)
	  if !ok || r.UserID != userID {
		  return Hold{}, ErrNotFound
	   }
	  h, ok := s.book.Hold(r.Collateral)
	   if !ok {
		   return Hold{}, ErrNotFound
	  }
	  if err := CheckLTV(r.Amount, h.Amount, h.CapBPS); err == nil {
		  return Hold{}, fmt.Errorf("ltv still inside cap")
	  }
	   if s.liq != nil {
		   res, err := s.liq.FreezeWithGen(context.Background(), liqgengate.FreezeRequest{
			   UserID: userID, BorrowID: borrowID,
		   })
		   if res != nil && res.HoldID != "" {
			   if fr, ok := s.book.Hold(res.HoldID); ok {
				   if err != nil {
					   return fr, err
				  }
				  return fr, nil
			   }
		   }
		   if err != nil {
			  return Hold{}, err
		   }
	  }
	   fr := Hold{
		   ID: uuid.NewString(), UserID: userID, Kind: KindLiqFreeze,
		   Asset: h.Asset, Amount: h.Amount, Status: StatusFrozen,
		   Reason: "ltv_breach", OpenedAt: s.clock(), BorrowID: r.ID,
	   }
	   s.book.PutHold(fr)
	  r.LiqGen++
	  r.LiqPhase = string(liqgengate.PhaseFreezing)
	  r.Status = StatusFrozen
	   s.book.PutBorrow(r)
	   if s.freeze != nil {
		   if err := s.freeze(userID, account.BookMargin, true); err != nil {
			  r.LiqPhase = string(liqgengate.PhaseHalfOpen)
			   s.book.PutBorrow(r)
			  return fr, err
		   }
	  }
	  r.LiqPhase = string(liqgengate.PhaseFrozen)
	  s.book.PutBorrow(r)
	   return fr, nil
  }

  func (s *Service) Release(holdID string) error {
	  return s.ReleaseWithGen(holdID, 0, false)
  }

// ReleaseWithGen 释放须匹配 liq_gen；缺世代或漂移 fail-closed。
   func (s *Service) ReleaseWithGen(holdID string, gen uint64, hasGen bool) error {
	  h, ok := s.book.Hold(holdID)
	  if !ok {
		   return ErrNotFound
	   }
	   if h.Status == StatusClosed {
		  return ErrClosed
	  }
	   if h.BorrowID != "" {
		  if r, ok := s.book.Borrow(h.BorrowID); ok && r.Status == StatusOpen {
			   return fmt.Errorf("borrow still open")
		   }
		   if s.liq != nil && h.BorrowID != "" {
			  res, err := s.liq.ReleaseWithGen(context.Background(), liqgengate.ReleaseRequest{
				   UserID: h.UserID, BorrowID: h.BorrowID, HoldID: holdID, LiqGen: gen, HasGen: hasGen,
			   })
			  if err != nil {
				  if res != nil && res.RejectReason != "" {
					   return fmt.Errorf("%w: %s", ErrLiqGen, res.RejectReason)
				  }
				   return err
			  }
			   return nil
		  }
	   }
	   h.Status = StatusReleased
	  s.book.PutHold(h)
	   return nil
   }

  func (s *Service) CloseBorrow(id string) error {
	   return s.CloseBorrowWithGen(id, 0, false)
   }

// CloseBorrowWithGen 关借须匹配 liq_gen。

  func (s *Service) CloseBorrowWithGen(id string, gen uint64, hasGen bool) error {
	   r, ok := s.book.Borrow(id)
	  if !ok {
		   return ErrNotFound
	  }
	  if s.liq != nil {
		   res, err := s.liq.CloseWithGen(context.Background(), liqgengate.CloseRequest{
			   UserID:   r.UserID,
			   BorrowID: id, LiqGen: gen, HasGen: hasGen,
		  })
		  if err != nil {
			   if res != nil && res.RejectReason != "" {
				  return fmt.Errorf("%w: %s", ErrLiqGen, res.RejectReason)
			   }
			   return err
		  }
		  return nil
	  }
	   r.Status = StatusClosed
	   r.LiqPhase = string(liqgengate.PhaseClosed)
	  s.book.PutBorrow(r)
	  return nil
   }

  func (s *Service) Holds(userID string) []Hold { return s.book.UserHolds(userID) }

  func (b *Book) Users() []string {
	   if b == nil {
		   return nil
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]string,
		   0, len(b.byUser))
	   for uid := range b.byUser {
		  out = append(out, uid)
	   }
	  return out
   }

   func (b *Book) OpenBorrows() []Borrow {
	   if b == nil {
		   return nil
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]Borrow, 0)
	  for _, r := range b.borrows {
		  if r != nil && r.Status == StatusOpen {
			  out = append(out, *r)
		  }
	  }
	   return out
  }
