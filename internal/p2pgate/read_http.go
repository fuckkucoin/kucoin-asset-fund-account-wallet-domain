package p2pgate

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
)

// RegisterOccupancyRead 挂到既有 /v1 组，与 RegisterFunds 并列。

   func RegisterOccupancyRead(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if v1 == nil || rt == nil {
		   return
	  }
	   funds := NewFunds(rt.Ledger,
		  holdcat.New(rt.KV))
	   opt := callerid.FromRuntime(rt.Mem, "", "", "")
	   if rt.Cfg != nil {
		  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	  bindOccupancyRead(v1, funds, opt)
   }

  func bindOccupancyRead(v1 *gin.RouterGroup, funds *Funds, opt callerid.Opt) {
	  read := func(c *gin.Context) (*Funds,
		  string, OccupancyQuery, bool) {
		   uid, err := callerid.Require(c, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return nil, "", OccupancyQuery{}, false
		  }
		  q, err := ParseOccupancyQuery(uid, c.Request.URL.Query())
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return nil, "", OccupancyQuery{}, false
		   }
		   return funds, uid, q, true
	   }
	  v1.GET("/p2p/details", func(c *gin.Context) {
		  f, uid, q, ok := read(c)
		  if !ok {
			  return
		   }
		   if wantsCSV(c.Query("format")) {
			  raw, err := f.ExportDetailsCSV(c.Request.Context(), q)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			   }
			   c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(raw))
			   return
		  }
		   page, err := f.ListDetails(c.Request.Context(), q)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "items": page.Items, "page": page.Page, "host": config.HostP2PFunding})
	  })
	   v1.GET("/p2p/holds/:id", func(c *gin.Context) {
		  f, uid, _, ok := read(c)
		   if !ok {
			   return
		   }
		  got, err := f.GetDetail(c.Request.Context(), uid, c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "detail": got, "host": config.HostP2PFunding})
	  })
	  v1.GET("/p2p/flow", func(c *gin.Context) {
		   f, uid, q, ok := read(c)
		   if !ok {
			   return
		   }
		   if wantsCSV(c.Query("format")) {
			   raw, err := f.ExportFlowCSV(c.Request.Context(), q)
			  if err != nil {
				  mw.WriteProblem(c, err)
				   return
			  }
			  c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(raw))
			   return
		   }
		  page, err := f.ListFlow(c.Request.Context(), q)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   net,
			  err := NetFlow(page.Items)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "items": page.Items, "page": page.Page, "net": net, "host": config.HostP2PFunding})
	  })
	   v1.GET("/p2p/merchant", func(c *gin.Context) {
		  f, uid, q, ok := read(c)
		  if !ok {
			   return
		  }
		  if wantsCSV(c.Query("format")) {
			   raw, err := f.ExportMerchantCSV(c.Request.Context(), q)
			  if err != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			  c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(raw))
			  return
		  }
		   page, err := f.ListMerchant(c.Request.Context(), q)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "items": page.Items, "page": page.Page, "host": config.HostP2PFunding})
	  })
	  v1.GET("/p2p/merchant/summary", func(c *gin.Context) {
		  f, uid, q, ok := read(c)
		  if !ok {
			  return
		  }
		  sum, err := f.MerchantOccupancy(c.Request.Context(), q)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"user_id": uid, "summary": sum, "host": config.HostP2PFunding})
	  })
	  v1.GET("/p2p/summary", func(c *gin.Context) {
		  f, uid, _, ok := read(c)
		   if !ok {
			  return
		   }
		  sum, err := f.OccupancySummaryOf(c.Request.Context(), uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   cool, _ := f.CoolingState(c.Request.Context(), uid)
		   c.JSON(200, gin.H{
			   "user_id": uid, "open_holds": sum.OpenHolds, "frozen_total": sum.FrozenTotal,
			  "by_asset": sum.ByAsset, "sell_open": sum.SellOpen, "sell_total": sum.SellTotal,
			  "merchant_open": sum.MerchantOpen, "merchant_total": sum.MerchantTotal,
			   "released_count": sum.ReleasedCount, "flow_count": sum.FlowCount,
			  "integrity_ok": sum.IntegrityOK, "cooling": cool, "host": config.HostP2PFunding,
		  })
	  })
	   v1.GET("/p2p/orders", func(c *gin.Context) {
		   f, uid, q, ok := read(c)
		   if !ok {
			  return
		   }
		  if id := strings.TrimSpace(c.Query("order_id")); id != "" && strings.TrimSpace(c.Query("id")) == "" {
			  sheet, err := f.GetOrderSheet(c.Request.Context(), uid, id)
			   if err != nil {
				   mw.WriteProblem(c, err)
				   return
			   }
			   c.JSON(200, gin.H{"user_id": uid, "order": sheet, "host": config.HostP2PFunding})
			   return
		   }
		   page,
			  err := f.ListOrders(c.Request.Context(), q)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "items": page.Items, "page": page.Page, "host": config.HostP2PFunding})
	   })
	  v1.GET("/p2p/orders/:id", func(c *gin.Context) {
		   f, uid, _, ok := read(c)
		  if !ok {
			  return
		  }
		  sheet, err := f.GetOrderSheet(c.Request.Context(), uid, c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "order": sheet, "host": config.HostP2PFunding})
	  })
	  v1.GET("/p2p/integrity", func(c *gin.Context) {
		   f, uid, _, ok := read(c)
		  if !ok {
			  return
		  }
		   rep, err := f.Integrity(c.Request.Context(), uid)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"user_id": uid, "report": rep, "host": config.HostP2PFunding})
	  })
	   v1.GET("/p2p/aggregate", func(c *gin.Context) {
		   f, uid, q, ok := read(c)
		  if !ok {
			  return
		   }
		  byAsset,
			  err := f.AggregateByAsset(c.Request.Context(), q)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  byReason, err := f.AggregateByReason(c.Request.Context(), uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "by_asset": byAsset, "by_reason": byReason, "host": config.HostP2PFunding})
	   })
	   v1.GET("/p2p/windows", func(c *gin.Context) {
		   f, uid, q, ok := read(c)
		   if !ok {
			  return
		  }
		  days,
			  err := f.DayWindows(c.Request.Context(), q)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "days": days, "host": config.HostP2PFunding})
	   })
	  v1.GET("/p2p/cooling", func(c *gin.Context) {
		  f, uid, _, ok := read(c)
		  if !ok {
			   return
		  }
		  cool, err := f.CoolingState(c.Request.Context(), uid)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"user_id": uid, "cooling": cool, "host": config.HostP2PFunding})
	  })
	  v1.GET("/p2p/catalog", func(c *gin.Context) {
		   c.JSON(200, OccupancyCatalog())
	  })
	   v1.GET("/p2p/snapshot", func(c *gin.Context) {
		  f, uid, _, ok := read(c)
		   if !ok {
			   return
		   }
		  snap, err := f.Snapshot(c.Request.Context(), uid)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "snapshot": snap, "host": config.HostP2PFunding})
	  })
  }
