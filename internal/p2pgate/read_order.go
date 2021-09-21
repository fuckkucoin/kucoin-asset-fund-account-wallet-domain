package p2pgate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func composeOrderSheets(views []HoldView, flows []FlowEntry, q OccupancyQuery) []OrderSheet {
	   type bucket struct {
		  sheet OrderSheet
	   }
	   by := map[string]*bucket{}
	  touch := func(id, asset string) *bucket {
		  id = strings.TrimSpace(id)
		  if id == "" {
			  return nil
		  }
		   b := by[id]
		  if b == nil {
			   b = &bucket{sheet: OrderSheet{OrderID: id, Asset: asset, Flows: []FlowEntry{}}}
			   by[id] = b
		  }
		  if b.sheet.Asset == "" && asset != "" {
			  b.sheet.Asset = asset
		  }
		  return b
	   }
	   for _, v := range views {
		  if q.Asset != "" && !strings.EqualFold(v.Asset, q.Asset) {
			  continue
		  }
		   if q.Ref != "" && v.Ref != q.Ref {
			  continue
		   }
		  b := touch(v.Ref, v.Asset)
		   if b == nil {
			  continue
		   }
		  cp := v
		  switch v.Reason {
		   case holdcat.ReasonP2PSell:
			   b.sheet.Sell = &cp
		   case holdcat.ReasonP2PMargin:
			  b.sheet.Merchant = &cp
		  }
	   }
	   for _, e := range flows {
		  if e.Adjacent {
			   continue
		  }
		   if q.Asset != "" && !strings.EqualFold(e.Asset, q.Asset) {
			  continue
		   }
		  oid := e.OrderID
		   if oid == "" {
			  continue
		  }
		   if q.Ref != "" && oid != q.Ref {
			  continue
		  }
		  b := touch(oid, e.Asset)
		   if b == nil {
			   continue
		   }
		  b.sheet.Flows = append(b.sheet.Flows, e)
		  if e.Action == ActionBuy {
			  cp := e
			   b.sheet.Buy = &cp
		  }
	   }
	   out := make([]OrderSheet, 0, len(by))
	  for _, b := range by {
		  sortFlowEntries(b.sheet.Flows, true)
		   out = append(out, b.sheet)
	   }
	   sortOrderSheets(out, q.SortAsc)
	   return out
   }

// OrderSheets 全量订单拼版。
  func (f *Funds) OrderSheets(ctx context.Context, q OccupancyQuery) ([]OrderSheet, error) {
	  q.prepare()
	   uid,
		  err := validate.UserID(q.UserID)
	  if err != nil {
		   return nil, err
	   }
	  views, idx, err := f.loadHoldViews(ctx, uid)
	  if err != nil {
		  return nil, err
	  }
	  return composeOrderSheets(views, idx.flows, q), nil
   }

// GetOrderSheet 单订单拼版。
  func (f *Funds) GetOrderSheet(ctx context.Context, userID, orderID string) (OrderSheet, error) {
	  orderID = strings.TrimSpace(orderID)
	  if orderID == "" {
		   return OrderSheet{}, ErrOrder
	  }
	  q := OccupancyQuery{UserID: userID, Ref: orderID, Status: StatusAll}
	  sheets, err := f.OrderSheets(ctx, q)
	   if err != nil {
		  return OrderSheet{}, err
	   }
	   for _, s := range sheets {
		   if s.OrderID == orderID {
			   return s, nil
		  }
	  }
	  return OrderSheet{OrderID: orderID, Flows: []FlowEntry{}}, nil
   }
