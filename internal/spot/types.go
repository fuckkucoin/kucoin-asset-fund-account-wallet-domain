// 现货订单结构。
// 作者: King
// 日期: 2021.01.19
package spot

import (
	"errors"
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

var (
	ErrSymbol = errors.New("invalid symbol")
	ErrSide = errors.New("side must be buy or sell")
	ErrType = errors.New("type must be market or limit")
	ErrLimitPrice = errors.New("limit price required")
)

// 订单状态与盘口结构。空簿市价 IOC 按最新成交托底。

type Side string

const (
	SideBuy Side = "buy"
	SideSell Side = "sell"
)

type Type string

const (
	TypeMarket Type = "market"
	TypeLimit  Type = "limit"
)

type Status string

const (
	StatusOpen Status = "open"
	StatusPartial   Status = "partial"
	StatusFilled    Status = "filled"
	StatusCancelled Status = "cancelled"
	StatusRejected  Status = "rejected"
)

   func (s Status) Open() bool { return s == StatusOpen || s == StatusPartial }

   func (o *Order) Open() bool {
	   return o != nil && o.Status.Open()
  }

type Trade struct {
	ID string `json:"id"`
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
	Size   string `json:"size"`
	Side       string `json:"taker_side"`
	TakerOrder string `json:"taker_order_id"`
	MakerOrder string `json:"maker_order_id"`
	TakerUser string `json:"taker_user_id"`
	MakerUser string `json:"maker_user_id"`
	Ts int64 `json:"ts"`
}

type DepthLevel struct {
	Price string `json:"price"`
	Size   string `json:"size"`
	Orders int    `json:"orders"`
}

type Depth struct {
	Symbol string       `json:"symbol"`
	Bids   []DepthLevel `json:"bids"`
	Asks []DepthLevel `json:"asks"`
	Note string       `json:"note"`
}

type Order struct {
	ID string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID         string `json:"user_id"`
	Symbol string `json:"symbol"`
	Base   string `json:"base"`
	Quote  string `json:"quote"`
	Side Side `json:"side"`
	Type Type `json:"type"`
	Size decimal.Decimal `json:"size"`
	Price  decimal.Decimal `json:"price"`
	Filled decimal.Decimal `json:"filled"`
	QuoteAmount decimal.Decimal `json:"quote_amount"`
	FeeAmount  decimal.Decimal `json:"fee_amount"`
	FeeAsset   string          `json:"fee_asset"`
	Status     Status          `json:"status"`
	FailReason string          `json:"fail_reason,omitempty"`
	FenceEpoch int64  `json:"fence_epoch,omitempty"`
	FenceToken string `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type PlaceReq struct {
	UserID string
	Symbol string
	Side string
	Type string
	Size string
	Price          string
	IdempotencyKey string
}

type OrderView struct {
	ID string `json:"id"`
	OrderID string `json:"order_id"`
	ClientOid string `json:"client_oid"`
	UserID string `json:"user_id"`
	Symbol string `json:"symbol"`
	Side   string `json:"side"`
	Type string `json:"type"`
	Size   string `json:"size"`
	Price  string `json:"price"`
	Filled string `json:"filled_size"`
	Remain string `json:"remain_size"`
	QuoteAmount string `json:"quote_amount"`
	FeeAmount string `json:"fee_amount"`
	FeeAsset string `json:"fee_asset"`
	Status   string `json:"status"`
	FailReason string `json:"fail_reason,omitempty"`
	CreatedAtMs int64 `json:"created_at_ms"`
}

  func ViewOrder(o *Order) OrderView {
	   if o == nil {
		  return OrderView{}
	  }
	   remain := o.Size.Sub(o.Filled)
	  if remain.IsNegative() {
		   remain = decimal.Zero
	   }
	  ms := int64(0)
	   if !o.CreatedAt.IsZero() {
		   ms = o.CreatedAt.UnixMilli()
	   }
	  return OrderView{
		  ID:      o.ID,
		   OrderID: o.ID, ClientOid: o.IdempotencyKey, UserID: o.UserID,
		  Symbol: o.Symbol, Side: string(o.Side), Type: string(o.Type),
		  Size: o.Size.String(), Price: o.Price.String(), Filled: o.Filled.String(),
		   Remain: remain.String(),
		   QuoteAmount: o.QuoteAmount.String(),
		   FeeAmount:   o.FeeAmount.String(), FeeAsset: o.FeeAsset, Status: string(o.Status),
		   FailReason: o.FailReason, CreatedAtMs: ms,
	  }
  }

   func (o *Order) remainSize() decimal.Decimal {
	   if o == nil {
		  return decimal.Zero
	   }
	   r := o.Size.Sub(o.Filled)
	   if r.IsNegative() {
		   return decimal.Zero
	   }
	  return r
   }

   func (o *Order) remainHold() decimal.Decimal {
	   if o == nil {
		   return decimal.Zero
	  }
	  if o.Side == SideBuy {
		   qty := o.remainSize()
		   quote := qty.Mul(o.Price)
		   if o.Size.IsZero() {
			   return quote
		   }
		   fee := decimal.Zero
		  if !o.FeeAmount.IsZero() && !o.Size.IsZero() {
			   fee = o.FeeAmount.Mul(qty).Div(o.Size)
		   }
		   return quote.Add(fee)
	   }
	   return o.remainSize()
  }

  func parseSymbol(sym string) (base, quote string, err error) {
	   sym = strings.ToUpper(strings.TrimSpace(sym))
	   a, b, ok := strings.Cut(sym, "-")
	   if !ok || a == "" || b == "" {
		  return "", "", ErrSymbol
	   }
	  return a, b, nil
   }
