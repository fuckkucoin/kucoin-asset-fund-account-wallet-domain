package utxograph

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func conserve(inputs, outputs []Coin, declared, feeMin string, feeMaxBps int) (string, error) {
	   sumIn := money.Zero()
	   for _, c := range inputs {
		  amt, err := money.Parse(c.Amount)
		   if err != nil {
			  return "", fmt.Errorf("utxo input amount")
		   }
		   next, err := sumIn.Add(amt)
		   if err != nil {
			   return "", err
		   }
		   sumIn = next
	   }
	  sumOut := money.Zero()
	  for _, c := range outputs {
		  amt, err := money.Parse(c.Amount)
		  if err != nil {
			  return "", fmt.Errorf("utxo output amount")
		  }
		   next, err := sumOut.Add(amt)
		  if err != nil {
			  return "", err
		  }
		   sumOut = next
	  }
	   fee, err := sumIn.Sub(sumOut)
	   if err != nil {
		  return "", fmt.Errorf("utxo fee negative")
	   }
	  if declared != "" {
		   want, err := money.ParseBook(declared)
		   if err != nil {
			  return "", fmt.Errorf("utxo fee declared")
		  }
		   if !want.Equal(fee) {
			  return "", fmt.Errorf("utxo fee mismatch")
		  }
	   }
	   if feeMin != "" && feeMin != "0" {
		   min, err := money.Parse(feeMin)
		   if err != nil {
			   return "", err
		  }
		  if fee.Cmp(min) < 0 {
			  return "", fmt.Errorf("utxo fee below min")
		  }
	   }
	  if feeMaxBps > 0 && fee.IsPositive() {
		  _, max, err := sumIn.FeeBPS(feeMaxBps)
		  if err != nil {
			  return "", err
		  }
		   // FeeBPS 返回的是费和净额。这里只拿费的上限：本金 * bps。
		  capFee, err := feeCap(sumIn, feeMaxBps)
		   if err != nil {
			   return "", err
		  }
		   _ = max
		   if fee.Cmp(capFee) > 0 {
			   return "", fmt.Errorf("utxo fee above band")
		  }
	  }
	  if fee.IsZero() {
		  return "0", nil
	   }
	   return fee.String(), nil
  }

  func feeCap(sum money.Amount, bps int) (money.Amount, error) {
	  if bps <= 0 {
		   return money.Zero(), nil
	  }
	   fee, _, err := sum.FeeBPS(bps)
	  if err != nil {
		  return money.Zero(), err
	  }
	  if fee.IsZero() {
		  // 本金很小、bps 很低时切出来的费可能被金额库收成 0。
		  // 这种情况下只要实际费本身是 0 才算带内，正费仍超带。
		  return money.Zero(), nil
	   }
	   return fee, nil
   }
