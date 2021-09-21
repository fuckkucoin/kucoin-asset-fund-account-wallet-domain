package p2pgate

import (
	"context"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func merchantFrom(v HoldView) MerchantRow {
	   remaining := "0"
	   if v.Open {
		   remaining = v.Amount
	   }
	  return MerchantRow{HoldView: v, Remaining: remaining}
   }

  func merchantViews(views []HoldView) []HoldView {
	   out := make([]HoldView, 0, len(views))
	   for _, v := range views {
		   if v.Reason == holdcat.ReasonP2PMargin || v.Kind == KindMerchant {
			  out = append(out, v)
		   }
	   }
	   return out
   }

// ListMerchant 商家占用明细分页。

  func (f *Funds) ListMerchant(ctx context.Context, q OccupancyQuery) (MerchantPage, error) {
	   q.prepare()
	   if _, err := validate.UserID(q.UserID); err != nil {
		  return MerchantPage{}, err
	   }
	   q.Reason = holdcat.ReasonP2PMargin
	   views, err := f.ListHoldViews(ctx, q)
	  if err != nil {
		   return MerchantPage{}, err
	   }
	  rows := make([]MerchantRow, 0, len(views))
	  for _, v := range views {
		  rows = append(rows, merchantFrom(v))
	  }
	  sortMerchantRows(rows, q.SortAsc)
	  items, meta := slicePage(rows, q.Page, q.PageSize)
	  if items == nil {
		  items = []MerchantRow{}
	   }
	  return MerchantPage{Items: items, Page: meta}, nil
  }

// MerchantOccupancy 商家占用汇总（含 by_ref）。

  func (f *Funds) MerchantOccupancy(ctx context.Context, q OccupancyQuery) (MerchantSummary, error) {
	   q.prepare()
	  uid, err := validate.UserID(q.UserID)
	  if err != nil {
		  return MerchantSummary{}, err
	  }
	   q.UserID = uid
	   q.Reason = holdcat.ReasonP2PMargin
	   q.Page, q.PageSize = 1, MaxPageSize
	  views, err := f.ListHoldViews(ctx, q)
	   if err != nil {
		   return MerchantSummary{}, err
	   }
	  return composeMerchantSummary(uid, views), nil
  }

   func composeMerchantSummary(userID string, views []HoldView) MerchantSummary {
	  sum := MerchantSummary{
		   UserID:  userID,
		   ByAsset: map[string]string{},
		  ByRef:   []MerchantRefAgg{},
	   }
	   type key struct{ ref, asset string }
	   aggs := map[key]*MerchantRefAgg{}
	   var openAcc, relAcc decAcc
	  for _, v := range merchantViews(views) {
		   k := key{ref: v.Ref,
			   asset: v.Asset}
		   agg := aggs[k]
		   if agg == nil {
			  agg = &MerchantRefAgg{Ref: v.Ref, Asset: v.Asset}
			  aggs[k] = agg
		   }
		   if v.Open {
			  sum.OpenCount++
			  openAcc.add(v.Amount)
			   addAmountMap(sum.ByAsset, v.Asset, v.Amount)
			   agg.OpenCount++
			  var a decAcc
			   a.add(agg.OpenTotal)
			  a.add(v.Amount)
			   agg.OpenTotal = a.string()
		   } else {
			  sum.ReleasedCount++
			  relAcc.add(v.Amount)
			  agg.ReleasedCount++
			   var a decAcc
			  a.add(agg.ReleasedTotal)
			  a.add(v.Amount)
			  agg.ReleasedTotal = a.string()
		  }
	  }
	  sum.OpenTotal = openAcc.string()
	  sum.ReleasedTotal = relAcc.string()
	   for _, agg := range aggs {
		  if agg.OpenTotal == "" {
			  agg.OpenTotal = "0"
		   }
		  if agg.ReleasedTotal == "" {
			  agg.ReleasedTotal = "0"
		  }
		  sum.ByRef = append(sum.ByRef, *agg)
	   }
	  sort.SliceStable(sum.ByRef, func(i, j int) bool {
		   if sum.ByRef[i].Asset != sum.ByRef[j].Asset {
			  return sum.ByRef[i].Asset < sum.ByRef[j].Asset
		  }
		  return sum.ByRef[i].Ref < sum.ByRef[j].Ref
	  })
	   return sum
   }

// MerchantOpenTotal 开放商家占用合计（单币种可选）。
   func (f *Funds) MerchantOpenTotal(ctx context.Context, userID, asset string) (string, error) {
	  q := OccupancyQuery{UserID: userID, Asset: strings.ToUpper(strings.TrimSpace(asset)), Reason: holdcat.ReasonP2PMargin, Status: StatusOpen}
	  views, err := f.ListHoldViews(ctx, q)
	  if err != nil {
		   return "0", err
	  }
	   var acc decAcc
	  for _, v := range views {
		  acc.add(v.Amount)
	  }
	   return acc.string(), nil
   }
