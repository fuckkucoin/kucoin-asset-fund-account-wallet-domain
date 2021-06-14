// @FileName: main.go
// @Summary: 资金 mesh 出站幂等围栏独立进程入口。
// 重点：与 fundmesh-gate 共用 AttachCancelWire；挂出站提交与取消双入口。
// 风险点：不得另起进程内面作默认扣减账本。
// @Author: Draven
// @Created: 2021.06.13

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
	   svcboot.MainEngine("fundmesh-outbound", config.HostQueryGateway, registerMesh, registerObserve)
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
