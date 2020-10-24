// 账户账本领域服务实现。

// Author: Knox
// Date: 2020.09.21

package account

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/command"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/shopspring/decimal"
)

   func timeNowUnix() int64 { return time.Now().UTC().Unix() }

type Service struct {
	led *ledger.Service
	cmds       *command.Log
	classifier ShardClassifier
	mu sync.Mutex
	life     map[string]Status
	reason   map[string]string
	events   []Event
	lifeGate LifeGate
	tenantOf map[string]string
	sql      *persist.SQLDB
}

   func New(led *ledger.Service) *Service {
	   return &Service{led: led, cmds: command.NewLog(), life: map[string]Status{}, reason: map[string]string{}}
  }

// BindShardClassifier 挂上分片分类。未挂上时划转仍走原单分片路径。

   func (s *Service) BindShardClassifier(c ShardClassifier) *Service {
	   if s != nil {
		  s.classifier = c
	  }
	  return s
  }

   func lifeKey(userID string, book Book) string {
	   return strings.TrimSpace(userID) + "|" + string(book)
  }

  func (s *Service) StatusOf(userID string, book Book) Status {
	  return s.statusOf(userID, book)
   }

  func (s *Service) statusOf(userID string, book Book) Status {
	  key := lifeKey(userID, book)
	  s.mu.Lock()
	  st, ok := s.life[key]
	  s.mu.Unlock()
	  if ok {
		  return st
	  }
	  if loaded, reason, found := s.loadLife(userID, book); found {
		  s.mu.Lock()
		  s.life[key] = loaded
		  if reason != "" {
			  s.reason[key] = reason
		  }
		  s.mu.Unlock()
		  return loaded
	  }
	  // funding 默认已开；其它科目要先 open。库里没有记录时才用这条规则。
	  if book == BookFunding {
		  return StatusActive
	  }
	  return StatusClosed
  }

   func (s *Service) setStatus(userID string, book Book, st Status) error {
	  return s.setStatusReason(userID, book, st, "", "")
  }

  func (s *Service) setStatusReason(userID string, book Book, st Status, actor, reason string) error {
	  key := lifeKey(userID, book)
	  s.mu.Lock()
	  prev, had := s.life[key]
	  prevReason, hadReason := s.reason[key]
	  s.life[key] = st
	  if reason != "" {
		  s.reason[key] = reason
	  }
	  if st != StatusFrozen {
		  delete(s.reason, key)
	  }
	  s.mu.Unlock()
	  if err := s.persistLife(userID, book, st, reason); err != nil {
		  s.mu.Lock()
		  if had {
			  s.life[key] = prev
		  } else {
			  delete(s.life, key)
		  }
		  if hadReason {
			  s.reason[key] = prevReason
		  } else {
			  delete(s.reason, key)
		  }
		  s.mu.Unlock()
		  return err
	  }
	  s.mu.Lock()
	  s.events = append(s.events, Event{
		  UserID: userID, Book: book, Action: string(st), Actor: actor, Reason: reason, At: timeNowUnix(),
	  })
	  s.mu.Unlock()
	  return nil
  }

   func (s *Service) Open(ctx context.Context, userID string, book Book) (BookState, error) {
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return BookState{}, fmt.Errorf("user_id required")
	   }
	  if err := s.admitTenant(userID, TenantFrom(ctx)); err != nil {
		  return BookState{}, err
	  }
	  cur := s.statusOf(userID, book)
	  if cur == StatusFrozen {
		  return BookState{}, fmt.Errorf("book %s is frozen", book)
	  }
	   if err := s.setStatus(userID, book, StatusActive); err != nil {
		   return BookState{}, err
	   }
	   return BookState{UserID: userID, Book: book, Status: StatusActive}, nil
  }

   func (s *Service) OpenAll(ctx context.Context, userID string) ([]BookState, error) {
	   out := make([]BookState, 0, len(AllBooks()))
	   for _, b := range AllBooks() {
		   st, err := s.Open(ctx, userID, b)
		  if err != nil {
			  return out, err
		  }
		   out = append(out, st)
	  }
	  return out, nil
   }

  func (s *Service) Freeze(ctx context.Context, userID string, book Book, frozen bool) (BookState, error) {
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return BookState{}, fmt.Errorf("user_id required")
	  }
	  if err := s.admitTenant(userID, TenantFrom(ctx)); err != nil {
		  return BookState{}, err
	  }
	  cur := s.statusOf(userID, book)
	  if frozen {
		  if cur == StatusClosed {
			   return BookState{}, fmt.Errorf("cannot freeze closed book %s", book)
		   }
		  if err := s.setStatusReason(userID, book, StatusFrozen, "", "freeze"); err != nil {
			  return BookState{}, err
		  }
		   return BookState{UserID: userID, Book: book, Status: StatusFrozen, Reason: "freeze"}, nil
	  }
	   if cur != StatusFrozen {
		   return BookState{UserID: userID, Book: book, Status: cur}, nil
	  }
	  if err := s.setStatus(userID, book, StatusActive); err != nil {
		  return BookState{}, err
	  }
	   return BookState{UserID: userID, Book: book, Status: StatusActive}, nil
   }

  func (s *Service) Close(ctx context.Context, userID string, book Book) (BookState, error) {
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return BookState{}, fmt.Errorf("user_id required")
	  }
	  if err := s.admitTenant(userID, TenantFrom(ctx)); err != nil {
		  return BookState{}, err
	  }
	  if book == BookFunding {
		   return BookState{}, fmt.Errorf("funding book cannot be closed")
	  }
	   if s.led != nil {
		   books, err := s.led.Accounts(ctx, userID)
		  if err != nil {
			  return BookState{}, err
		   }
		  want := string(book.AccountType())
		  for _, b := range books {
			   if b.Book != want && b.Subject != string(book) {
				  continue
			  }
			   avail, err := decimal.NewFromString(b.Available)
			  if err != nil {
				  return BookState{}, err
			   }
			  fr, err := decimal.NewFromString(b.Frozen)
			   if err != nil {
				  return BookState{}, err
			  }
			   if !avail.IsZero() || !fr.IsZero() {
				  return BookState{}, fmt.Errorf("book %s still holds %s available / %s frozen", book, b.Available, b.Frozen)
			  }
		   }
	  }
	  if err := s.setStatusReason(userID, book, StatusClosed, "", "close"); err != nil {
		  return BookState{}, err
	  }
	   return BookState{UserID: userID,
		   Book: book, Status: StatusClosed}, nil
   }

  func (s *Service) Events(userID string, limit int) []Event {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	   }
	  out := make([]Event, 0, limit)
	  for i := len(s.events) - 1; i >= 0 && len(out) < limit; i-- {
		   if userID != "" && s.events[i].UserID != userID {
			   continue
		  }
		   out = append(out, s.events[i])
	  }
	  return out
   }

