package hotinv

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	GateAllowSweep     = "allow_sweep"
	GateAllowReplenish = "allow_replenish"
	GateDenyInBand     = "deny_in_band"
	GateDenyUnreadable = "deny_unreadable"
	GateDenyUnknown = "deny_unknown_asset"
)

type GateBand struct {
	HotLow   string
	HotHigh  string
	WarmLow  string
	WarmHigh string
}

type GateVerdict struct {
	Asset string `json:"asset"`
	Code string `json:"code"`
	Allow bool   `json:"allow"`
	Hot   string `json:"hot,omitempty"`
	Warm  string `json:"warm,omitempty"`
	Cold  string `json:"cold,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// JudgeBalances 纯判断。解析失败按不可读拒绝，不把残缺数字当成带内。

  func JudgeBalances(asset, hot, warm, cold string, band GateBand) GateVerdict {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  v := GateVerdict{Asset: asset, Hot: hot, Warm: warm, Cold: cold}
	   if strings.TrimSpace(hot) == "" || strings.TrimSpace(warm) == "" || strings.TrimSpace(cold) == "" {
		  v.Code = GateDenyUnreadable
		   v.Reason = "balance blank"
		   return v
	  }
	  h, err := money.ParseBook(strings.TrimSpace(hot))
	  if err != nil {
		  v.Code = GateDenyUnreadable
		   v.Reason = "hot unreadable"
		  return v
	   }
	  w, err := money.ParseBook(strings.TrimSpace(warm))
	   if err != nil {
		   v.Code = GateDenyUnreadable
		   v.Reason = "warm unreadable"
		   return v
	  }
	  if _, err := money.ParseBook(strings.TrimSpace(cold)); err != nil {
		  v.Code = GateDenyUnreadable
		  v.Reason = "cold unreadable"
		  return v
	  }
	  hotLow, err := money.Parse(band.HotLow)
	  if err != nil {
		   v.Code = GateDenyUnreadable
		  v.Reason = "band unreadable"
		  return v
	   }
	   hotHigh, err := money.Parse(band.HotHigh)
	   if err != nil {
		   v.Code = GateDenyUnreadable
		   v.Reason = "band unreadable"
		  return v
	  }
	   warmHigh, err := money.Parse(band.WarmHigh)
	  if err != nil {
		   v.Code = GateDenyUnreadable
		  v.Reason = "band unreadable"
		   return v
	  }
	   if _, err := money.Parse(band.WarmLow); err != nil {
		  v.Code = GateDenyUnreadable
		  v.Reason = "band unreadable"
		   return v
	  }
	   if h.Cmp(hotHigh) > 0 || w.Cmp(warmHigh) > 0 {
		  v.Code = GateAllowSweep
		   v.Allow = true
		   v.Reason = "above high"
		   return v
	  }
	   if h.Cmp(hotLow) < 0 {
		  v.Code = GateAllowReplenish
		  v.Allow = true
		   v.Reason = "below low"
		  return v
	   }
	  v.Code = GateDenyInBand
	  v.Reason = "in band"
	   return v
   }

// ReadGate 从账本观察后判断。观察失败失败即关，不沿用上次快照。

  func (b *Book) ReadGate(ctx context.Context, st ledger.Store, asset string, band GateBand) (GateVerdict, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   if b == nil || st == nil {
		   return GateVerdict{Asset: asset, Code: GateDenyUnreadable, Reason: "store unavailable"}, errGateStore
	  }
	   rows, err := b.Observe(ctx, st)
	  if err != nil {
		  return GateVerdict{Asset: asset, Code: GateDenyUnreadable, Reason: "observe failed"}, err
	   }
	   for _, row := range rows {
		  if row.Asset != asset {
			  continue
		   }
		   return JudgeBalances(asset,
			   row.HotAvail, row.WarmAvail, row.ColdAvail, band), nil
	  }
	   return GateVerdict{Asset: asset, Code: GateDenyUnknown, Reason: "asset not in inventory"}, nil
  }
