package p2pgate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/validate"
)

// AggregateByAsset 占用与流水按币种轧差。

   func (f *Funds) AggregateByAsset(ctx context.Context, q OccupancyQuery) ([]AssetAgg, error) {
	   q.prepare()
	  uid, err := validate.UserID(q.UserID)
	   if err != nil {
		   return nil, err
	   }
	  q.UserID = uid
	   views, err := f.ListHoldViews(ctx, q)
	   if err != nil {
		  return nil, err
	   }
	   flows, err := f.ListFlowAll(ctx, q)
	  if err != nil {
		  return nil, err
	   }
	   type acc struct {
		   openC, relC int
		   open, rel     decAcc
		   credit, debit decAcc
	  }
	   by := map[string]*acc{}
	  touch := func(asset string) *acc {
		   a := by[asset]
		  if a == nil {
			  a = &acc{}
			  by[asset] = a
		  }
		  return a
	  }
	   for _, v := range views {
		  a := touch(v.Asset)
		  if v.Open {
			   a.openC++
			   a.open.add(v.Amount)
		   } else {
			   a.relC++
			   a.rel.add(v.Amount)
		   }
	  }
	   for _, e := range flows {
		   if e.Adjacent || !flowCredited(e) {
			   continue
		   }
		   a := touch(e.Asset)
		   switch e.Sign {
		  case SignCredit:
			   a.credit.add(e.Amount)
		   case SignDebit:
			   a.debit.add(e.Amount)
		   }
	  }
	  out := make([]AssetAgg, 0, len(by))
	  for asset, a := range by {
		  out = append(out, AssetAgg{
			  Asset: asset, OpenCount: a.openC, ReleasedCount: a.relC,
			  OpenTotal: a.open.string(), ReleasedTotal: a.rel.string(),
			   FlowCredit: a.credit.string(),
			  FlowDebit: a.debit.string(),
		   })
	   }
	   sortAssetAggs(out)
	  return out, nil
  }

// AggregateByReason 开放占用按原因。

  func (f *Funds) AggregateByReason(ctx context.Context, userID string) ([]ReasonAgg, error) {
	   if _, err := validate.UserID(userID); err != nil {
		   return nil, err
	   }
	   views, _, err := f.loadHoldViews(ctx, userID)
	  if err != nil {
		  return nil, err
	  }
	   type acc struct {
		   n int
		   a decAcc
	   }
	   by := map[string]*acc{}
	  for _, v := range views {
		  if !v.Open {
			   continue
		   }
		  x := by[v.Reason]
		   if x == nil {
			  x = &acc{}
			   by[v.Reason] = x
		   }
		   x.n++
		   x.a.add(v.Amount)
	  }
	   out := make([]ReasonAgg, 0, 2)
	   for _, reason := range []string{holdcat.ReasonP2PSell, holdcat.ReasonP2PMargin} {
		  x := by[reason]
		   if x == nil {
			  out = append(out, ReasonAgg{Reason: reason, Kind: KindOfReason(reason), OpenTotal: "0"})
			  continue
		   }
		   out = append(out, ReasonAgg{Reason: reason, Kind: KindOfReason(reason), OpenCount: x.n, OpenTotal: x.a.string()})
	   }
	   return out, nil
  }

// ListOrders 以 order_id 拼版的占用单据。
  func (f *Funds) ListOrders(ctx context.Context, q OccupancyQuery) (OrderPage, error) {
	  q.prepare()
	   uid, err := validate.UserID(q.UserID)
	   if err != nil {
		  return OrderPage{}, err
	  }
	  q.UserID = uid
	  sheets, err := f.OrderSheets(ctx, q)
	   if err != nil {
		  return OrderPage{}, err
	  }
	   items, meta := slicePage(sheets, q.Page, q.PageSize)
	   if items == nil {
		  items = []OrderSheet{}
	  }
	  return OrderPage{Items: items, Page: meta}, nil
   }
