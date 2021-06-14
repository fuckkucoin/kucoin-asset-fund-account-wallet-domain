// 功能：关键写所需配置键目录。
// 重点：缺任一键即漂移，禁止提现写。
// 风险点：键名与配置中心前缀必须一致。

// 作者：jakson
// 日期：2021.06.13

package fundmesh

const ConfigPrefix = "/aa/cfg/account-assets/production/"

  func RequiredCriticalKeys() []string {
	  return []string{
		   "fundmesh/critical_write_enabled",
		   "fundmesh/remote_ledger_required",
		   "fundmesh/silent_local_ledger",
		  "fundmesh/cancel_idem_ttl",
		  "ledger/endpoint",
		  "ledger/fail_closed",
		  "withdraw/cancel_enabled",
		   "withdraw/broadcast_cancel_denied",
		  "etcd/namespace",
		  "etcd/env",
		   "observe/ping_readonly",
		   "sidecall/unknown_fail_closed",
	  }
   }

   func RequiredKeyPath(key string) string {
	   if len(key) > 0 && key[0] == '/' {
		  return key
	  }
	  return ConfigPrefix + key
  }

  func ExpectedCriticalValues() map[string]string {
	  return map[string]string{
		   "fundmesh/silent_local_ledger":    "false",
		  "fundmesh/remote_ledger_required": "true",
		  "fundmesh/critical_write_enabled": "true",
		  "ledger/fail_closed":              "true",
		   "withdraw/broadcast_cancel_denied": "true",
		  "observe/ping_readonly":            "true",
		   "sidecall/unknown_fail_closed": "true",
	  }
  }
