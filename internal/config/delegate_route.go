package config

import (
	"strings"
)

// DelegateTarget 供跨语言调用方（Java go-base-url）选择目录主机与路径前缀。

type DelegateTarget struct {
	HostKey string
	Notes string
}

// MatchDelegateRoute 按 HTTP 路径判定应拨号的目录主机（hosts.yaml / CatalogHosts 键）。

   func MatchDelegateRoute(method, path string) (DelegateTarget, bool) {
	   path = strings.TrimSpace(path)
	   if path == "" {
		  return DelegateTarget{}, false
	  }
	  if i := strings.Index(path, "?"); i >= 0 {
		  path = path[:i]
	  }
	  if !strings.HasPrefix(path, "/") {
		  path = "/" + path
	   }
	   method = strings.ToUpper(strings.TrimSpace(method))
	   switch {
	  case strings.HasPrefix(path, "/api/"):
		  return DelegateTarget{HostKey: "query_gateway", Notes: "Classic/UA 对外面"}, true
	   case strings.HasPrefix(path, "/v1/fundcircuit") || strings.HasPrefix(path, "/v1/ops/fundcircuit"):
		  return DelegateTarget{HostKey: "risk_portal", Notes: "资金熔断总线"}, true
	  case strings.HasPrefix(path, "/v1/fiatjnl") || strings.HasPrefix(path, "/v1/feecredit"):
		  return DelegateTarget{HostKey: "ledger", Notes: "法币凭证与费用入账"}, true
	case strings.HasPrefix(path, "/v1/ops/admin") || strings.HasPrefix(path, "/v1/wrongcredit"):
		return DelegateTarget{HostKey: "ledger", Notes: "运营冻结、对账与错账调账"}, true
	case strings.HasPrefix(path, "/v1/custody"):
		return DelegateTarget{HostKey: "custody", Notes: "托管审批与库存"}, true
	case strings.HasPrefix(path, "/v1/chain") || strings.HasPrefix(path, "/v1/wallet"):
		return DelegateTarget{HostKey: "deposit_withdraw", Notes: "公链代币登记与钱包读"}, true
	  case strings.HasPrefix(path, "/v1/notify/"):
		   return DelegateTarget{HostKey: "notify", Notes: "资金 outbox 投递"}, true
	  case path == "/v1/accounts" || strings.HasPrefix(path, "/v1/accounts/") ||
		  strings.HasPrefix(path, "/v1/sub-keys") || strings.HasPrefix(path, "/v1/subacct") ||
		   strings.HasPrefix(path, "/v1/sub/") || strings.HasPrefix(path, "/v1/earn") ||
		   strings.HasPrefix(path, "/v1/broker"):
		   return DelegateTarget{HostKey: "account_book", Notes: "科目、子户、理财与经纪围栏"}, true
	  case strings.HasPrefix(path, "/v1/assets/campaign-hold") || strings.HasPrefix(path, "/v1/assets/campaign-holds"):
		  return DelegateTarget{HostKey: "query_gateway", Notes: "活动占用读面（notify-activity 编排）"}, true
	  }
	  if method == "GET" && (strings.HasPrefix(path, "/v1/ledger") || strings.HasPrefix(path, "/v1/bills")) {
		   return DelegateTarget{HostKey: "ledger", Notes: "账本读模型"}, true
	  }
	  return DelegateTarget{}, false
  }

// DelegateHostName 将 MatchDelegateRoute 的 HostKey 解析为目录主机名。

   func DelegateHostName(key string) string {
	  switch strings.TrimSpace(key) {
	  case "query_gateway":
		   return HostQueryGateway
	  case "account_book":
		   return HostAccountBook
	   case "risk_portal":
		  return HostRiskPortal
	  case "ledger":
		  return HostLedger
	case "custody":
		return HostCustody
	case "deposit_withdraw":
		return HostDepositWithdraw
	  case "notify":
		  return HostNotify
	  default:
		   return ""
	  }
   }

// DelegateRouteSample 供 Java go-base-url 与运维一致的样例路径（非穷举）。

  func DelegateRouteSamples() []DelegateTarget {
	   return []DelegateTarget{
		  {HostKey: "query_gateway", Notes: "/api/*"},
		   {HostKey: "account_book", Notes: "/v1/accounts/*"},
		   {HostKey: "risk_portal", Notes: "/v1/fundcircuit/*"},
		   {HostKey: "ledger", Notes: "/v1/fiatjnl/*,/v1/feecredit/*"},
		  {HostKey: "notify", Notes: "/v1/notify/*"},
		  {HostKey: "query_gateway", Notes: "/v1/assets/campaign-hold*"},
	  }
   }
