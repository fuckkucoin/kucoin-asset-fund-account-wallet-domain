// @FileName: main.go
// @Summary: 审计仓进程。-fixture 跑验收演练，否则挂查询并周期巡检。
// @Author: Likeo
// @Created: 2021.02.10

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/auditchain"
	"account-assets.kucoin.net/funding/internal/auditchain/fixture"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	  fix := flag.Bool("fixture", false, "run audit warehouse drills")
	   flag.Parse()
	  if *fix {
		  ingest.SetMode(ingest.ModeOn)
		  if err := fixture.RunAll(); err != nil {
			   fmt.Fprintf(os.Stderr, "audit-wh: %v\n", err)
			   os.Exit(1)
		   }
		  fmt.Println("audit-wh fixtures ok")
		  return
	   }
	   svcboot.Main("audit-wh", "audit-warehouse.kucoin.net", func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   if rt == nil || rt.SQL == nil || rt.SQL.DB == nil {
			   fmt.Fprintf(os.Stderr, "audit-wh: sql is not open\n")
			   return
		   }
		   st := auditchain.NewSQL(rt.SQL)
		  wh := auditchain.NewWarehouse(st, auditchain.DefaultConfig())
		   ingest.Install(wh)
		  ingest.OnStrict(func() { auditchain.HaltExport(true) })
		   if rt != nil && rt.Ledger != nil {
			  wh.SetReader(auditchain.LedgerBook{Ledger: rt.Ledger})
		  }
		   desk := auditchain.NewDesk(wh, nil)
		  auditchain.RegisterWarehouse(v1, rt, wh, desk)
		   go (&auditchain.Worker{WH: wh}).Loop(context.Background())
	   })
  }
