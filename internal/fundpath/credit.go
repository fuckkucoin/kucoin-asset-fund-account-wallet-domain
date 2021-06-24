package fundpath

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func (s *Service) Credit(ctx context.Context, req RunReq, uid, chain, asset string) (string, error) {
	   if s == nil {
		   return "", ErrCredit
	   }
	  if s.Halted() {
		  return "", ErrHalt
	   }
	  if req.Amount != "" {
		  if err := s.attr.DustReject(req.Amount); err != nil {
			   return "", ErrDust
		  }
		   if _, err := money.Parse(req.Amount); err != nil {
			   return "", ErrAmount
		  }
	  }
	   if s.dep == nil {
		   return "", ErrCredit
	   }
	  if req.TxHash == "" {
		  return "", fmt.Errorf("%w: tx_hash required", ErrCredit)
	   }
	  tr, err := s.dep.Observe(ctx, deposit.ObserveReq{
		  UserID: uid, Chain: chain, TxHash: req.TxHash, Asset: asset,
		   IdempotencyKey: req.IdempotencyKey + "-obs", WaitConfirm: req.WaitConfirm,
		  Memo: req.Memo, Tag: req.Tag, UTXO: req.UTXO,
	  })
	   if err != nil {
		  return "", err
	  }
	  if req.Amount != "" {
		  got, err := money.Parse(tr.Amount.String())
		  if err != nil {
			  return "", ErrAmount
		   }
		   want, err := money.Parse(req.Amount)
		   if err != nil {
			   return "", ErrAmount
		  }
		   if err := money.Conserve(got, want); err != nil {
			   return "", err
		   }
	  }
	  return tr.ID, nil
   }
