package addrpool

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/slo"
	"account-assets.kucoin.net/funding/internal/wallet"
)

const shortageFreeThresh = 0

// ShortageView 单链或全台短缺面。

type ShortageView struct {
	Chain    string `json:"chain"`
	Free     int    `json:"free"`
	Reserved int    `json:"reserved"`
	Bound int `json:"bound"`
	Cooling int `json:"cooling"`
	FamMissing []string `json:"fam_missing,omitempty"`
	Short      bool     `json:"short"`
	Threshold  int      `json:"threshold"`
	Reason     string   `json:"reason,omitempty"`
}

// InspectShortage 读库存与 FamMEMO 缺口。short 时打点并告警。
  func InspectShortage(inv *Inventory, shared *SharedMemoPool, chain string) ShortageView {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   view := ShortageView{Chain: chain, Threshold: shortageFreeThresh}
	   if inv != nil {
		  c := inv.Counts(chain)
		  view.Free, view.Reserved, view.Bound, view.Cooling = c.Free, c.Reserved, c.Bound, c.Cooling
	   }
	  if shared != nil {
		  if chain == "" {
			  view.FamMissing = shared.MissingFamChains()
		   } else if wallet.RequiresFamMemo(chain) {
			   if _, ok := shared.Address(chain); !ok {
				   view.FamMissing = []string{chain}
			  }
		  }
	  }
	   switch {
	   case len(view.FamMissing) > 0:
		   view.Short = true
		  view.Reason = "fam_shared_unregistered"
	  case inv != nil && view.Free <= shortageFreeThresh && chain != "" && !wallet.RequiresFamMemo(chain):
		   view.Short = true
		   view.Reason = "inventory_empty"
	  }
	  if view.Short {
		   SignalShortage(chain,
			   view.Free, view.Reason)
	   }
	  return view
  }

// SignalShortage 短缺埋点。value=剩余空闲，threshold=0。

  func SignalShortage(chain string, free int, reason string) {
	   label := strings.TrimSpace(reason)
	   if label == "" {
		  label = "shortage"
	  }
	  metrics.FundGates.WithLabelValues("addrpool", label).Inc()
	   slo.FireMiddlewareAlertDetail("addrpool.shortage", float64(free), float64(shortageFreeThresh), chain+" "+label)
  }
