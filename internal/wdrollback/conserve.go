package wdrollback

import "account-assets.kucoin.net/funding/pkg/money"

// Check 核对一个用户资产位。
// 功能：确认回滚没有把冻结或费预留变没，也没有凭空增加可用。
// entries 可含其它用户，本函数只累计同一用户同一资产。
// 不变量：任一轧差不等即返回 ErrConserve，调用方必须回滚本次内存变更。

  func Check(pos Position, entries []Entry) error {
	  eq, err := pos.Equity()
	  if err != nil {
		  return err
	  }
	  if !eq.Equal(pos.Equity0) {
		   return ErrConserve
	   }
	  sumFreeze := money.Zero()
	  sumFee := money.Zero()
	  for _, e := range entries {
		  if e.UserID != pos.UserID || e.Asset != pos.Asset || !e.Applied {
			  continue
		  }
		   amt, err := money.ParseBook(e.Amount)
		  if err != nil {
			   return ErrConserve
		  }
		  switch e.Kind {
		   case KindFreeze:
			  sumFreeze, err = sumFreeze.Add(amt)
		  case KindFee:
			  sumFee, err = sumFee.Add(amt)
		   default:
			   continue
		   }
		   if err != nil {
			  return err
		  }
	   }
	  frozenDelta, err := pos.Frozen0.Sub(pos.Frozen)
	   if err != nil || !frozenDelta.Equal(sumFreeze) {
		   return ErrConserve
	   }
	  feeDelta, err := pos.Fee0.Sub(pos.FeeReserved)
	   if err != nil || !feeDelta.Equal(sumFee) {
		   return ErrConserve
	   }
	   availDelta, err := pos.Available.Sub(pos.Available0)
	   if err != nil {
		   return ErrConserve
	  }
	  want, err := sumFreeze.Add(sumFee)
	  if err != nil || !availDelta.Equal(want) {
		   return ErrConserve
	  }
	   return nil
  }
