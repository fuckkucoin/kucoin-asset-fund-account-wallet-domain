package holdcat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	kind    = "aa_hold_cat"
	kindRef = "aa_hold_ref"
)

const (
	ReasonOpenOrders      = "open_orders"
	ReasonPendingWithdraw = "pending_withdraw"
	ReasonPendingTransfer = "pending_transfer"
	ReasonP2PSell   = "p2p_sell"
	ReasonP2PMargin = "p2p_merchant_margin"
	ReasonEarnHold   = "earn_hold"
	ReasonMarginHold = "margin_hold"
	ReasonFuturesMargin = "futures_margin"
	ReasonIsolatedHold = "isolated_hold"
	ReasonConvertLimit = "convert_limit"
	ReasonRedEnvelope  = "red_envelope"
	ReasonLendingOrder = "lending_order"
)

var (
	ErrUser   = errors.New("hold user required")
	ErrReason = errors.New("hold reason unknown")
	ErrAmount = errors.New("hold amount invalid")
	ErrStore = errors.New("hold store unavailable")
	ErrGone  = errors.New("hold not found")
	ErrOwner = errors.New("hold owner mismatch")
)

type Row struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Book  string `json:"book"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
	Ref string `json:"ref,omitempty"`
	Open bool `json:"open"`
	CreatedAt string `json:"created_at"`
}

type Book struct {
	kv persist.KV
}

  func New(kv persist.KV) *Book { return &Book{kv: kv} }

// ParkedOnBridge 占用已随 ConvertDebit/P2PDebit 入过渡账（含红包/放贷邻接）；用户 Frozen=0，标签只解释占用。
   func ParkedOnBridge(reason string) bool {
	  switch strings.ToLower(strings.TrimSpace(reason)) {
	  case ReasonConvertLimit, ReasonP2PSell, ReasonP2PMargin, ReasonRedEnvelope, ReasonLendingOrder:
		  return true
	   default:
		  return false
	  }
  }

   func KnownReason(s string) bool {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	  case ReasonOpenOrders, ReasonPendingWithdraw, ReasonPendingTransfer,
		  ReasonP2PSell,
		  ReasonP2PMargin, ReasonEarnHold, ReasonMarginHold,
		   ReasonFuturesMargin, ReasonIsolatedHold, ReasonConvertLimit,
		  ReasonRedEnvelope, ReasonLendingOrder:
		  return true
	   default:
		   return false
	  }
  }

  func (b *Book) Place(ctx context.Context, userID, asset, book, amount, reason, ref string) (Row, error) {
	   if b == nil || b.kv == nil {
		  return Row{}, ErrStore
	  }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return Row{}, ErrUser
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		   return Row{}, ErrAmount
	   }
	  amt, err := decimal.NewFromString(strings.TrimSpace(amount))
	   if err != nil || !amt.IsPositive() {
		  return Row{}, ErrAmount
	  }
	   if !KnownReason(reason) {
		   return Row{}, ErrReason
	   }
	  if book == "" {
		  book = "funding"
	   }
	   reason = strings.ToLower(strings.TrimSpace(reason))
	  ref = strings.TrimSpace(ref)
	   if ref != "" {
		  if prev, err := b.byRef(ctx, userID, reason, ref); err == nil && prev.Open {
			  return prev, nil
		   }
	   }
	  row := Row{
		   ID: uuid.NewString(), UserID: userID, Asset: asset, Book: strings.ToLower(book),
		   Amount: amt.String(),
		  Reason: reason, Ref: ref, Open: true,
		   CreatedAt: time.Now().UTC().Format(time.RFC3339),
	  }
	  if ref != "" {
		  if err := b.kv.Put(ctx, persist.Row{Kind: kindRef, Key: refKey(userID, reason, ref), Body: []byte(row.ID)}, persist.Fence{}); err != nil {
			  if prev, e2 := b.byRef(ctx, userID, reason, ref); e2 == nil {
				   return prev, nil
			   }
			   return Row{}, err
		   }
	   }
	  body, err := json.Marshal(row)
	   if err != nil {
		   return Row{}, err
	   }
	  if err := b.kv.Put(ctx, persist.Row{Kind: kind, Key: row.ID, Body: body}, persist.Fence{}); err != nil {
		  return Row{}, err
	  }
	   return row, nil
  }

  func refKey(userID, reason, ref string) string {
	   return userID + "|" + reason + "|" + ref
  }

  func (b *Book) byRef(ctx context.Context, userID, reason, ref string) (Row, error) {
	   if b == nil || b.kv == nil {
		   return Row{}, ErrStore
	   }
	   row, err := b.kv.Get(ctx, kindRef, refKey(userID, reason, ref))
	   if err != nil {
		   return Row{}, err
	   }
	  got, _, err := b.load(ctx, string(row.Body))
	  return got, err
   }

   func (b *Book) Get(ctx context.Context, userID, id string) (Row, error) {
	   row, _, err := b.load(ctx, id)
	  if err != nil {
		  return Row{}, err
	   }
	   if userID != "" && row.UserID != userID {
		  return Row{}, ErrOwner
	   }
	  return row, nil
  }

   func (b *Book) Release(ctx context.Context, userID, id string) (Row, error) {
	   row, stored, err := b.load(ctx, id)
	   if err != nil {
		  return Row{}, err
	  }
	   if userID != "" && row.UserID != userID {
		   return Row{}, ErrOwner
	  }
	  row.Open = false
	   body, err := json.Marshal(row)
	   if err != nil {
		   return Row{}, err
	   }
	  if err := b.kv.Put(ctx, persist.Row{Kind: kind, Key: row.ID, Body: body}, stored.Fence); err != nil {
		  return Row{}, err
	  }
	  return row, nil
   }

  func (b *Book) List(ctx context.Context, userID string, openOnly bool) ([]Row, error) {
	   if b == nil || b.kv == nil {
		   return nil, nil
	   }
	  rows, err := b.kv.List(ctx, kind, 400)
	  if err != nil {
		   return nil, err
	   }
	   out := make([]Row, 0)
	   for _, r := range rows {
		   var row Row
		   if json.Unmarshal(r.Body, &row) != nil || row.ID == "" {
			   continue
		   }
		  if userID != "" && row.UserID != userID {
			   continue
		   }
		   if openOnly && !row.Open {
			  continue
		  }
		   out = append(out, row)
	   }
	   return out, nil
  }

  func (b *Book) load(ctx context.Context, id string) (Row, persist.Row, error) {
	  if b == nil || b.kv == nil {
		   return Row{}, persist.Row{}, ErrStore
	   }
	   stored, err := b.kv.Get(ctx, kind, strings.TrimSpace(id))
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return Row{}, persist.Row{}, ErrGone
		   }
		   return Row{}, persist.Row{}, err
	   }
	   var row Row
	   if json.Unmarshal(stored.Body, &row) != nil || row.ID == "" {
		   return Row{}, persist.Row{}, ErrGone
	   }
	   return row, stored, nil
  }

// ParkCoverageCheck 按用户判定哪些 open bridge 标签缺少匹配 park（与 ledger.HoldcatMismatches 同语义）。
// 仅返回应 Release 的孤儿行；禁止在此回调里补 ConvertCredit/P2PCredit。

type ParkCoverageCheck func(ctx context.Context,
	  userID string, holds []Row) (orphans []Row, err error)

// SweepOrphans 扫 open bridge 占用标签：无匹配 park 的只 Release 标签。limit 默认 200、硬顶 500。

  func (b *Book) SweepOrphans(ctx context.Context, limit int, check ParkCoverageCheck) (int, error) {
	  if b == nil || b.kv == nil || check == nil {
		  return 0, nil
	  }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   if limit <= 0 {
		   limit = 200
	   }
	  if limit > 500 {
		  limit = 500
	   }
	   rows, err := b.List(ctx, "", true)
	   if err != nil {
		   return 0, err
	  }
	  byUser := map[string][]Row{}
	  for _, r := range rows {
		  if !r.Open || !ParkedOnBridge(r.Reason) {
			  continue
		   }
		   byUser[r.UserID] = append(byUser[r.UserID], r)
	  }
	   n := 0
	  for uid, holds := range byUser {
		   if n >= limit {
			  break
		  }
		   orphans, err := check(ctx, uid, holds)
		  if err != nil {
			   return n, err
		  }
		   for _, o := range orphans {
			   if n >= limit {
				  break
			   }
			  if o.ID == "" || !o.Open {
				  continue
			   }
			  if _, err := b.Release(ctx, uid, o.ID); err != nil {
				  if errors.Is(err, ErrGone) {
					  continue
				  }
				   return n, err
			   }
			  n++
		  }
	   }
	  return n, nil
   }
