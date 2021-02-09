// 兑换单对外展示。
// 作者: Span
// 日期: 2021.01.13
package swap

import (
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

  func (s Status) AppStatus() string {
	   switch s {
	   case StatusFilled:
		  return "success"
	   case StatusPartial:
		  return "partial"
	   case StatusFailed:
		   return "failed"
	  case StatusFrozen:
		  return "frozen"
	   case StatusQuoted:
		   return "quoted"
	   default:
		  return string(s)
	  }
   }

  func (s Status) AppStatusDesc() string {
	  switch s.AppStatus() {
	   case "success":
		  return "filled; wallet books and fee credited"
	  case "partial":
		  return "partial fill settled; unfilled not frozen"
	   case "failed":
		  return "failed; freeze released when no live hash"
	  case "frozen":
		  return "wallet inventory frozen, chain not settled"
	  case "quoted":
		  return "quoted, not frozen"
	  default:
		   return string(s)
	  }
   }

// OrderView 给前端的兑换单。机内状态仍在 Status。
type OrderView struct {
	ID string `json:"id"`
	OrderID    string `json:"order_id"`
	ClientOid  string `json:"client_oid"`
	MovementID string `json:"movement_id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID    string `json:"user_id"`
	FromAsset string `json:"from_asset"`
	ToAsset    string `json:"to_asset"`
	FromAmount string `json:"from_amount"`
	FilledAmount string `json:"filled_amount"`
	RemainAmount string `json:"remain_amount"`
	ToAmount     string `json:"to_amount"`
	QuoteRate    string `json:"quote_rate"`
	FeeAsset  string `json:"fee_asset"`
	FeeAmount string `json:"fee_amount"`
	Account   string `json:"account"`
	Status string `json:"status"`
	StatusDesc string `json:"status_desc"`
	State string `json:"state"`
	Chain  string `json:"chain,omitempty"`
	TxHash string `json:"tx_hash,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
	Kind string `json:"kind"`
	FenceEpoch  int64     `json:"fence_epoch,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedAtMs int64     `json:"created_at_ms"`
}

  func ViewOrder(o *Order) OrderView {
	   if o == nil {
		  return OrderView{}
	   }
	   remain := o.FromAmount.Sub(o.FilledAmount)
	  if remain.IsNegative() {
		  remain = decimal.Zero
	  }
	   kind := o.Kind
	   if kind == "" {
		  kind = "swap"
	   }
	  mid := o.MovementID
	   if mid == "" {
		  mid = o.ID
	  }
	   return OrderView{
		  ID: o.ID, OrderID: o.ID, ClientOid: o.IdempotencyKey, MovementID: mid,
		  IdempotencyKey: o.IdempotencyKey, UserID: o.UserID,
		  FromAsset:  o.FromAsset,
		   ToAsset:    o.ToAsset,
		  FromAmount: o.FromAmount.String(), FilledAmount: o.FilledAmount.String(),
		   RemainAmount: remain.String(), ToAmount: o.ToAmount.String(),
		  QuoteRate: o.QuoteRate.String(), FeeAsset: o.FeeAsset, FeeAmount: o.FeeAmount.String(),
		   Account: ledger.AccountWallet.Subject(),
		   Status: o.Status.AppStatus(), StatusDesc: o.Status.AppStatusDesc(), State: string(o.Status),
		  Chain: o.Chain, TxHash: o.TxHash, FailReason: o.FailReason, Kind: kind,
		   FenceEpoch: o.FenceEpoch, CreatedAt: o.CreatedAt, CreatedAtMs: o.CreatedAt.UnixMilli(),
	  }
   }

  func MatchOrder(o *Order, status string) bool {
	   if o == nil {
		  return false
	   }
	   if status == "" {
		   return true
	  }
	  return o.Status.AppStatus() == status || string(o.Status) == status
   }

   func FeesPaid(orders []*Order) []ledger.FeePaid {
	   by := map[string]decimal.Decimal{}
	   for _, o := range orders {
		  if o == nil {
			  continue
		   }
		  if o.Status != StatusFilled && o.Status != StatusPartial {
			   continue
		  }
		  if o.FeeAmount.IsZero() {
			  continue
		   }
		   by[o.FeeAsset] = by[o.FeeAsset].Add(o.FeeAmount)
	   }
	   out := make([]ledger.FeePaid, 0, len(by))
	  for asset, amt := range by {
		   out = append(out, ledger.FeePaid{Asset: asset, Amount: amt.String(), Subject: "fee"})
	  }
	  return out
   }
