// 重点：不重挂偏好读面（query-gateway 已有 gateway/pref_mesh）。
// 风险点：缺配置源必须 fail-closed，不得补齐全 Mem 配置。

// Author: Talon
// Date: 2021.06.14

package fundmesh

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/cfgwatch"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/svcdial"
	"github.com/gin-gonic/gin"
)

  func partsOf(ledgerSvc *ledger.Service, watch cfgwatch.Store, dial *svcdial.Client, profile string) RuntimeParts {
	   var d Dialer
	  if dial != nil {
		  d = dial
	  }
	  return RuntimeParts{
		   Ledger: ledgerSvc,
		  Watch: watch,
		  Dial: d,
		  MemProfile: strings.EqualFold(strings.TrimSpace(profile), "mem"),
	  }
   }

// AttachCancelWire 给既有 query-gateway 偏好面复用同一取消引擎。

   func AttachCancelWire(v1 *gin.RouterGroup, ledgerSvc *ledger.Service, watch cfgwatch.Store, dial *svcdial.Client, profile string, id callerid.Opt) {
	   if v1 == nil {
		  return
	   }
	  eng := BuildEngine(partsOf(ledgerSvc, watch, dial, profile))
	  RegisterWire(v1, eng, id)
  }
