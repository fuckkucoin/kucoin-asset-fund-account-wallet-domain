package addrpool

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/wallet"
)

const defaultTopupWant = 8

// TopupView 主动补给面。
type TopupView struct {
	ShortageView
	Want int `json:"want"`
	Need int `json:"need"`
	Request  bool `json:"topup_request"`
	TierHot  int  `json:"tier_hot_free"`
	TierCold int  `json:"tier_cold_free"`
}

// InspectTopup 读库存并在低于目标时发补给信号。
   func InspectTopup(inv *Inventory, shared *SharedMemoPool, chain string, want int) TopupView {
	  if want <= 0 {
		  want = defaultTopupWant
	   }
	   short := InspectShortage(inv,
		   shared, chain)
	   view := TopupView{ShortageView: short, Want: want}
	  if inv != nil {
		   hot, cold := inv.CountsByTier(chain)
		   view.TierHot, view.TierCold = hot.Free, cold.Free
	  }
	  if wallet.RequiresFamMemo(strings.ToUpper(strings.TrimSpace(chain))) {
		   if short.Short {
			   view.Request = true
			   view.Need = 1
			   SignalTopup(chain, view.Free, want, short.Reason)
		   }
		  return view
	  }
	   if view.Free < want {
		  view.Request = true
		  view.Need = want - view.Free
		  if view.Need < 0 {
			  view.Need = 0
		  }
		   SignalTopup(chain, view.Free, want, "below_target")
	  }
	   return view
   }

// SignalTopup 主动补给埋点。
  func SignalTopup(chain string, free, want int, reason string) {
	   label := strings.TrimSpace(reason)
	  if label == "" {
		  label = "topup"
	  }
	   metrics.FundGates.WithLabelValues("addrpool", "topup").Inc()
	  slo.FireMiddlewareAlertDetail("addrpool.topup", float64(free), float64(want), chain+" "+label)
   }
