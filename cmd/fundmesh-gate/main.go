// 重点：取消双入口与观察组 ping；偏好读面仍走 query-gateway。
// 风险点：与账本主进程分部署，仅共享配置与远端取消面。

// Author: Cason
// Date: 2021.06.10

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundmesh"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	  svcboot.MainEngine("fundmesh-gate", config.HostQueryGateway, registerMesh, registerObserve)
   }

  func registerMesh(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if v1 == nil || rt == nil {
		   return
	  }
	  profile := ""
	   if rt.Cfg != nil {
		   profile = rt.Cfg.Profile
	   }
	  id := callerid.Opt{}
	  if rt.Cfg != nil {
		   id = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	   }
	  fundmesh.AttachCancelWire(v1, rt.Ledger, rt.CfgWatch, rt.Dial, profile, id)
  }

  func registerObserve(r *gin.Engine, _ *app.Runtime) {
	   fundmesh.RegisterObserveRoot(r)
   }
