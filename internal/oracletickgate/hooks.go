package oracletickgate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ConvertHook 只读 Convert，可选签发租约。
  func (s *Service) ConvertHook(ctx context.Context, asset, amount, quote string, mode LeaseMode, actor string) (money.Amount, oracle.Tick, *TickLease, error) {
	   val, tick, err := s.Feed().Convert(asset, amount, quote)
	  if err != nil {
		   if mode == ModeEnforce {
			  return val, tick, nil, err
		   }
		  return val, tick, nil, err
	  }
	   lease, lerr := s.IssueTickLease(ctx, IssueTickLeaseRequest{
		   Asset: asset, Quote: quote, Amount: amount, ValuationKind: ValConvert, Mode: mode, Actor: actor,
	   })
	  return val, tick, lease, lerr
   }

// MarkToMarketHook 只读 MTM + 租约。
   func (s *Service) MarkToMarketHook(ctx context.Context, quote string, amounts map[string]string, mode LeaseMode, actor string) (oracle.Mark, *TickLease, error) {
	  mark,
		  err := s.Feed().MarkToMarket(quote, amounts)
	   if err != nil {
		   return mark, nil, err
	  }
	  lease, lerr := s.IssueTickLease(ctx, IssueTickLeaseRequest{
		   Quote: quote, Amounts: amounts, ValuationKind: ValMTM, Mode: mode, Actor: actor,
	  })
	  return mark, lease, lerr
   }

// MarginLTVHook 只读 LTV + 租约。
   func (s *Service) MarginLTVHook(ctx context.Context, borrowAsset, borrowAmt, colAsset, colAmt, quote string, mode LeaseMode, actor string) (int, *TickLease, error) {
	  bps, err := s.Feed().MarginLTV(borrowAsset, borrowAmt, colAsset, colAmt, quote)
	   if err != nil {
		   return 0, nil, err
	   }
	   lease, lerr := s.IssueTickLease(ctx, IssueTickLeaseRequest{
		   Quote: quote, ValuationKind: ValLTV, Mode: mode, Actor: actor,
		  BorrowAsset: borrowAsset, BorrowAmt: borrowAmt, ColAsset: colAsset, ColAmt: colAmt,
	   })
	  return bps, lease, lerr
  }

// FundWriteHook 资金写调用点：先 Assert。
  func (s *Service) FundWriteHook(ctx context.Context, leaseID string, kind FundWriteKind, ref, actor string) (*FundWriteRecord, error) {
	   _, rec, err := s.AssertBeforeFundWrite(ctx, AssertFundWriteRequest{
		   LeaseID: leaseID, WriteKind: kind, WriteRef: ref, Actor: actor,
	  })
	  return rec, err
  }
