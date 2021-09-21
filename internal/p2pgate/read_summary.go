package p2pgate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/validate"
)

// OccupancySummaryOf 组装资金页摘要。日记不可用时 IntegrityOK=false，合计仍以标签为准。
   func (f *Funds) OccupancySummaryOf(ctx context.Context, userID string) (OccupancySummary, error) {
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return OccupancySummary{}, err
	  }
	   views, idx, err := f.loadHoldViews(ctx, uid)
	  if err != nil {
		  return OccupancySummary{}, err
	   }
	   sum := OccupancySummary{
		  UserID:  uid,
		   ByAsset: map[string]string{},
	  }
	  var frozen, sell, merchant decAcc
	  for _, v := range views {
		   if v.Open {
			   if _, ok := parseDec(v.Amount); !ok {
				  return OccupancySummary{},
					   ErrQueryAsset
			  }
			  sum.OpenHolds++
			  frozen.add(v.Amount)
			   addAmountMap(sum.ByAsset, v.Asset, v.Amount)
			   switch v.Reason {
			  case holdcat.ReasonP2PSell:
				  sum.SellOpen++
				   sell.add(v.Amount)
			   case holdcat.ReasonP2PMargin:
				   sum.MerchantOpen++
				   merchant.add(v.Amount)
			   }
		   } else {
			  sum.ReleasedCount++
		  }
	  }
	   if frozen.failed() || sell.failed() || merchant.failed() {
		  return OccupancySummary{}, ErrQueryAsset
	   }
	   sum.FrozenTotal = frozen.string()
	  sum.SellTotal = sell.string()
	  sum.MerchantTotal = merchant.string()
	  for _, e := range idx.flows {
		   if e.Adjacent {
			  continue
		   }
		  sum.FlowCount++
		   if e.Action == ActionBuy {
			  sum.BuyCount++
		  }
	  }
	   rep := composeIntegrity(uid, views, idx)
	  sum.IntegrityOK = rep.ParkCoverageOK
	  return sum, nil
  }

// SummaryCompat 既有 /summary 所需的最小字段。
  func (s OccupancySummary) Compat() (open int, frozen string, byAsset map[string]string) {
	   if s.ByAsset == nil {
		  s.ByAsset = map[string]string{}
	   }
	  return s.OpenHolds, s.FrozenTotal, s.ByAsset
   }
