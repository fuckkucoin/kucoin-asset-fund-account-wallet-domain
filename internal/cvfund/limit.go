package cvfund

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const kindLimit = "aa_convert_limit"

// LimitOrder 限价闪兑：先冻源账，成交或取消/过期再解冻或兑入目标币。
type LimitOrder struct {
	ID string `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	UserID string `json:"user_id"`
	FromCurrency string `json:"fromCurrency"`
	ToCurrency   string `json:"toCurrency"`
	FromSize     string `json:"fromCurrencySize"`
	ToSize string `json:"toCurrencySize"`
	Price  string `json:"price"`
	AccountType string `json:"accountType"`
	Status    string `json:"status"`
	OrderTime int64  `json:"orderTime"`
	ExpiryTime int64 `json:"expiryTime"`
	FilledTime int64 `json:"filledTime,omitempty"`
	CancelTime int64 `json:"cancelTime,omitempty"`
	DebitID  string `json:"debit_id,omitempty"`
	CreditID string `json:"credit_id,omitempty"`
	HoldID   string `json:"hold_id,omitempty"`
	FilledSize string `json:"filledSize,omitempty"`
	FilledToSize string `json:"filledToSize,omitempty"`
}

   func (b *Book) LimitQuote(ctx context.Context, from, to, fromSize, toSize string) (decimal.Decimal, int64, error) {
	  if b == nil || b.marks == nil {
		   return decimal.Zero, 0, ErrQuoteSrc
	  }
	   from, to = strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
	   if from == "" || to == "" || from == to {
		   return decimal.Zero, 0, ErrAsset
	   }
	   rate, err := b.marks.Rate(ctx, from, to)
	  if err != nil || !rate.IsPositive() {
		   return decimal.Zero, 0, ErrQuoteSrc
	  }
	   _ = fromSize
	   _ = toSize
	  return rate, time.Now().UTC().Add(15 * time.Second).UnixMilli(), nil
  }

   func (b *Book) PlaceLimit(ctx context.Context, userID, clientOID, from, to, fromSize, toSize, accountType string) (LimitOrder, error) {
	  userID = strings.TrimSpace(userID)
	   clientOID = strings.TrimSpace(clientOID)
	   from, to = strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to))
	   if userID == "" {
		  return LimitOrder{}, ErrUser
	  }
	  if clientOID == "" || from == "" || to == "" || from == to {
		   return LimitOrder{}, ErrLimit
	  }
	   if prev, ok := b.limitByClient(ctx, userID, clientOID); ok {
		  return prev, nil
	   }
	  src, err := decimal.NewFromString(strings.TrimSpace(fromSize))
	  if err != nil || !src.IsPositive() {
		  return LimitOrder{}, ErrAmount
	   }
	  dst, err := decimal.NewFromString(strings.TrimSpace(toSize))
	  if err != nil || !dst.IsPositive() {
		   return LimitOrder{}, ErrAmount
	   }
	  price := dst.Div(src)
	  protect, _, err := b.LimitQuote(ctx, from, to, fromSize, toSize)
	  if err != nil {
		  return LimitOrder{}, err
	  }
	  if price.LessThan(protect) {
		   return LimitOrder{}, ErrProtect
	   }
	   book := convertBook(accountType)
	   now := time.Now().UTC()
	   o := LimitOrder{
		  ID: uuid.NewString(), ClientOrderID: clientOID, UserID: userID,
		  FromCurrency: from, ToCurrency: to, FromSize: src.String(), ToSize: dst.String(),
		   Price: price.String(), AccountType: strings.ToUpper(strings.TrimSpace(accountType)),
		   Status: "OPEN", OrderTime: now.UnixMilli(), ExpiryTime: now.Add(7 * 24 * time.Hour).UnixMilli(),
	  }
	  if o.AccountType == "" {
		  o.AccountType = "MAIN"
	  }
	   if b.led != nil {
		   if book == "trading" {
			  if _, err := b.led.Create(ctx, ledger.CreateReq{
				   UserID: userID, Direction: ledger.TradingToFunding, Asset: from, Amount: src,
				  IdempotencyKey: "cv-lim-hop:" + clientOID,
			   }); err != nil {
				  return LimitOrder{}, err
			  }
		   }
		  tr, err := b.led.Create(ctx, ledger.CreateReq{
			   UserID: userID, Direction: ledger.ConvertDebit, Asset: from, Amount: src,
			  IdempotencyKey: "cv-lim-debit:" + clientOID, CoverTrading: book == "both",
		  })
		  if err != nil {
			  return LimitOrder{}, err
		   }
		   if tr != nil {
			   o.DebitID = tr.ID
		   }
	  }
	  if b.kv != nil {
		  hold, err := holdcat.New(b.kv).Place(ctx, userID, from, "funding", src.String(), holdcat.ReasonConvertLimit, clientOID)
		  if err == nil {
			   o.HoldID = hold.ID
		   }
	  }
	  if err := b.saveLimit(ctx, o); err != nil {
		   return LimitOrder{}, err
	   }
	  return b.maybeFillLimit(ctx, o)
   }

  func (b *Book) CancelLimit(ctx context.Context, userID, orderID, clientOID string) (LimitOrder, error) {
	   o, err := b.loadLimit(ctx, userID, orderID, clientOID)
	   if err != nil {
		  return LimitOrder{}, err
	  }
	  if o.Status != "OPEN" && o.Status != "PARTIAL" {
		  return o, ErrOpen
	  }
	   if time.Now().UTC().UnixMilli() > o.ExpiryTime {
		   return b.expireLimit(ctx, o)
	  }
	   if err := b.restoreLimit(ctx, o); err != nil {
		  return LimitOrder{}, err
	   }
	   o.Status = "CANCELLED"
	  o.CancelTime = time.Now().UTC().UnixMilli()
	  o.HoldID = ""
	  if err := b.saveLimit(ctx, o); err != nil {
		  return LimitOrder{}, err
	  }
	  return o, nil
  }

   func (b *Book) GetLimit(ctx context.Context, userID, orderID, clientOID string) (LimitOrder, error) {
	  o, err := b.loadLimit(ctx, userID, orderID, clientOID)
	   if err != nil {
		   return LimitOrder{}, err
	   }
	   return b.maybeFillLimit(ctx, o)
   }

   func (b *Book) ListLimits(ctx context.Context, userID, status string, limit int) []LimitOrder {
	   if limit <= 0 || limit > 100 {
		  limit = 20
	  }
	  out := make([]LimitOrder, 0)
	   if b == nil || b.kv == nil {
		  return out
	   }
	  rows,
		  err := b.kv.List(ctx, kindLimit, 200)
	   if err != nil {
		   return out
	  }
	   status = strings.ToUpper(strings.TrimSpace(status))
	  for _, row := range rows {
		  var o LimitOrder
		  if json.Unmarshal(row.Body, &o) != nil || o.ID == "" {
			   continue
		   }
		   if userID != "" && o.UserID != userID {
			   continue
		  }
		   filled,
			  _ := b.maybeFillLimit(ctx, o)
		   o = filled
		  if status != "" && !strings.EqualFold(o.Status, status) {
			  continue
		  }
		  out = append(out, o)
		   if len(out) >= limit {
			  break
		   }
	  }
	   return out
   }

  func (b *Book) maybeFillLimit(ctx context.Context, o LimitOrder) (LimitOrder, error) {
	  if o.Status != "OPEN" {
		   return o, nil
	   }
	  if time.Now().UTC().UnixMilli() > o.ExpiryTime {
		   return b.expireLimit(ctx, o)
	  }
	  if b.marks == nil {
		  return o, nil
	   }
	   rate, err := b.marks.Rate(ctx, o.FromCurrency, o.ToCurrency)
	   if err != nil || !rate.IsPositive() {
		   return o, nil
	  }
	  src := decimal.RequireFromString(o.FromSize)
	   want := decimal.RequireFromString(o.ToSize)
	  got := rate.Mul(src)
	  if got.LessThan(want) {
		  return o, nil
	  }
	   if got.GreaterThan(want) {
		  got = want
	   }
	  got, err = ledger.RoundLedger(got)
	   if err != nil {
		  return o, err
	  }
	   if b.led != nil {
		  tr, err := b.led.Create(ctx, ledger.CreateReq{
			   UserID: o.UserID, Direction: ledger.ConvertCredit, Asset: o.ToCurrency, Amount: got,
			   IdempotencyKey: "cv-lim-credit:" + o.ClientOrderID,
		  })
		  if err != nil {
			   return o, err
		   }
		   if tr != nil {
			   o.CreditID = tr.ID
		   }
		   if convertBook(o.AccountType) == "trading" {
			   if _, err := b.led.Create(ctx, ledger.CreateReq{
				   UserID:    o.UserID,
				  Direction: ledger.FundingToTrading, Asset: o.ToCurrency, Amount: got,
				  IdempotencyKey: "cv-lim-hop-out:" + o.ClientOrderID,
			  }); err != nil {
				   _ = err
			  }
		  }
	   }
	  if o.HoldID != "" && b.kv != nil {
		  _, _ = holdcat.New(b.kv).Release(ctx, o.UserID, o.HoldID)
	  }
	  o.Status = "FILLED"
	  o.FilledTime = time.Now().UTC().UnixMilli()
	  o.ToSize = got.String()
	  if err := b.saveLimit(ctx, o); err != nil {
		  return o, err
	  }
	  return o, nil
  }

  func (b *Book) expireLimit(ctx context.Context, o LimitOrder) (LimitOrder, error) {
	   if o.Status != "OPEN" && o.Status != "PARTIAL" {
		   return o, nil
	  }
	   if err := b.restoreLimit(ctx, o); err != nil {
		  return o, err
	  }
	   o.Status = "EXPIRED"
	   o.CancelTime = time.Now().UTC().UnixMilli()
	  o.HoldID = ""
	   if err := b.saveLimit(ctx, o); err != nil {
		  return o, err
	  }
	  return o, nil
  }

// restoreLimit 退还限价单剩余未成交源币并释放 holdcat。
// 部分成交后只退 FromSize-FilledSize；幂等键含已成交量避免与全额退冲突。
// 不得把已兑出的源币再次贷回。

  func (b *Book) restoreLimit(ctx context.Context, o LimitOrder) error {
	  src, err := decimal.NewFromString(o.FromSize)
	   if err != nil {
		  return err
	  }
	   filled := decimal.Zero
	   if strings.TrimSpace(o.FilledSize) != "" {
		  filled, err = decimal.NewFromString(o.FilledSize)
		   if err != nil {
			  return err
		   }
	   }
	   remain := src.Sub(filled)
	  if !remain.IsPositive() {
		   if o.HoldID != "" && b.kv != nil {
			  _, _ = holdcat.New(b.kv).Release(ctx, o.UserID, o.HoldID)
		   }
		   return nil
	  }
	  idemSuffix := filled.String()
	  if b.led != nil {
		  if _, err := b.led.Create(ctx, ledger.CreateReq{
			  UserID: o.UserID, Direction: ledger.ConvertCredit, Asset: o.FromCurrency, Amount: remain,
			  IdempotencyKey: "cv-lim-restore:" + o.ClientOrderID + ":" + idemSuffix,
		   }); err != nil {
			   return err
		   }
		  if convertBook(o.AccountType) == "trading" {
			   if _, err := b.led.Create(ctx, ledger.CreateReq{
				   UserID: o.UserID, Direction: ledger.FundingToTrading, Asset: o.FromCurrency, Amount: remain,
				  IdempotencyKey: "cv-lim-hop-back:" + o.ClientOrderID + ":" + idemSuffix,
			  }); err != nil {
				  return err
			   }
		   }
	  }
	   if o.HoldID != "" && b.kv != nil {
		   _, _ = holdcat.New(b.kv).Release(ctx, o.UserID, o.HoldID)
	  }
	  return nil
  }

  func convertBook(accountType string) string {
	  switch strings.ToUpper(strings.TrimSpace(accountType)) {
	  case "TRADE", "TRADING":
		  return "trading"
	   case "BOTH":
		   return "both"
	   default:
		   return "funding"
	  }
   }

   func (b *Book) saveLimit(ctx context.Context, o LimitOrder) error {
	  if b == nil || b.kv == nil {
		   return ErrQuoteSrc
	  }
	   body, err := json.Marshal(o)
	   if err != nil {
		  return err
	  }
	  exp := persist.Fence{}
	  if cur, err := b.kv.Get(ctx, kindLimit, o.ID); err == nil {
		   exp = cur.Fence
	  }
	  if err := b.kv.Put(ctx, persist.Row{Kind: kindLimit, Key: o.ID, Body: body}, exp); err != nil {
		  return err
	  }
	  idx := "oid|" + o.UserID + "|" + o.ClientOrderID
	   if _, err := b.kv.Get(ctx, kindLimit, idx); err != nil {
		  _ = b.kv.Put(ctx,
			  persist.Row{Kind: kindLimit, Key: idx, Body: []byte(o.ID)}, persist.Fence{})
	   }
	  return nil
   }

  func (b *Book) limitByClient(ctx context.Context, userID, clientOID string) (LimitOrder, bool) {
	  if b == nil || b.kv == nil {
		  return LimitOrder{}, false
	   }
	   row, err := b.kv.Get(ctx, kindLimit, "oid|"+userID+"|"+clientOID)
	   if err != nil {
		   return LimitOrder{}, false
	  }
	  got, err := b.loadLimit(ctx, userID, string(row.Body), "")
	   if err != nil {
		  return LimitOrder{}, false
	  }
	  return got, true
  }

   func (b *Book) loadLimit(ctx context.Context, userID, orderID, clientOID string) (LimitOrder, error) {
	  if b == nil || b.kv == nil {
		  return LimitOrder{}, ErrGone
	  }
	   id := strings.TrimSpace(orderID)
	  if id == "" && clientOID != "" {
		   if prev, ok := b.limitByClient(ctx, userID, clientOID); ok {
			   return prev, nil
		  }
		  return LimitOrder{}, ErrGone
	  }
	   row, err := b.kv.Get(ctx, kindLimit, id)
	  if err != nil {
		  return LimitOrder{}, ErrGone
	  }
	  var o LimitOrder
	   if json.Unmarshal(row.Body, &o) != nil || o.ID == "" {
		   return LimitOrder{}, ErrGone
	  }
	  if userID != "" && o.UserID != userID {
		   return LimitOrder{}, ErrOwner
	  }
	   return o, nil
  }

// SweepExpiredLimits 后台过期 OPEN 限价：无用户触达时仍走 expireLimit，解冻 ConvertDebit 与 holdcat。
  func (b *Book) SweepExpiredLimits(ctx context.Context, limit int) (int, error) {
	   if b == nil || b.kv == nil {
		  return 0, nil
	  }
	   if limit <= 0 {
		   limit = 200
	   }
	  if limit > 500 {
		   limit = 500
	   }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	   scan := limit * 2
	   if scan < 200 {
		  scan = 200
	   }
	   if scan > 1000 {
		  scan = 1000
	  }
	  rows, err := b.kv.List(ctx, kindLimit, scan)
	  if err != nil {
		  return 0, err
	  }
	   now := time.Now().UTC().UnixMilli()
	  n := 0
	   for _, row := range rows {
		   if n >= limit {
			   break
		   }
		  var o LimitOrder
		  if json.Unmarshal(row.Body, &o) != nil || o.ID == "" {
			  continue
		  }
		   if (o.Status != "OPEN" && o.Status != "PARTIAL") || now <= o.ExpiryTime {
			  continue
		  }
		  got, err := b.expireLimit(ctx, o)
		  if err != nil {
			  return n, err
		  }
		  if got.Status == "EXPIRED" {
			  n++
		   }
	  }
	   return n, nil
  }
