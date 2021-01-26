// 用法：
// accrual-run -day 2024-01-15 -dayclose-id <id> -actor sys-accrual
// 1. 从指定参数创建应计批次（draft）；
// 2. 扫描指定营业日的在途 Item（从 stdin 读取 JSON）；
// 3. 为跨日在途 Item 追加应计分录对；
// 4. 过账批次并打印结果 JSON。
// 不调用 recon 分类器，不改三方对账公式。

// Author: MilesFord
// Date: 2021.01.20

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"account-assets.kucoin.net/funding/internal/accrual"
)

   func main() {
	   var day = flag.String("day", "", "营业日（YYYY-MM-DD）")
	  var dayCloseID = flag.String("dayclose-id", "", "关联的 day_close ID")
	   var actor = flag.String("actor", "sys-accrual", "操作 actor")
	  var note = flag.String("note", "manual-accrual-run", "批次备注")
	  flag.Parse()
	   if *day == "" {
		  fmt.Fprintln(os.Stderr, "error: -day required")
		   os.Exit(1)
	   }
	  engine := accrual.NewEngine()
	  // 创建批次
	   bat, err := engine.CreateBatch(*day, *dayCloseID, *actor, *note)
	   if err != nil {
		   fmt.Fprintf(os.Stderr, "create batch error: %v\n", err)
		  os.Exit(1)
	   }
	   fmt.Printf("batch created: %s (draft)\n", bat.ID)
	  // 从 stdin 读取分录对（JSON 数组）
	  var linePairs []struct {
		  Kind        string `json:"kind"`
		   DebitUserID string `json:"debit_user_id"`
		  CreditUserID string `json:"credit_user_id"`
		  Asset        string `json:"asset"`
		  Amount       string `json:"amount"`
		  MovementID   string `json:"movement_id"`
		   Note         string `json:"note"`
	   }
	  if err := json.NewDecoder(os.Stdin).Decode(&linePairs); err != nil && err.Error() != "EOF" {
		  fmt.Fprintf(os.Stderr, "read line pairs error: %v\n", err)
		   os.Exit(1)
	   }
	   for i, lp := range linePairs {
		   d, cr, err := engine.AddLinePair(
			   bat.ID, lp.Kind, lp.DebitUserID, lp.CreditUserID,
			   lp.Asset, lp.Amount, lp.MovementID, lp.Note,
		   )
		   if err != nil {
			  fmt.Fprintf(os.Stderr, "add line pair[%d] error: %v\n", i, err)
			  os.Exit(1)
		  }
		   fmt.Printf("  line pair[%d]: debit=%s credit=%s amount=%s\n", i, d.ID, cr.ID, lp.Amount)
	   }
	  // 刷新批次以获取最新围栏
	  bat, err = engine.GetBatch(bat.ID)
	   if err != nil {
		  fmt.Fprintf(os.Stderr, "get batch error: %v\n", err)
		  os.Exit(1)
	  }
	  // 过账
	  posted,
		   err := engine.Post(bat.ID, *actor, bat.Fence.Epoch, bat.Fence.Token)
	  if err != nil {
		   fmt.Fprintf(os.Stderr, "post batch error: %v\n", err)
		  os.Exit(1)
	  }
	  b, _ := json.MarshalIndent(posted, "", "  ")
	  fmt.Printf("accrual_batch (posted):\n%s\n", string(b))
  }
