package chainad

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// SubmitForWithdraw 出金广播。地址、金额先过闸；同一 nonce 回放原哈希。

   func SubmitForWithdraw(ctx context.Context, sub Submitter, req Outbound) (Receipt, error) {
	   if sub == nil {
		  return Receipt{}, ErrUnavailable
	  }
	   if req.Chain == "" {
		   req.Chain = firstChain(sub.Family())
	  }
	   ch,
		   err := validate.Chain(req.Chain)
	   if err != nil {
		  return Receipt{}, err
	   }
	  req.Chain = ch
	  if _, err := validate.Address(ch, req.To); err != nil {
		  if err := wallet.ValidChainAddress(ch, req.To); err != nil {
			  return Receipt{}, fmt.Errorf("%w: %v", ErrAddress, err)
		   }
	  }
	   if _, err := money.Parse(req.Amount); err != nil {
		  return Receipt{}, err
	  }
	   if req.Asset != "" {
		  if _, err := validate.Asset(req.Asset); err != nil {
			  return Receipt{}, err
		  }
	   }
	  // 备注链广播前归一 destination_tag 并过 CheckMemo；非备注链不塞空备注。
	   NormalizeOutboundMemo(&req)
	  if RequiresMemo(req.Chain) {
		   memo := strings.TrimSpace(req.DestinationTag)
		  if memo == "" {
			  memo = strings.TrimSpace(req.Memo)
		  }
		   if memo == "" {
			   memo = strings.TrimSpace(req.Tag)
		   }
		   if err := CheckMemo(req.Chain, memo); err != nil {
			   return Receipt{}, err
		  }
	   }
	   rec, err := sub.SubmitWithdraw(ctx, req)
	   if err != nil {
		  return Receipt{}, err
	  }
	   if rec.Hash == "" {
		   return rec, ErrSubmit
	  }
	   return rec, nil
  }
