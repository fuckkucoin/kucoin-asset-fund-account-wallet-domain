package custodyhalt

import (
	"context"
	"strings"
)

// Lane 资金账户危险动作入口类别。
type Lane string

const (
	LaneDeposit Lane = "deposit"
	LaneWithdraw Lane = "withdraw"
	LaneSweep    Lane = "sweep"
)

// LaneStatus 单入口闸位读面。充停与提停共用 KV 标志；读失败视为应停。

type LaneStatus struct {
	Lane     Lane         `json:"lane"`
	Blocked  bool         `json:"blocked"`
	GlobalOn bool         `json:"global_on"`
	Active   []ScopedFlag `json:"active,omitempty"`
	Reason string `json:"reason,omitempty"`
	FailClosed bool `json:"fail_closed,omitempty"`
	Host string `json:"host"`
}

// LaneBoard 充停/提停/清扫聚合读面。

type LaneBoard struct {
	Deposit  LaneStatus   `json:"deposit"`
	Withdraw LaneStatus   `json:"withdraw"`
	Sweep    LaneStatus   `json:"sweep"`
	Active   []ScopedFlag `json:"active_scopes"`
	Host string `json:"host"`
}

// CheckLane 资金账户入口闸：清扫走 CheckActive；充/提在可知链资产时走 CheckScope，否则 CheckActive。
// 把作用域熔断接到入金/出金/清扫读判。读失败失败即关；不得在本路径 Clear。链/资产拼写错误会漏匹配作用域键，值班须核对 ActiveFlags。
   func (g *Gate) CheckLane(ctx context.Context, lane Lane, chain, asset string) error {
	   switch lane {
	  case LaneSweep:
		  return g.CheckActive(ctx)
	  case LaneDeposit, LaneWithdraw, "":
		  chain = strings.TrimSpace(chain)
		   asset = strings.TrimSpace(asset)
		  if chain == "" && asset == "" {
			  return g.CheckActive(ctx)
		  }
		   if chain != "" && asset != "" {
			  return g.CheckPair(ctx, chain, asset)
		   }
		   return g.CheckScope(ctx, chain, asset)
	  default:
		   return g.CheckActive(ctx)
	   }
   }

// LaneOf 读单入口闸位。
  func (g *Gate) LaneOf(ctx context.Context, lane Lane, chain, asset string) LaneStatus {
	   st := LaneStatus{Lane: lane, Host: "custody-boundary-lab.kucoin.net"}
	   if lane == "" {
		  lane = LaneDeposit
		   st.Lane = lane
	   }
	  gf, err := g.Snapshot(ctx)
	   if err != nil {
		  st.Blocked = true
		  st.FailClosed = true
		  st.Reason = err.Error()
		  return st
	  }
	  st.GlobalOn = gf.On
	  active, aerr := g.ActiveFlags(ctx)
	  if aerr != nil {
		   st.Blocked = true
		   st.FailClosed = true
		   st.Reason = aerr.Error()
		   return st
	  }
	   st.Active = active
	  if err := g.CheckLane(ctx, lane, chain, asset); err != nil {
		  st.Blocked = true
		  st.Reason = err.Error()
		   return st
	   }
	   return st
  }

// Board 充停/提停/清扫聚合读面。

   func (g *Gate) Board(ctx context.Context, chain, asset string) LaneBoard {
	  active, _ := g.ActiveFlags(ctx)
	  return LaneBoard{
		   Deposit:  g.LaneOf(ctx, LaneDeposit, chain, asset),
		  Withdraw: g.LaneOf(ctx, LaneWithdraw, chain, asset),
		   Sweep:  g.LaneOf(ctx, LaneSweep, "", ""),
		  Active: active,
		  Host:   "custody-boundary-lab.kucoin.net",
	   }
   }
