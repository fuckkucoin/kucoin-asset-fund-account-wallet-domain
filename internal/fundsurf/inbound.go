package fundsurf

import (
	"context"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

var ErrDest = errors.New("unknown deposit destination book")

// inboundHop 把入账科目映射为 Funding 出发的 hop。MAIN/FUNDING 留在钱包账，避免改写既有充值用例。

   func inboundHop(to string) (dir ledger.Direction, skip bool, err error) {
	   switch strings.ToUpper(strings.TrimSpace(to)) {
	  case "", "MAIN", "FUNDING", "WALLET":
		   return "", true, nil
	  case "TRADE", "TRADING", "TRADE_HF":
		   return ledger.FundingToTrading,
			   false, nil
	  case "MARGIN", "CROSS", "MARGIN_HF", "MARGIN_V2":
		   return ledger.FundingToMargin, false, nil
	  case "ISOLATED", "ISOLATED_V2":
		  return ledger.FundingToIsolated, false, nil
	  case "FUTURES", "CONTRACT":
		   return ledger.FundingToFutures,
			  false, nil
	   case "EARN":
		   // 理财必须走申购仓位；入金只落到资金账，禁止无仓位的 earn 科目。
		  return "", true, nil
	  default:
		   return "", false, ErrDest
	  }
   }

// ApplyInbound 在链上入账credited之后，按创建地址时登记的 to 划入目标科目。
// 先 WalletToSpot，再 Funding→目标；两步各用独立幂等键，失败即返回，不得伪造成功。

   func (b *Book) ApplyInbound(ctx context.Context, userID, chain, asset, amount, idem string) error {
	   if b == nil || b.led == nil {
		   return ErrLedger
	  }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return ErrUser
	  }
	  dest := b.DepositTo(ctx, userID, chain)
	   dir, skip, err := inboundHop(dest)
	   if err != nil {
		  return err
	   }
	  amt, err := ledger.ParseAmount(amount)
	   if err != nil {
		  return err
	  }
	   idem = strings.TrimSpace(idem)
	   if idem == "" {
		  return ledger.ErrMissingIdempotency
	  }
	   landFunding := !skip || strings.EqualFold(strings.TrimSpace(dest), "EARN")
	  if !landFunding {
		  return nil
	   }
	   if _, err := b.led.Create(ctx, ledger.CreateReq{
		   UserID: userID, Direction: ledger.WalletToSpot, Asset: asset, Amount: amt,
		   IdempotencyKey: idem + ":wallet-funding",
	   }); err != nil {
		  return err
	  }
	  if skip || dir == "" {
		  return nil
	  }
	   if _, err := b.led.Create(ctx, ledger.CreateReq{
		  UserID: userID, Direction: dir, Asset: asset, Amount: amt,
		  IdempotencyKey: idem + ":funding-dest",
	   }); err != nil {
		  _, _ = b.led.Create(ctx, ledger.CreateReq{
			  UserID: userID, Direction: ledger.SpotToWallet, Asset: asset, Amount: amt,
			   IdempotencyKey: idem + ":wallet-funding-undo",
		  })
		   return err
	   }
	   return nil
   }
