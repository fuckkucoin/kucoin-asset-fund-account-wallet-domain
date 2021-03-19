// 用法：
// clearing-close -day 2024-01-15 -actor sys-daycut
// 1. 检查指定营业日是否为假日；
// 2. 假日直接登记 HolidayCatchUp 并退出；
// 3. 非假日：open → freeze → （扫描在途）→ close；
// 4. 打印最终 DayClose 状态 JSON。
// 不调用 recon 分类器，不改三方对账公式。

// Author: EliotVoss
// Date: 2021.03.18

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
	"account-assets.kucoin.net/funding/internal/accrual"
	"account-assets.kucoin.net/funding/internal/clearing"
)

  func main() {
	  var day = flag.String("day", "", "营业日（YYYY-MM-DD），默认为今日 UTC")
	  var actor = flag.String("actor", "sys-daycut", "触发日切的系统 actor")
	  var holidayNext = flag.String("holiday-next", "", "假日时的下一工作日（YYYY-MM-DD），留空则自动推算")
	  flag.Parse()
	   if *day == "" {
		  *day = time.Now().UTC().Format("2006-01-02")
	  }
	  cal := clearing.NewCalendar()
	  dcReg := clearing.NewDayCloseRegistry(cal)
	  suspBook := clearing.NewSuspenseBook()
	   engine := accrual.NewEngine()
	   cfg := accrual.DefaultWorkerConfig()
	   cfg.SuspenseEscalate = func(e clearing.SuspenseEntry) {
		   fmt.Fprintf(os.Stderr, "[ESCALATE] suspense_id=%s asset=%s amount=%s\n",
			  e.ID, e.Asset, e.Amount)
	   }
	   w := accrual.NewWorker(cfg,
		  engine, dcReg, suspBook)
	  dc, bat, err := w.RunDayCut(*day, *actor, nil, "")
	  if err != nil {
		  if err == clearing.ErrDayCloseHoliday {
			   // 假日：调用 HolidayCatchUp
			   nextBiz := *holidayNext
			  if nextBiz == "" {
				  t, _ := time.Parse("2006-01-02", *day)
				  next := t.Add(24 * time.Hour)
				   for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
					  next = next.Add(24 * time.Hour)
				  }
				  nextBiz = next.Format("2006-01-02")
			  }
			  dc, err = dcReg.OpenHolidayCatchUp(*day, nextBiz, *actor)
			   if err != nil {
				  fmt.Fprintf(os.Stderr, "holiday-catchup error: %v\n", err)
				   os.Exit(1)
			  }
			  printJSON("day_close", dc)
			   fmt.Fprintln(os.Stdout, "holiday skipped, catch_up_day="+dc.CatchUpDay)
			  os.Exit(0)
		  }
		   fmt.Fprintf(os.Stderr, "day-cut error: %v\n", err)
		  os.Exit(1)
	   }
	   printJSON("day_close", dc)
	   if bat.ID != "" {
		   printJSON("accrual_batch", bat)
	   } else {
		  fmt.Fprintln(os.Stdout, "no cross-day items; accrual batch skipped")
	  }
   }

   func printJSON(label string, v any) {
	  b, _ := json.MarshalIndent(v, "", "  ")
	  fmt.Printf("%s:\n%s\n", label, string(b))
  }