type Snapshot struct {
	UserID string        `json:"user_id"`
	Books  []BookWithBal `json:"books"`
}

type BookWithBal struct {
	Book Book `json:"book"`
	Status Status `json:"status"`
	Asset string `json:"asset,omitempty"`
	Available string `json:"available,omitempty"`
	Frozen    string `json:"frozen,omitempty"`
}

   func (s *Service) List(ctx context.Context, userID string) (Snapshot, error) {
	  userID = strings.TrimSpace(userID)
	  out := Snapshot{UserID: userID,
		  Books: nil}
	  byBook := map[Book][]ledger.BookView{}
	  if s.led != nil {
		   rows, err := s.led.Accounts(ctx, userID)
		   if err != nil {
			   return Snapshot{}, err
		   }
		  for _, row := range rows {
			   b, err := ParseBook(row.Subject)
			  if err != nil {
				   b, err = ParseBook(row.Book)
				  if err != nil {
					   continue
				   }
			   }
			  byBook[b] = append(byBook[b], row)
		   }
	  }
	  for _, book := range AllBooks() {
		   st := s.statusOf(userID, book)
		   rows := byBook[book]
		   if len(rows) == 0 {
			  out.Books = append(out.Books, BookWithBal{Book: book, Status: st})
			   continue
		   }
		  byAsset := map[string]BookWithBal{}
		   assets := make([]string, 0, len(rows))
		   for _, row := range rows {
			  cur, ok := byAsset[row.Asset]
			   if !ok {
				  byAsset[row.Asset] = BookWithBal{
					  Book: book, Status: st, Asset: row.Asset,
					  Available: row.Available, Frozen: row.Frozen,
				   }
				   assets = append(assets, row.Asset)
				  continue
			  }
			  cur.Available = addBookAmt(cur.Available, row.Available)
			  cur.Frozen = addBookAmt(cur.Frozen, row.Frozen)
			  byAsset[row.Asset] = cur
		   }
		   for _, asset := range assets {
			  out.Books = append(out.Books, byAsset[asset])
		  }
	   }
	   return out, nil
  }

