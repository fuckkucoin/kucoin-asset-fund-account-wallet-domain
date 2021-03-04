// 兑换订单和报价结构。
// 作者: Likeo
// 日期: 2021.01.13
package swap

import (
	"time"
	"github.com/shopspring/decimal"
)

type Status string

const (
	StatusQuoted Status = "quoted"
	StatusFrozen  Status = "frozen" // inventory held; chain not settled
	StatusFilled  Status = "filled"
	StatusPartial Status = "partial"
	StatusFailed  Status = "failed"
)

// Open 这单还占着钱包冻结。

   func (s Status) Open() bool {
	  return s == StatusQuoted || s == StatusFrozen
  }

type Order struct {
	ID string `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	UserID         string `json:"user_id"`
	FromAsset      string `json:"from_asset"`
	ToAsset string `json:"to_asset"`
	FromAmount   decimal.Decimal `json:"from_amount"`
	QuoteRate    decimal.Decimal `json:"quote_rate"`
	FilledAmount decimal.Decimal `json:"filled_amount"`
	ToAmount     decimal.Decimal `json:"to_amount"`
	FeeAsset string `json:"fee_asset"`
	FeeAmount decimal.Decimal `json:"fee_amount"`
	Status Status `json:"status"`
	Chain  string `json:"chain,omitempty"`
	TxHash string `json:"tx_hash,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	FailReason string `json:"fail_reason,omitempty"`
	Kind       string `json:"kind,omitempty"`
	MovementID string `json:"movement_id,omitempty"`
	FenceEpoch int64  `json:"fence_epoch,omitempty"`
	FenceToken string `json:"-"`
	// FreezeLeaseID 冻额效果令牌 ID（freeze_lease_id），供他包只读引用；空表示未启用 Lease 互斥。
	FreezeLeaseID string `json:"freeze_lease_id,omitempty"`
	// FreezeLeaseToken 效果令牌字符串，仅在内存中传递，不落库，不对外暴露。
	FreezeLeaseToken string `json:"-"`
	// ThawProofID 双释事故 ThawCase ID（thaw_proof_id），出现时表示该订单触发了双释告警。
	ThawProofID string `json:"thaw_proof_id,omitempty"`
}

type Quote struct {
	FromAsset string `json:"from_asset"`
	ToAsset    string `json:"to_asset"`
	FromAmount string `json:"from_amount"`
	Rate     string `json:"rate"`
	ToAmount string `json:"to_amount"`
	FeeAsset string `json:"fee_asset"`
	FeeAmount string `json:"fee_amount"`
	ExpiresAt int64 `json:"expires_at"`
	Source string `json:"source"`
}

type ExecReq struct {
	UserID string
	FromAsset  string
	ToAsset    string
	FromAmount string
	IdempotencyKey string
	FillRatio      string
	Chain          string
	QuoteRate string
	Async bool
}
