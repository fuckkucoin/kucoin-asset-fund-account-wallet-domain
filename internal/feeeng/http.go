package feeeng

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		  svc.BindSQL(rt.SQL)
	   }
	  v1.GET("/fees/schedules", func(c *gin.Context) {
		   c.JSON(200, gin.H{"schedules": svc.List(), "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
	  v1.POST("/fees/quote", func(c *gin.Context) {
		  var body struct {
			   Kind  string `json:"kind"`
			   Asset string `json:"asset"`
			  Amount string `json:"amount"`
			  Volume string `json:"volume"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   q, err := svc.Quote(QuoteReq{Kind: body.Kind, Asset: body.Asset, Gross: body.Amount, Volume: body.Volume})
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, q)
	  })
	  v1.POST("/fees/schedules", func(c *gin.Context) {
		   var body Schedule
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   if body.Asset != "" {
			  if as, err := validate.Asset(body.Asset); err != nil {
				   mw.WriteProblem(c, err)
				  return
			  } else {
				   body.Asset = as
			  }
		  }
		   body.Active = true
		   if err := svc.Put(body); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, body)
	  })
  }
