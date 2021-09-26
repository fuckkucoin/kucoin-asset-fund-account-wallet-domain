package p2pgate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/notify"
	"github.com/gin-gonic/gin"
)

   func RegisterFunds(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if v1 == nil || rt == nil {
		   return
	   }
	  funds := NewFunds(rt.Ledger, holdcat.New(rt.KV))
	  opt := callerid.FromRuntime(rt.Mem, "", "", "")
	  if rt.Cfg != nil {
		   opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	   v1.POST("/p2p/buy", func(c *gin.Context) {
		   var body struct {
			  UserID string `json:"user_id"`
			  Asset string `json:"asset"`
			   Amount string `json:"amount"`
			   OrderID string `json:"order_id"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  tr, err := funds.Buy(c.Request.Context(), uid, body.Asset, body.Amount, body.OrderID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  _ = notify.FromConfig(rt.KV, rt.Cfg).Publish(c.Request.Context(), "p2p-buy:"+body.OrderID, notify.KindFiatCredited, body.OrderID)
		   c.JSON(200, gin.H{"transfer": tr, "host": config.HostP2PFunding})
	  })
	  v1.POST("/p2p/sell/hold", func(c *gin.Context) {
		  var body struct {
			   UserID  string `json:"user_id"`
			  Asset   string `json:"asset"`
			  Amount  string `json:"amount"`
			   OrderID string `json:"order_id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Bind(c, body.UserID, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  row,
			   err := funds.SellHold(c.Request.Context(), uid, body.Asset, body.Amount, body.OrderID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  _ = notify.FromConfig(rt.KV, rt.Cfg).Publish(c.Request.Context(), "p2p-sell:"+body.OrderID, notify.KindP2PHold, body.OrderID)
		   c.JSON(200, gin.H{"hold": row, "host": config.HostP2PFunding})
	   })
	  v1.POST("/p2p/margin/hold", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
			  Asset   string `json:"asset"`
			  Amount  string `json:"amount"`
			   OrderID string `json:"order_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  row, err := funds.MarginHold(c.Request.Context(), uid, body.Asset, body.Amount, body.OrderID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   _ = notify.FromConfig(rt.KV, rt.Cfg).Publish(c.Request.Context(), "p2p-margin:"+body.OrderID, notify.KindP2PHold, body.OrderID)
		   c.JSON(200, gin.H{"hold": row, "host": config.HostP2PFunding})
	  })
	   v1.POST("/p2p/hold/release", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
			   HoldID string `json:"hold_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid, err := callerid.Bind(c, body.UserID, opt)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   row, err := funds.Release(c.Request.Context(), uid, body.HoldID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"hold": row})
	  })
	   v1.GET("/p2p/holds", func(c *gin.Context) {
		   uid, err := callerid.Require(c, opt)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  // 仅回显 P2P 卖出/保证金占用（资金页冻结邻接）。
		   rows, err := funds.ListOpen(c.Request.Context(), uid)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"items": rows, "host": config.HostP2PFunding})
	  })
  }
