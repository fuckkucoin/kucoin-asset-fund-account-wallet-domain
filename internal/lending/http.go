// @FileName: http.go
// @Summary: lending 包内 http 实现。
// @Author: Kael
// @Created: 2021.08.22

package lending

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
)

// Register 挂放贷资金占用路由。宿主为 fund-surface；撮合面不在本仓。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   if v1 == nil || rt == nil {
		   return
	   }
	  svc := New(rt.Ledger, holdcat.New(rt.KV))
	   opt := callerid.FromRuntime(rt.Mem, "", "", "")
	  if rt.Cfg != nil {
		   opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
	  }
	  v1.POST("/lending/place", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
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
		  row, err := svc.Place(c.Request.Context(), uid, body.Asset, body.Amount, body.OrderID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"hold": row, "host": config.HostFundSurface})
	   })
	   v1.POST("/lending/settle", func(c *gin.Context) {
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
		   row, err := svc.Settle(c.Request.Context(), uid, body.Asset, body.Amount, body.OrderID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"hold": row, "host": config.HostFundSurface})
	   })
  }
