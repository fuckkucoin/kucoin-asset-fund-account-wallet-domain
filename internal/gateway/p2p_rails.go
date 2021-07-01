// 写路径仍走 p2pgate Funds；本文件只聚合 query-gateway 只读路由。

// Author: Axel
// Date: 2021.06.29

package gateway

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/p2pgate"
	"github.com/gin-gonic/gin"
)

  func registerP2PRails(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if rt == nil || rt.Ledger == nil {
		   return
	  }
	   funds := p2pgate.NewFunds(rt.Ledger, holdcat.New(rt.KV))
	  opt := callerid.FromRuntime(rt.Mem, "", "", "")
	  if rt.Cfg != nil {
		   opt = callerid.FromRuntime(rt.Mem,
			   rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	  bindP2PAssetReads(v1, funds, opt)
   }

   func bindP2PAssetReads(v1 *gin.RouterGroup, funds *p2pgate.Funds, opt callerid.Opt) {
	   mustUID := func(c *gin.Context) (string, bool) {
		  uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return "", false
		  }
		   return uid, true
	  }
	  parseQ := func(c *gin.Context, uid string) (p2pgate.OccupancyQuery, bool) {
		  q, err := p2pgate.ParseOccupancyQuery(uid, c.Request.URL.Query())
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return p2pgate.OccupancyQuery{}, false
		   }
		  return q, true
	  }
	  // 既有开放占用列表：items 仍为 holdcat.Row。
	  v1.GET("/assets/p2p/holds", func(c *gin.Context) {
		  uid, ok := mustUID(c)
		   if !ok {
			   return
		  }
		  rows, err := funds.ListOpen(c.Request.Context(), uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "items": rows, "host": config.HostP2PFunding})
	   })
	  // 既有摘要字段 + 卖出/商家拆分。
	  v1.GET("/assets/p2p/summary", func(c *gin.Context) {
		  uid, ok := mustUID(c)
		   if !ok {
			   return
		  }
		   sum, err := funds.OccupancySummaryOf(c.Request.Context(), uid)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{
			  "user_id": uid, "open_holds": sum.OpenHolds, "frozen_total": sum.FrozenTotal,
			  "by_asset": sum.ByAsset, "sell_open": sum.SellOpen, "sell_total": sum.SellTotal,
			  "merchant_open": sum.MerchantOpen, "merchant_total": sum.MerchantTotal,
			  "released_count": sum.ReleasedCount, "flow_count": sum.FlowCount,
			  "integrity_ok": sum.IntegrityOK, "host": config.HostP2PFunding,
		   })
	   })
	  v1.GET("/assets/p2p/details", func(c *gin.Context) {
		  uid, ok := mustUID(c)
		   if !ok {
			  return
		  }
		  q, okQ := parseQ(c, uid)
		  if !okQ {
			   return
		  }
		  if strings.EqualFold(c.Query("format"), "csv") {
			   raw, err := funds.ExportDetailsCSV(c.Request.Context(), q)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			  c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(raw))
			  return
		   }
		  page, err := funds.ListDetails(c.Request.Context(), q)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "items": page.Items, "page": page.Page, "host": config.HostP2PFunding})
	  })
	  v1.GET("/assets/p2p/holds/:id", func(c *gin.Context) {
		   uid, ok := mustUID(c)
		   if !ok {
			   return
		   }
		   got, err := funds.GetDetail(c.Request.Context(), uid, c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "detail": got, "host": config.HostP2PFunding})
	  })
	  v1.GET("/assets/p2p/flow", func(c *gin.Context) {
		   uid, ok := mustUID(c)
		  if !ok {
			  return
		   }
		   q, okQ := parseQ(c, uid)
		   if !okQ {
			   return
		  }
		   if strings.EqualFold(c.Query("format"), "csv") {
			   raw, err := funds.ExportFlowCSV(c.Request.Context(), q)
			  if err != nil {
				  mw.WriteProblem(c, err)
				   return
			   }
			  c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(raw))
			  return
		  }
		   page, err := funds.ListFlow(c.Request.Context(), q)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   net, err := p2pgate.NetFlow(page.Items)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{
			   "user_id": uid, "items": page.Items, "page": page.Page,
			   "net": net, "host": config.HostP2PFunding,
		  })
	  })
	  v1.GET("/assets/p2p/merchant", func(c *gin.Context) {
		   uid, ok := mustUID(c)
		  if !ok {
			   return
		   }
		   q, okQ := parseQ(c, uid)
		  if !okQ {
			   return
		  }
		   page, err := funds.ListMerchant(c.Request.Context(), q)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "items": page.Items, "page": page.Page, "host": config.HostP2PFunding})
	   })
	  v1.GET("/assets/p2p/merchant/summary", func(c *gin.Context) {
		  uid, ok := mustUID(c)
		  if !ok {
			  return
		   }
		  q, okQ := parseQ(c, uid)
		   if !okQ {
			  return
		   }
		   sum, err := funds.MerchantOccupancy(c.Request.Context(), q)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "summary": sum, "host": config.HostP2PFunding})
	  })
	   v1.GET("/assets/p2p/orders", func(c *gin.Context) {
		   uid, ok := mustUID(c)
		  if !ok {
			  return
		  }
		  q, okQ := parseQ(c, uid)
		  if !okQ {
			  return
		   }
		   page,
			  err := funds.ListOrders(c.Request.Context(), q)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"user_id": uid, "items": page.Items, "page": page.Page, "host": config.HostP2PFunding})
	   })
	  v1.GET("/assets/p2p/orders/:id", func(c *gin.Context) {
		  uid, ok := mustUID(c)
		   if !ok {
			  return
		  }
		   sheet, err := funds.GetOrderSheet(c.Request.Context(), uid, c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "order": sheet, "host": config.HostP2PFunding})
	  })
	   v1.GET("/assets/p2p/integrity", func(c *gin.Context) {
		   uid, ok := mustUID(c)
		  if !ok {
			  return
		   }
		  rep,
			  err := funds.Integrity(c.Request.Context(), uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "report": rep, "host": config.HostP2PFunding})
	  })
	  v1.GET("/assets/p2p/aggregate", func(c *gin.Context) {
		  uid, ok := mustUID(c)
		   if !ok {
			   return
		   }
		   q, okQ := parseQ(c, uid)
		  if !okQ {
			   return
		  }
		  byAsset, err := funds.AggregateByAsset(c.Request.Context(), q)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  byReason, err := funds.AggregateByReason(c.Request.Context(), uid)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "by_asset": byAsset, "by_reason": byReason, "host": config.HostP2PFunding})
	   })
	  v1.GET("/assets/p2p/windows", func(c *gin.Context) {
		   uid, ok := mustUID(c)
		   if !ok {
			   return
		  }
		   q, okQ := parseQ(c, uid)
		  if !okQ {
			  return
		  }
		   days, err := funds.DayWindows(c.Request.Context(), q)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "days": days, "host": config.HostP2PFunding})
	   })
	  v1.GET("/assets/p2p/cooling", func(c *gin.Context) {
		   uid, ok := mustUID(c)
		  if !ok {
			   return
		  }
		   cool, err := funds.CoolingState(c.Request.Context(), uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "cooling": cool, "host": config.HostP2PFunding})
	  })
	  v1.GET("/assets/p2p/catalog", func(c *gin.Context) {
		   c.JSON(200, p2pgate.OccupancyCatalog())
	   })
	   v1.GET("/assets/p2p/snapshot", func(c *gin.Context) {
		  uid, ok := mustUID(c)
		  if !ok {
			   return
		  }
		  snap, err := funds.Snapshot(c.Request.Context(), uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"user_id": uid, "snapshot": snap, "host": config.HostP2PFunding})
	  })
   }
