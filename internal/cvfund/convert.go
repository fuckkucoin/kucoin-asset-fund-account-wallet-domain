package cvfund

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	ErrUser  = errors.New("convert user required")
	ErrAsset = errors.New("convert assets required")
	ErrAmount = errors.New("convert amount required")
	ErrRate = errors.New("convert rate required")
	ErrBook = errors.New("convert book funding|trading")
	ErrGone = errors.New("convert order not found")
	ErrOwner   = errors.New("convert order owner mismatch")
	ErrQuote   = errors.New("convert quote required")
	ErrExpired = errors.New("convert quote expired")
	ErrQuoteSrc = errors.New("convert quote source unavailable")
	ErrLimit   = errors.New("convert limit order invalid")
	ErrProtect = errors.New("convert limit price below protection")
	ErrOpen    = errors.New("convert limit order not open")
)

const kind = "aa_convert_order"

func orderKVKey(userID, orderID string) string {
	return "u/" + strings.TrimSpace(userID) + "/o/" + strings.TrimSpace(orderID)
}

func orderKVUserPrefix(userID string) string {
	return "u/" + strings.TrimSpace(userID) + "/o/"
}

type Order struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	FromAsset string `json:"from_asset"`
	ToAsset string `json:"to_asset"`
	Amount string `json:"amount"`
	Rate   string `json:"rate"`
	Book   string `json:"book"`
	Status string `json:"status"`
	CreatedAt string `json:"created_at"`
	DebitID   string `json:"debit_id,omitempty"`
	CreditID  string `json:"credit_id,omitempty"`
}

type Book struct {
	mu sync.Mutex
	ord map[string]Order
	kv persist.KV
	led *ledger.Service
	marks  MarkSource
	strict bool
	ttl time.Duration
	lease      QuoteLeasePort
	occChecker OccCheckerAdapter
}

// QuoteLeasePort 报价租约端口，实现位于 cvquotelease，避免包循环。
type QuoteLeasePort interface {
	QuoteIssued(ctx context.Context, userID, from, to, amount, book string) (Quote, error)
	LeaseIDOfQuote(ctx context.Context, quoteID string) (string, error)
	ExecuteBound(ctx context.Context, userID, quoteID, leaseID string) (Order, error)
	PlaceQuotedBound(ctx context.Context, userID, from, to, amount, rate, book, quoteID string, allowBare bool) (Order, error)
}

   func (b *Book) BindQuoteLeasePort(p QuoteLeasePort) *Book {
	   if b != nil {
		  b.lease = p
		   if p != nil {
			  b.strict = true
		  }
	  }
	   return b
  }

type MarkSource interface {
	Rate(ctx context.Context, from, to string) (decimal.Decimal, error)
}

  func New() *Book { return &Book{ord: map[string]Order{}} }

  func (b *Book) BindKV(kv persist.KV) *Book {
	   if b != nil {
		  b.kv = kv
	  }
	  return b
  }

   func (b *Book) BindLedger(led *ledger.Service) *Book {
	  if b != nil {
		   b.led = led
	  }
	  return b
  }

  func (b *Book) BindMarks(src MarkSource) *Book {
	   if b != nil {
		  b.marks = src
	   }
	  return b
  }

  func (b *Book) BindStrict(on bool, ttl time.Duration) *Book {
	  if b != nil {
		  b.strict = on
		   if ttl > 0 {
			  b.ttl = ttl
		  }
	   }
	   return b
   }

   func (b *Book) Place(userID, from, to, amount, rate, book string) (Order, error) {
	  return b.place(context.Background(), "", userID, from, to, amount, rate, book)
   }

  func (b *Book) place(ctx context.Context, id, userID, from, to, amount, rate, book string) (Order, error) {
	   userID = strings.TrimSpace(userID)
	  from = strings.ToUpper(strings.TrimSpace(from))
	  to = strings.ToUpper(strings.TrimSpace(to))
	   book = strings.ToLower(strings.TrimSpace(book))
	  id = strings.TrimSpace(id)
	  if userID == "" {
		  return Order{}, ErrUser
	   }
	  if from == "" || to == "" || from == to {
		  return Order{}, ErrAsset
	  }
	  amt, err := decimal.NewFromString(strings.TrimSpace(amount))
	   if err != nil || !amt.IsPositive() {
		   return Order{}, ErrAmount
	  }
	   rt, err := decimal.NewFromString(strings.TrimSpace(rate))
	   if err != nil || !rt.IsPositive() {
		   return Order{}, ErrRate
	   }
	   if book == "" {
		   book = "funding"
	   }
	   if book != "funding" && book != "trading" && book != "both" {
		   return Order{}, ErrBook
	  }
	   if id != "" {
		   if prev, err := b.Get(id); err == nil && prev.ID != "" {
			   return prev, nil
		  }
	   } else {
		  id = uuid.NewString()
	  }
	  o := Order{
		  ID:     id,
		   UserID: userID, FromAsset: from, ToAsset: to,
		   Amount: amt.String(), Rate: rt.String(), Book: book, Status: "accepted",
		   CreatedAt: time.Now().UTC().Format(time.RFC3339),
	  }
	   if err := b.settle(ctx, &o, amt, rt); err != nil {
		   return Order{}, err
	  }
	   if err := b.save(o, persist.Fence{}); err != nil {
		  return Order{}, err
	   }
	  return o, nil
  }

