package subkey

import "strings"

// PolicyDecision 权限组合判定结果。

type PolicyDecision struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason,omitempty"`
	RequiredIP bool   `json:"requiredIpWhitelist"`
	SuggestPerms []string `json:"suggestPerms,omitempty"`
}

// EvaluatePermissionPolicy 创建/修改前策略闸（fail-closed 组合）。
   func EvaluatePermissionPolicy(perm string, ipWhitelist string) PolicyDecision {
	   bits, err := ParsePermission(perm)
	  if err != nil {
		  return PolicyDecision{Allowed: false, Reason: err.Error()}
	   }
	   dec := PolicyDecision{Allowed: true}
	   if bits&(bitPermWithdraw|bitPermTransfer) != 0 && strings.TrimSpace(ipWhitelist) == "" {
		  dec.RequiredIP = true
		  dec.SuggestPerms = []string{"建议为提现/划转权限配置 IP 白名单"}
	  }
	   if bits&bitPermWithdraw != 0 && bits&bitPermSubManage != 0 {
		  return PolicyDecision{Allowed: false, Reason: "Withdraw 与 SubManage 不可同时授予子户 Key"}
	   }
	   if bits&bitPermFutures != 0 && bits&bitPermMargin == 0 && bits&bitPermTrade == 0 {
		   dec.SuggestPerms = append(dec.SuggestPerms, "Futures 通常搭配 Trade 或 Margin")
	  }
	   return dec
   }

// ComposeDefaultPermission 按子户类型返回默认权限串。

  func ComposeDefaultPermission(subType string) string {
	   switch strings.ToLower(strings.TrimSpace(subType)) {
	   case "trade", "trading":
		   return "General,Trade"
	  case "withdraw":
		   return "General,Withdraw"
	  case "readonly", "read":
		   return "General"
	   default:
		  return "General"
	   }
   }

// PolicyMatrixRow 策略矩阵行（运维读表）。
type PolicyMatrixRow struct {
	Name       string `json:"name"`
	Permission string `json:"permission"`
	NeedIP     bool   `json:"needIp"`
	Description string `json:"description"`
}

// PolicyMatrix 返回常见组合矩阵。
   func PolicyMatrix() []PolicyMatrixRow {
	  return []PolicyMatrixRow{
		  {Name: "read_only", Permission: "General", NeedIP: false, Description: "只读资产"},
		  {Name: "spot_trade", Permission: "General,Trade", NeedIP: false, Description: "现货交易"},
		  {Name: "withdraw_only", Permission: "General,Withdraw", NeedIP: true, Description: "仅提现"},
		  {Name: "full_trade", Permission: "General,Trade,Margin,Futures", NeedIP: true, Description: "杠杆+合约"},
		  {Name: "earn_ops", Permission: "General,Earn", NeedIP: false, Description: "理财申购赎回"},
		   {Name: "transfer_ops", Permission: "General,Transfer", NeedIP: true, Description: "账户划转"},
	  }
   }

// MatchPolicyMatrix 按权限串匹配矩阵名。

  func MatchPolicyMatrix(perm string) string {
	   norm,
		  err := ParsePermission(perm)
	  if err != nil {
		  return ""
	  }
	   formatted := FormatPermission(norm)
	  for _, row := range PolicyMatrix() {
		  if strings.EqualFold(formatted, row.Permission) {
			   return row.Name
		   }
	   }
	   return "custom"
   }
