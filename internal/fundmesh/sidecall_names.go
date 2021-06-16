package fundmesh

import "sync"

   func RegisteredSideCalls() []SideCallSpec {
	  return []SideCallSpec{
		  {Name: "address-custody", Service: "address-custody", Method: "GET", Path: "/v1/addresses/deposit", Write: false},
		   {Name: "risk-kyt", Service: "risk-kyt", Method: "POST", Path: "/v1/kyt/screen", Write: false},
		  {Name: "notify-withdraw-cancel", Service: "deposit-withdraw", Method: "POST", Path: "/v1/notify/enqueue", Write: false},
		   {Name: "travel-vasp-read", Service: "travel-vasp", Method: "GET", Path: "/v1/travel/peers", Write: false},
		  {Name: "destallow-read", Service: "destallow", Method: "GET", Path: "/v1/withdrawals/destallow", Write: false},
		   {Name: "quota-read", Service: "withdraw-quota", Method: "GET", Path: "/v1/withdrawals/quotas", Write: false},
		  {Name: "addrbook-read", Service: "address-book", Method: "GET", Path: "/v1/withdrawals/addresses", Write: false},
		   {Name: "observe-chain-read", Service: "chain-observe", Method: "GET", Path: "/v1/chain/observe", Write: false},
		  {Name: "ledger-transfer-get", Service: "ledger", Method: "GET", Path: "/v1/transfers", Write: false},
		  {Name: "hotbound-observe", Service: "hotbound", Method: "GET", Path: "/v1/hotbound/observe", Write: false},
		   {Name: "outbox-lag-read", Service: "outbox-pump", Method: "GET", Path: "/v1/outbox/lag", Write: false},
		   {Name: "cfg-center-read", Service: "cfg-center", Method: "GET", Path: "/v1/cfg/snapshot", Write: false},
	   }
   }

var (
	catalogOnce sync.Once
	catalogMap map[string]SideCallSpec
)

  func catalogIndex() map[string]SideCallSpec {
	   catalogOnce.Do(func() {
		   catalogMap = make(map[string]SideCallSpec, 16)
		  for _, s := range RegisteredSideCalls() {
			  catalogMap[NormalizeSideCallName(s.Name)] = s
		  }
	   })
	   return catalogMap
   }

  func SideCallNames() []string {
	  rows := RegisteredSideCalls()
	  out := make([]string, 0, len(rows))
	   for _, s := range rows {
		  out = append(out, s.Name)
	   }
	  return out
   }