type MoveReq struct {
	UserID string
	From   Book
	To     Book
	Asset  string
	Amount decimal.Decimal
	IdempotencyKey string
}

   func (s *Service) Move(ctx context.Context, req MoveReq) (*ledger.Transfer, error) {
	  if s == nil || s.led == nil {
		   return nil, fmt.Errorf("account ledger unavailable")
	   }
	  if err := s.admitTenant(req.UserID, TenantFrom(ctx)); err != nil {
		  return nil, err
	  }
	  if req.From == req.To {
		   return nil, fmt.Errorf("from_book and to_book must differ")
	   }
	   src := s.statusOf(req.UserID, req.From)
	  dst := s.statusOf(req.UserID, req.To)
	   if !src.CanDebit() {
		  return nil, fmt.Errorf("source book %s is %s", req.From, src)
	   }
	  if !dst.CanCredit() {
		   return nil, fmt.Errorf("dest book %s is %s", req.To, dst)
	   }
	  dir, err := MoveDirection(req.From, req.To)
	   if err != nil {
		  return nil, err
	   }
	  if s.classifier != nil {
		  cl,
			   err := s.classifier.ClassifyHop(ctx, req.UserID, string(req.From), string(req.To), req.Asset)
		  if err != nil {
			  return nil, err
		   }
		   if !cl.SameShard {
			  return nil, ErrCrossShard
		  }
	   }
	  payload := command.HashPayload("book_move", req.UserID, string(req.From), string(req.To), req.Asset, req.Amount.String())
	   rec, existed, err := s.cmds.Begin(req.IdempotencyKey, "book_move", payload)
	  if err != nil {
		   return nil, err
	  }
	  if existed && rec.Status == command.StatusSucceeded && rec.ResultID != "" {
		  return s.led.Get(ctx,
			  rec.ResultID)
	  }
	   tr, err := s.led.Create(ctx, ledger.CreateReq{
		  UserID:    req.UserID,
		  Direction: dir, Asset: req.Asset, Amount: req.Amount,
		   IdempotencyKey: req.IdempotencyKey,
	   })
	   if err != nil {
		   s.cmds.Fail(req.IdempotencyKey, err)
		  return tr, err
	   }
	   s.cmds.Succeed(req.IdempotencyKey, tr.ID)
	   return tr, nil
  }

// CompensateMove 在对向科目回跳，用于 hop 在 Move 之后绑意图失败时的补偿。
  func (s *Service) CompensateMove(ctx context.Context, req MoveReq) (*ledger.Transfer, error) {
	  idem := strings.TrimSpace(req.IdempotencyKey)
	  if idem == "" {
		   return nil, fmt.Errorf("compensate idempotency_key required")
	  }
	  return s.Move(ctx, MoveReq{
		   UserID: req.UserID, From: req.To, To: req.From,
		   Asset: req.Asset, Amount: req.Amount,
		  IdempotencyKey: idem + ":compensate",
	  })
  }
