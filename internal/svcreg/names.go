package svcreg

import "strings"

// 注册名与 cmd 行业进程名一致；兼容别名见文档 aa-microservice-map。
const (
	NameAccount = "account"
	NameLedger = "ledger"
	NameDeposit = "deposit"
	NameWithdraw = "withdraw"
	NameAddress  = "address"
	NameCustody = "custody"
	NameKYT     = "kyt"
	NameGateway = "gateway"
	NameAccountService  = "account-service"
	NameLedgerService   = "ledger-service"
	NameDepositWithdraw = "deposit-withdraw"
	NameWithdrawService = "withdraw-service"
	NameAddressService = "address-service"
	NameCustodyService = "custody-service"
	NameKYTService     = "kyt-service"
	NameQueryGateway = "query-gateway"
)

// AliasNames 行业进程名与历史 *-service 名互为发现别名。请求名先。
// 网关按 ledger-service 查找时，行业进程注册的 ledger 仍能被选中。
   func AliasNames(name string) []string {
	  name = strings.TrimSpace(name)
	   if name == "" {
		  return nil
	   }
	   groups := [][]string{
		   {"ledger", "ledger-service"},
		   {"account", "account-service"},
		  {"address", "address-service"},
		   {"custody", "custody-service"},
		  {"kyt", "kyt-service"},
		   {"deposit-withdraw", "deposit", "withdraw", "withdraw-service"},
		  {"gateway", "query-gateway"},
		   {"stmt-export", "statement-export"},
		   {"funding-bff", "funding-bff-service"},
		   {"p2p-funding", "p2p-abandon"},
	  }
	   for _, g := range groups {
		   hit := false
		   for _, n := range g {
			  if n == name {
				  hit = true
				   break
			  }
		  }
		  if !hit {
			   continue
		  }
		  out := []string{name}
		   for _, n := range g {
			  if n != name {
				   out = append(out, n)
			   }
		   }
		   return out
	   }
	  return []string{name}
   }
