package p2pgate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/validate"
)

// OccupancySnapshot 资金页一次拉取的占用块。

type OccupancySnapshot struct {
	Summary OccupancySummary `json:"summary"`
	Cooling CoolingView      `json:"cooling"`
	Holds    []HoldView    `json:"holds"`
	Merchant []MerchantRow `json:"merchant"`
	Flow      []FlowEntry     `json:"flow"`
	Integrity IntegrityReport `json:"integrity"`
}

// Snapshot 组装占用快照。holds/merchant/flow 各截最近 20 条（最新在前）。
   func (f *Funds) Snapshot(ctx context.Context, userID string) (OccupancySnapshot, error) {
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return OccupancySnapshot{}, err
	   }
	  sum, err := f.OccupancySummaryOf(ctx, uid)
	  if err != nil {
		  return OccupancySnapshot{}, err
	   }
	  cool, err := f.CoolingState(ctx, uid)
	  if err != nil {
		   return OccupancySnapshot{}, err
	  }
	   rep, err := f.Integrity(ctx, uid)
	   if err != nil {
		   return OccupancySnapshot{}, err
	  }
	  holds, err := f.ListHoldViews(ctx, OccupancyQuery{UserID: uid, Status: StatusOpen})
	   if err != nil {
		  return OccupancySnapshot{}, err
	  }
	  sortHoldViews(holds, false)
	   holds = clipHolds(holds, 20)
	   merchPage, err := f.ListMerchant(ctx, OccupancyQuery{UserID: uid, Status: StatusOpen, Page: 1, PageSize: 20})
	   if err != nil {
		  return OccupancySnapshot{}, err
	  }
	   flowPage, err := f.ListFlow(ctx, OccupancyQuery{UserID: uid, Page: 1, PageSize: 20})
	   if err != nil {
		   return OccupancySnapshot{}, err
	  }
	  return OccupancySnapshot{
		   Summary: sum, Cooling: cool, Holds: holds,
		  Merchant: merchPage.Items, Flow: flowPage.Items, Integrity: rep,
	  }, nil
  }

   func clipHolds(items []HoldView, n int) []HoldView {
	  if n <= 0 || len(items) <= n {
		  return items
	   }
	   return items[:n]
  }

// SnapshotREST 公开 REST 快照字段。
  func SnapshotREST(s OccupancySnapshot) map[string]any {
	   return map[string]any{
		  "summary": SummaryToREST(s.Summary),
		  "cooling": s.Cooling,
		  "holds":    HoldsToREST(s.Holds),
		   "merchant": MerchantsToREST(s.Merchant),
		   "flow":     FlowsToREST(s.Flow),
		  "integrity": s.Integrity,
	  }
   }
