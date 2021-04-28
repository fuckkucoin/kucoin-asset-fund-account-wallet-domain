package chainad

import (
	"time"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

type Status string

const (
	StPending Status = "pending"
	StConfirmed Status = "confirmed"
	StFailed    Status = "failed"
	StReorged Status = "reorged"
)

type Inbound struct {
	Family Family `json:"family"`
	Chain  string `json:"chain"`
	Hash   string `json:"hash"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	To     string `json:"to"`
	From string `json:"from,omitempty"`
	Memo string `json:"memo,omitempty"`
	Tag           string `json:"tag,omitempty"`
	TagName       string `json:"tag_name,omitempty"`
	Confirmations int    `json:"confirmations"`
	Height    int64           `json:"height"`
	BlockHash string          `json:"block_hash,omitempty"`
	Status    Status          `json:"status"`
	Reorged   bool            `json:"reorged"`
	SeenAt    time.Time       `json:"seen_at"`
	Raw       decimal.Decimal `json:"-"`
}

type Outbound struct {
	Family Family `json:"family"`
	Chain  string `json:"chain"`
	From   string `json:"from"`
	To     string `json:"to"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Nonce  string `json:"nonce,omitempty"`
	Memo string `json:"memo,omitempty"` // FamMEMO 备注；来自 transfers.memo_hint
	Tag     string `json:"tag,omitempty"`      // 与 Memo 同义；来自 tag_hint
	TagName string `json:"tag_name,omitempty"` // memo / destination_tag / comment
	DestinationTag string `json:"destination_tag,omitempty"` // XRP 等；广播/签章透传
}

type Receipt struct {
	Hash string `json:"hash"`
	Family Family `json:"family"`
	Chain  string `json:"chain"`
	Status Status `json:"status"`
	Nonce  string `json:"nonce,omitempty"`
	Height int64 `json:"height,omitempty"`
	Replayed bool `json:"replayed,omitempty"`
}

type Block struct {
	Height int64 `json:"height"`
	Hash string `json:"hash"`
	Txs []string `json:"txs"`
}

  func (in Inbound) Observation() wallet.Observation {
	   st := wallet.TxPending
	  switch in.Status {
	   case StConfirmed:
		   st = wallet.TxConfirmed
	   case StFailed:
		   st = wallet.TxFailed
	   case StReorged:
		  st = wallet.TxReorged
	  }
	  amt := in.Raw
	   if amt.IsZero() && in.Amount != "" {
		   if d, err := decimal.NewFromString(in.Amount); err == nil {
			  amt = d
		   }
	   }
	   return wallet.Observation{
		  Hash: in.Hash, Asset: in.Asset, Amount: amt, ToAddress: in.To,
		  FromAddress: in.From, Memo: in.Memo, Tag: in.Tag, TagName: in.TagName,
		  Status:        st,
		  Confirmations: in.Confirmations, Height: in.Height,
		   BlockHash: in.BlockHash, Reorged: in.Reorged,
	   }
   }
