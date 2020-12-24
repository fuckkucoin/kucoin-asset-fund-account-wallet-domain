package custodyhalt

import "strings"

// EpochScopeView 只读作用域投影，供 halt_epoch 绑定。
type EpochScopeView struct {
	Scope Scope `json:"scope"`
	Chain string `json:"chain,omitempty"`
	Asset string `json:"asset,omitempty"`
	Key string `json:"key"`
}

// ViewEpochScope 把 Lane 入口参数投影为作用域键；不读写 KV 标志。

  func ViewEpochScope(scope Scope, chain, asset string) EpochScopeView {
	  if scope == "" {
		  scope = ScopeGlobal
	   }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   key, err := scopeKey(scope, chain, asset)
	  if err != nil {
		  key = KeyGlobal
		   scope = ScopeGlobal
	  }
	  return EpochScopeView{Scope: scope,
		  Chain: chain, Asset: asset, Key: key}
   }

// LaneEpochHint 入口类别字符串，供他包只读一致。
   func LaneEpochHint(lane Lane) string {
	   switch lane {
	  case LaneWithdraw:
		  return "withdraw"
	  case LaneSweep:
		  return "sweep"
	   default:
		   return "deposit"
	  }
  }
