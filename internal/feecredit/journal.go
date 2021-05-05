package feecredit

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func journalTake(asset string, q feeeng.Quote) ([]money.Line, error) {
	  gross, err := money.Parse(q.Gross)
	   if err != nil {
		  return nil, err
	   }
	   fee, err := money.ParseBook(q.Fee)
	  if err != nil {
		  return nil, err
	   }
	  net, err := money.ParseBook(q.Net)
	  if err != nil {
		   return nil, err
	  }
	  sum, err := fee.Add(net)
	  if err != nil {
		   return nil, err
	  }
	   if err := money.Conserve(sum, gross); err != nil {
		  return nil, fmt.Errorf("%w: %v", ErrJournal, err)
	  }
	  if fee.IsZero() {
		  return money.DebitPair(asset, gross), nil
	   }
	  lines := []money.Line{
		   {Asset: asset, Debit: gross, Memo: "user_funding_debit_gross"},
		  {Asset: asset, Credit: net, Memo: "bridge_net"},
		   {Asset: asset, Credit: fee, Memo: "system_fee"},
	  }
	   if err := money.Balanced(lines); err != nil {
		   return nil, fmt.Errorf("%w: %v", ErrJournal, err)
	  }
	   return lines, nil
  }

   func journalRebate(asset string, q feeeng.Quote) ([]money.Line, error) {
	   fee,
		  err := money.ParseBook(q.Fee)
	   if err != nil {
		  return nil, err
	  }
	   if !fee.IsPositive() {
		  return nil, ErrAmount
	   }
	   lines := money.TransferPair(asset, fee, "bridge_rebate", "user_funding_credit")
	   if err := money.Balanced(lines); err != nil {
		   return nil, fmt.Errorf("%w: %v", ErrJournal, err)
	   }
	  return lines, nil
   }
