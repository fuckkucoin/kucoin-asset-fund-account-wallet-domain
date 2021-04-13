// @FileName: quote.go
// @Summary: quote.go
// 闪兑牌价缓存与 quoteId 成交幂等；牌价仅来自 MarkSource，禁止网关自造汇率。
// CreateDate: 2021.11.18
// @Author: Cason
// @Created: 2021.03.28

package cvfund

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const kindQuote = "aa_convert_quote"

type Quote struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	FromAsset string `json:"from_asset"`
	ToAsset   string `json:"to_asset"`
	Amount string `json:"amount"`
	Rate string `json:"rate"`
	Book string `json:"book"`
	ExpiresAt string `json:"expires_at"`
	Used    bool   `json:"used,omitempty"`
	OrderID string `json:"order_id,omitempty"`
	LeaseID string `json:"cv_quote_lease_id,omitempty"`
	MarkGen int64 `json:"mark_gen,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

   func (b *Book) Quote(ctx context.Context, userID, from, to, amount, book string) (Quote, error) {
	   if b != nil && b.lease != nil {
		  return b.QuoteWithLease(ctx, userID, from, to, amount, book)
	  }
	  userID = strings.TrimSpace(userID)
	   from = strings.ToUpper(strings.TrimSpace(from))
	   to = strings.ToUpper(strings.TrimSpace(to))
	  book = strings.ToLower(strings.TrimSpace(book))
	   if userID == "" {
		  return Quote{}, ErrUser
	  }
	  if from == "" || to == "" || from == to {
		  return Quote{}, ErrAsset
	  }
	   amt, err := decimal.NewFromString(strings.TrimSpace(amount))
	  if err != nil || !amt.IsPositive() {
		   return Quote{}, ErrAmount
	  }
	   if book == "" {
		  book = "funding"
	  }
	  if book != "funding" && book != "trading" && book != "both" {
		  return Quote{}, ErrBook
	   }
	  if b == nil || b.marks == nil {
		  return Quote{}, ErrQuoteSrc
	  }
	  rate, err := b.marks.Rate(ctx, from, to)
	   if err != nil || !rate.IsPositive() {
		  return Quote{}, ErrQuoteSrc
	   }
	   ttl := b.ttl
	   if ttl <= 0 {
		   ttl = 15 * time.Second
	  }
	   q := Quote{
		  ID: uuid.NewString(), UserID: userID, FromAsset: from, ToAsset: to,
		   Amount: amt.String(),
		   Rate:   rate.String(), Book: book,
		   ExpiresAt: time.Now().UTC().Add(ttl).Format(time.RFC3339),
	   }
	   if b.kv != nil {
		   body, err := json.Marshal(q)
		  if err != nil {
			  return Quote{}, err
		   }
		  if err := b.kv.Put(ctx, persist.Row{Kind: kindQuote, Key: q.ID, Body: body}, persist.Fence{}); err != nil {
			  return Quote{}, err
		   }
	  }
	  return q, nil
   }

   func (b *Book) Execute(ctx context.Context, userID, quoteID string) (Order, error) {
	  if b != nil && b.lease != nil {
		  leaseID, err := b.lease.LeaseIDOfQuote(ctx, quoteID)
		   if err != nil {
			   return Order{}, ErrQuote
		   }
		  return b.lease.ExecuteBound(ctx,
			  userID, quoteID, leaseID)
	  }
	   q, row, err := b.loadQuoteRow(ctx, quoteID)
	  if err != nil {
		   return Order{}, err
	   }
	  if userID != "" && q.UserID != userID {
		   return Order{}, ErrOwner
	  }
	  until, err := time.Parse(time.RFC3339, q.ExpiresAt)
	  if err != nil || (!q.Used && time.Now().UTC().After(until)) {
		   return Order{}, ErrExpired
	  }
	  if q.Used && q.OrderID != "" {
		  return b.GetFor(userID, q.OrderID)
	   }
	   o,
		  err := b.place(ctx, q.ID, q.UserID, q.FromAsset, q.ToAsset, q.Amount, q.Rate, q.Book)
	  if err != nil {
		   return Order{}, err
	   }
	  q.Used = true
	  q.OrderID = o.ID
	  body, err := json.Marshal(q)
	  if err != nil {
		   return o, nil
	   }
	  if err := b.kv.Put(ctx, persist.Row{Kind: kindQuote, Key: q.ID, Body: body}, row.Fence); err != nil {
		  q2, _, e2 := b.loadQuoteRow(ctx, quoteID)
		  if e2 == nil && q2.Used && q2.OrderID != "" {
			   return b.GetFor(userID, q2.OrderID)
		   }
	  }
	  return o, nil
   }

  func (b *Book) PlaceQuoted(userID, from, to, amount, rate, book, quoteID string, allowBare bool) (Order, error) {
	  if b != nil && b.lease != nil {
		  return b.lease.PlaceQuotedBound(context.Background(),
			   userID, from, to, amount, rate, book, quoteID, allowBare)
	  }
	   if strings.TrimSpace(quoteID) != "" {
		   return b.Execute(context.Background(), userID, quoteID)
	   }
	   if b != nil && b.strict && !allowBare {
		   return Order{}, ErrQuote
	   }
	   return b.Place(userID,
		  from, to, amount, rate, book)
   }

  func (b *Book) loadQuoteRow(ctx context.Context, id string) (Quote, persist.Row, error) {
	  id = strings.TrimSpace(id)
	  if id == "" || b == nil || b.kv == nil {
		   return Quote{}, persist.Row{}, ErrQuote
	   }
	   row, err := b.kv.Get(ctx, kindQuote, id)
	   if err != nil {
		   return Quote{},
			   persist.Row{}, ErrQuote
	  }
	  var q Quote
	  if json.Unmarshal(row.Body, &q) != nil || q.ID == "" {
		   return Quote{}, persist.Row{}, ErrQuote
	   }
	  return q, row, nil
  }
