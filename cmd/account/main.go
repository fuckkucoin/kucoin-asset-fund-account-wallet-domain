// 功能：账户账本进程入口。
// 作者：Bronson
// 日期：2020.09.20

package main

import (
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/bookbill"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("account", config.HostAccountBook, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   account.Register(v1, rt)
		  subacct.Register(v1, rt)
		  bookbill.Register(v1, rt)
	  })
   }