// settle 按调用方牌价记账。Trading 先划入 Funding 再出账；目标币入 Funding 后再划回 Trading。

  func (b *Book) settle(ctx context.Context, o *Order, amt, rate decimal.Decimal) error {
	  if b == nil || b.led == nil {
		  return nil
	   }
	   got := amt.Mul(rate)
	  if !got.IsPositive() {
		  return ErrAmount
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  got, err := ledger.RoundLedger(got)
	  if err != nil {
		  return err
	  }
	   if o.Book == "trading" {
		  if _, err := b.led.Create(ctx, ledger.CreateReq{
			  UserID: o.UserID, Direction: ledger.TradingToFunding, Asset: o.FromAsset, Amount: amt,
			  IdempotencyKey: "cv-hop-in:" + o.ID,
		  }); err != nil {
			  return err
		   }
	  }
	   debit, err := b.led.Create(ctx, ledger.CreateReq{
		  UserID: o.UserID, Direction: ledger.ConvertDebit, Asset: o.FromAsset, Amount: amt,
		   IdempotencyKey: "cv-debit:" + o.ID, CoverTrading: o.Book == "both",
	  })
	   if err != nil {
		   return err
	   }
	  credit, err := b.led.Create(ctx, ledger.CreateReq{
		   UserID: o.UserID, Direction: ledger.ConvertCredit, Asset: o.ToAsset, Amount: got,
		   IdempotencyKey: "cv-credit:" + o.ID,
	   })
	  if err != nil {
		  _, _ = b.led.Create(ctx, ledger.CreateReq{
			   UserID: o.UserID, Direction: ledger.ConvertCredit, Asset: o.FromAsset, Amount: amt,
			   IdempotencyKey: "cv-debit-undo:" + o.ID,
		   })
		   return err
	   }
	  // BOTH：目标币留资金账（支付合计只约束源侧 CoverTrading）。
	  // TRADING：目标币划回交易账；失败不回滚已成交，凭幂等键可重试。
	  if o.Book == "trading" {
		  if _, err := b.led.Create(ctx, ledger.CreateReq{
			  UserID: o.UserID, Direction: ledger.FundingToTrading, Asset: o.ToAsset, Amount: got,
			   IdempotencyKey: "cv-hop-out:" + o.ID,
		  }); err != nil {
			   _ = err
		   }
	  }
	   if debit != nil {
		  o.DebitID = debit.ID
	  }
	  if credit != nil {
		   o.CreditID = credit.ID
	  }
	  o.Status = "filled"
	   return nil
   }

  func (b *Book) save(o Order, exp persist.Fence) error {
	  b.mu.Lock()
	  b.ord[o.ID] = o
	  b.mu.Unlock()
	  if b.kv == nil {
		  return nil
	   }
	  body, err := json.Marshal(o)
	   if err != nil {
		   return err
	   }
	  ctx := context.Background()
	  if err := b.kvPut(ctx, orderKVKey(o.UserID, o.ID), body, exp); err != nil {
		   return err
	  }
	  if !b.strict {
		   return b.kvPut(ctx, o.ID, body, exp)
	  }
	  return nil
  }

  func (b *Book) kvPut(ctx context.Context, key string, body []byte, exp persist.Fence) error {
	  return b.kv.Put(ctx, persist.Row{Kind: kind, Key: key, Body: body}, exp)
  }

  func (b *Book) Get(id string) (Order, error) {
	   id = strings.TrimSpace(id)
	  b.mu.Lock()
	   o, ok := b.ord[id]
	  b.mu.Unlock()
	   if ok {
		   return o, nil
	  }
	  if b.kv != nil {
		   row, err := b.kv.Get(context.Background(), kind, id)
		  if err == nil {
			  var stored Order
			   if json.Unmarshal(row.Body, &stored) == nil && stored.ID != "" {
				   return stored, nil
			  }
		   }
	   }
	  return Order{}, ErrGone
   }

  func (b *Book) GetFor(userID, id string) (Order, error) {
	   id = strings.TrimSpace(id)
	  userID = strings.TrimSpace(userID)
	  b.mu.Lock()
	  if o, ok := b.ord[id]; ok && (userID == "" || o.UserID == userID) {
		   b.mu.Unlock()
		   return o, nil
	  }
	  b.mu.Unlock()
	  if b.kv != nil && userID != "" {
		   row, err := b.kv.Get(context.Background(), kind, orderKVKey(userID, id))
		   if err == nil {
			   var o Order
			   if json.Unmarshal(row.Body, &o) == nil && o.ID != "" {
				   if userID != "" && o.UserID != userID {
					   return Order{}, ErrOwner
				   }
				   return o, nil
			   }
		   }
	  }
	  o, err := b.Get(id)
	   if err != nil {
		   return Order{}, err
	  }
	   if userID != "" && o.UserID != userID {
		  return Order{}, ErrOwner
	  }
	  return o, nil
  }

  func (b *Book) List(userID string, limit int) []Order {
	  if limit <= 0 || limit > 100 {
		  limit = 20
	  }
	   if b.kv != nil && b.strict {
		   return b.listKVUser(strings.TrimSpace(userID), limit)
	   }
	   out := make([]Order, 0)
	  b.mu.Lock()
	   for _, o := range b.ord {
		   if userID == "" || o.UserID == userID {
			   out = append(out, o)
		  }
	   }
	  b.mu.Unlock()
	  if b.kv != nil {
		  rows,
			   err := b.kv.List(context.Background(), kind, 100)
		  if err == nil {
			   seen := map[string]struct{}{}
			  for _, o := range out {
				  seen[o.ID] = struct{}{}
			   }
			  for _, row := range rows {
				  var o Order
				  if json.Unmarshal(row.Body, &o) != nil || o.ID == "" {
					  continue
				   }
				   if userID != "" && o.UserID != userID {
					  continue
				  }
				   if _, ok := seen[o.ID]; ok {
					  continue
				  }
				  out = append(out, o)
			  }
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	  if len(out) > limit {
		  return out[:limit]
	  }
	  return out
   }

  func (b *Book) listKVUser(userID string, limit int) []Order {
	  if userID == "" {
		  return nil
	  }
	  prefix := orderKVUserPrefix(userID)
	  after := prefix
	  out := make([]Order, 0, limit)
	  ctx := context.Background()
	  for len(out) < limit {
		   rows, err := b.kv.ListAfter(ctx, kind, after, limit*4)
		   if err != nil || len(rows) == 0 {
			   break
		   }
		   for _, row := range rows {
			   if !strings.HasPrefix(row.Key, prefix) {
				   return out
			   }
			   var o Order
			   if json.Unmarshal(row.Body, &o) != nil || o.ID == "" {
				   continue
			   }
			   out = append(out, o)
			   after = row.Key
			   if len(out) >= limit {
				   break
			   }
		   }
		   if len(rows) < limit*4 {
			   break
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	  if len(out) > limit {
		  return out[:limit]
	  }
	  return out
  }
