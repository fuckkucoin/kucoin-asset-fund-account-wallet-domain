// @FileName: main.go
// @Summary: 资金物化投影进程：挂读围栏并按秒追平出箱。
// @Author: Blaise
// @Created: 2021.06.02

package main

import (
	"context"
	"database/sql"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundread"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	  svcboot.Main("fund-mat", config.HostFundSurface, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  var db *sql.DB
		   dialect := persist.MySQL
		   var led = rt.Ledger
		   if rt.SQL != nil {
			   db = rt.SQL.DB
			   dialect = rt.SQL.Dialect
		  }
		   svc := fundread.NewWired(db, dialect, led)
		  opt := callerid.Opt{}
		  if rt.Cfg != nil {
			  opt = callerid.FromRuntime(rt.Mem,
				  rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
		   }
		  fundread.Mount(v1, svc, opt)
		   go svc.Run(context.Background(), time.Second)
	  })
   }
