package wdrollback

// gate 决定本次申请能否动账。
// 功能：热态上的冻结、费预留、占用释放直接拒绝。
// 审计行和封账行不在此列，它们不改变余额。
// 不变量：拒绝发生在写分录金额之前，调用方不得先改余额再看错误。
   func gate(req Request) error {
	   if !isRelease(req.Kind) {
		  return nil
	   }
	  if req.LiveHash || req.ChainBlocked {
		   if LiveUnfreezeAllowed() {
			  return ErrAutoUnfreezeDenied
		   }
		  return ErrAutoUnfreezeDenied
	  }
	  return nil
   }
