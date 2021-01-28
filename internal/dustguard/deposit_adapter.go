package dustguard

import "context"

// DepositAdapter 把 *ObserveHook 包成 deposit.DustGuardChecker。
// 实现接口方法签名与 deposit.DustGuardChecker 一致。
type DepositAdapter struct {
	hook *ObserveHook
}

// NewDepositAdapter 构造适配器。
   func NewDepositAdapter(hook *ObserveHook) *DepositAdapter {
	   return &DepositAdapter{hook: hook}
   }

// CheckObserve 实现 deposit.DustGuardChecker.CheckObserve。
// 地址空时跳过（FamMEMO 共享地址在更上游处理，此处不重复检查）。

   func (a *DepositAdapter) CheckObserve(ctx context.Context, chain, address, txHash, userID, amount, asset, fromAddr string) error {
	  if a == nil || a.hook == nil {
		   return nil
	   }
	   if address == "" {
		  return nil
	   }
	   p := ObserveParams{
		  UserID: userID,
		   Chain:   chain,
		  Address: address,
		  TxHash:  txHash,
		   Asset: asset,
		   Amount: amount,
		   FromAddr: fromAddr,
	  }
	   _, err := a.hook.Check(ctx, p)
	   return err
  }
