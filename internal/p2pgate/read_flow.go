package p2pgate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

// ListFlow 占用相关流水分页。
  func (f *Funds) ListFlow(ctx context.Context, q OccupancyQuery) (FlowPage, error) {
	   q.prepare()
	   uid, err := validate.UserID(q.UserID)
	  if err != nil {
		   return FlowPage{}, err
	  }
	  idx, err := f.occupancyIndex(ctx, uid)
	   if err != nil {
		  return FlowPage{}, err
	   }
	   matched := filterFlows(idx.flows, q)
	  sortFlowEntries(matched, q.SortAsc)
	  items, meta := slicePage(matched, q.Page, q.PageSize)
	   if items == nil {
		   items = []FlowEntry{}
	  }
	   return FlowPage{Items: items, Page: meta}, nil
  }

// ListFlowAll 不过分页的流水（汇总/导出）。

   func (f *Funds) ListFlowAll(ctx context.Context, q OccupancyQuery) ([]FlowEntry, error) {
	   q.prepare()
	   uid, err := validate.UserID(q.UserID)
	  if err != nil {
		  return nil, err
	  }
	   idx, err := f.occupancyIndex(ctx, uid)
	  if err != nil {
		   return nil, err
	   }
	   matched := filterFlows(idx.flows, q)
	  sortFlowEntries(matched, q.SortAsc)
	  return matched, nil
   }

// FlowByOrder 某订单下的占用流水（买入/卖出/保证金/释放）。

   func (f *Funds) FlowByOrder(ctx context.Context, userID, orderID string) ([]FlowEntry, error) {
	   q := OccupancyQuery{UserID: userID, Ref: strings.TrimSpace(orderID), Status: StatusAll, Adjacent: false}
	  return f.ListFlowAll(ctx, q)
   }

// NetFlow 按币种净额：贷记减借记。仅已入账计入。金额解析失败则整表拒绝，避免少计。
  func NetFlow(entries []FlowEntry) (map[string]string, error) {
	   type pair struct{ credit, debit decAcc }
	  by := map[string]*pair{}
	   for _, e := range entries {
		  if e.Adjacent || !flowCredited(e) {
			  continue
		  }
		  if _, ok := parseDec(e.Amount); !ok {
			  return nil, ErrQueryAsset
		  }
		  p := by[e.Asset]
		   if p == nil {
			   p = &pair{}
			  by[e.Asset] = p
		  }
		   switch e.Sign {
		   case SignCredit:
			   p.credit.add(e.Amount)
		   case SignDebit:
			  p.debit.add(e.Amount)
		   }
	  }
	   out := make(map[string]string, len(by))
	  for asset, p := range by {
		   if p.credit.failed() || p.debit.failed() {
			   return nil, ErrQueryAsset
		   }
		   out[asset] = p.credit.sub(p.debit).string()
	  }
	   return out, nil
   }

   func flowCredited(e FlowEntry) bool {
	   return e.State == "credited" || e.Status == "success"
   }

  func countActions(entries []FlowEntry) map[string]int {
	  out := map[string]int{}
	   for _, e := range entries {
		  if e.Adjacent {
			   continue
		   }
		  out[e.Action]++
	  }
	  return out
   }
