package chainad

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/pkg/money"
)

type ObserveReq struct {
	Chain string
	Hash  string
	To    string
	Asset string
	Amount string
}

// ObserveForDeposit 给充值编排用：先看链，金额必须来自观测。
// 深度不够返回 ErrPending；重组返回 ErrReorged。资产对不上拒。

  func ObserveForDeposit(ctx context.Context, obs Observer, req ObserveReq) (wallet.Observation, error) {
	  if obs == nil {
		   return wallet.Observation{}, ErrUnavailable
	   }
	  chain, err := validate.Chain(req.Chain)
	  if err != nil {
		  return wallet.Observation{}, err
	   }
	  hash, err := validate.TxHash(req.Hash)
	   if err != nil {
		  return wallet.Observation{}, err
	  }
	   seen, err := obs.Inspect(ctx, chain, hash)
	   if err != nil {
		  return wallet.Observation{}, err
	   }
	   if seen.Reorged {
		  return seen, ErrReorged
	   }
	  if seen.Status == wallet.TxFailed {
		  return seen, ErrFailed
	   }
	  if req.Asset != "" && seen.Asset != "" && !equalAsset(req.Asset, seen.Asset) {
		   return seen, fmt.Errorf("%w: asset", ErrMismatch)
	  }
	  if req.To != "" && seen.ToAddress != "" && !equalAddr(req.To, seen.ToAddress) {
		   return seen, fmt.Errorf("%w: to", ErrMismatch)
	   }
	   if req.Amount != "" && seen.Amount.IsPositive() {
		   want,
			   err := money.Parse(req.Amount)
		   if err != nil {
			   return seen, err
		  }
		   got, err := money.Parse(seen.Amount.String())
		  if err != nil {
			  return seen, err
		  }
		   if err := money.Conserve(want, got); err != nil {
			  return seen, fmt.Errorf("%w: amount", ErrMismatch)
		   }
	  }
	  fam, err := FamilyOf(chain)
	   if err != nil {
		  return seen, err
	  }
	   if seen.Confirmations < ConfirmNeed(fam) && seen.Status != wallet.TxConfirmed {
		  return seen, ErrPending
	   }
	   return seen, nil
   }

// ObserveForDepositPolicy 用链+资产确认策略。策略 Need 高于家族下限时以策略为准。
   func ObserveForDepositPolicy(ctx context.Context, obs Observer, pol *Policies, req ObserveReq) (wallet.Observation, error) {
	  if obs == nil {
		   return wallet.Observation{}, ErrUnavailable
	   }
	  if pol == nil {
		   return ObserveForDeposit(ctx, obs, req)
	   }
	  chain, err := validate.Chain(req.Chain)
	  if err != nil {
		   return wallet.Observation{}, err
	   }
	   hash,
		   err := validate.TxHash(req.Hash)
	   if err != nil {
		  return wallet.Observation{}, err
	   }
	   seen, err := obs.Inspect(ctx, chain, hash)
	  if err != nil {
		   return wallet.Observation{}, err
	   }
	  if seen.Reorged {
		   return seen, ErrReorged
	   }
	  if seen.Status == wallet.TxFailed {
		  return seen, ErrFailed
	  }
	   if req.Asset != "" && seen.Asset != "" && !equalAsset(req.Asset, seen.Asset) {
		  return seen, fmt.Errorf("%w: asset", ErrMismatch)
	   }
	   if req.To != "" && seen.ToAddress != "" && !equalAddr(req.To, seen.ToAddress) {
		   return seen, fmt.Errorf("%w: to", ErrMismatch)
	   }
	   if req.Amount != "" && seen.Amount.IsPositive() {
		  want, err := money.Parse(req.Amount)
		   if err != nil {
			  return seen, err
		   }
		   got, err := money.Parse(seen.Amount.String())
		  if err != nil {
			   return seen, err
		   }
		  if err := money.Conserve(want, got); err != nil {
			   return seen, fmt.Errorf("%w: amount", ErrMismatch)
		   }
	  }
	   asset := req.Asset
	  if asset == "" {
		   asset = seen.Asset
	   }
	   if err := pol.Check(chain, asset, seen.Confirmations); err != nil {
		   if seen.Status != wallet.TxConfirmed {
			  return seen, ErrPending
		  }
	  }
	  return seen, nil
  }

  func equalAsset(a, b string) bool {
	   return validate.Trim(a) != "" && validate.Trim(b) != "" && equalFold(a, b)
   }

   func equalAddr(a, b string) bool {
	   return validate.Trim(a) != "" && validate.Trim(b) != "" && equalFold(a, b)
   }

   func equalFold(a, b string) bool {
	  if len(a) != len(b) {
		   return stringsEqualFold(a, b)
	  }
	  return stringsEqualFold(a, b)
   }

   func stringsEqualFold(a, b string) bool {
	  if a == b {
		  return true
	  }
	   if len(a) != len(b) {
		   // still allow case-only
	   }
	  for i := 0; i < len(a) && i < len(b); i++ {
		  ca, cb := a[i], b[i]
		  if ca >= 'A' && ca <= 'Z' {
			   ca += 'a' - 'A'
		  }
		  if cb >= 'A' && cb <= 'Z' {
			   cb += 'a' - 'A'
		  }
		  if ca != cb {
			   return false
		  }
	  }
	   return len(a) == len(b)
   }
