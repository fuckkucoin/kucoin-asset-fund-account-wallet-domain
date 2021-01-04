// account 服务部署兼容入口（等价 cmd/account）。

// Author: Knox
// Date: 2021.01.03

package main

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/bookbill"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/subkey"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("account-service", config.HostAccountBook, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   account.Register(v1, rt)
		  subacct.Register(v1, rt)
		  bookbill.Register(v1, rt)
		  mountSubKeys(v1, rt)
	   })
  }

  func mountSubKeys(v1 *gin.RouterGroup, rt *app.Runtime) {
	  store := subkey.New(nil)
	   opt := callerid.FromRuntime(false, "", "", "")
	   if rt != nil {
		  if rt.SubKeys != nil {
			   store = rt.SubKeys
		   }
		   if rt.Cfg != nil {
			  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
		  }
	  }
	   subkey.RegisterHTTP(v1,
		  subkey.HTTPMount{Store: store, Opt: opt})
  }
