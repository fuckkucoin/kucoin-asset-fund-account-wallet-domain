// HTTP 面：对外路由与请求编排。

// Author: Cason
// Date: 2021.09.11

package oracle

import (
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := NewMock(time.Now().UTC())
	  _ = rt
	  v1.GET("/oracle/ticks", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"ticks": svc.Feed().List(), "host": "query-gateway.kucoin.net"})
	   })
	   v1.POST("/oracle/ticks", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  Asset string `json:"asset"`
			  Quote string `json:"quote"`
			  Price string `json:"price"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   t, err := svc.Put(body.Asset, body.Quote, body.Price, time.Now().UTC())
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, t)
	  })
	   v1.POST("/oracle/convert", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Asset string `json:"asset"`
			   Amount string `json:"amount"`
			  Quote string `json:"quote"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   if body.Quote == "" {
			   body.Quote = "USDT"
		   }
		  val, tick, err := svc.Feed().Convert(body.Asset, body.Amount, body.Quote)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"value": val.String(), "tick": tick})
	   })
	  v1.POST("/oracle/mtm", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Quote string `json:"quote"`
			   Amounts map[string]string `json:"amounts"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.Quote == "" {
			  body.Quote = "USDT"
		  }
		   mark, err := svc.Feed().MarkToMarket(body.Quote, body.Amounts)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, mark)
	  })
	   v1.POST("/oracle/margin-ltv", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   BorrowAsset string `json:"borrow_asset"`
			  Borrow string `json:"borrow"`
			  ColAsset string `json:"collateral_asset"`
			   Collateral string `json:"collateral"`
			   Quote  string `json:"quote"`
			  CapBPS int    `json:"cap_bps"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   if body.Quote == "" {
			   body.Quote = "USDT"
		   }
		   bps, err := svc.Feed().MarginLTV(body.BorrowAsset, body.Borrow, body.ColAsset, body.Collateral, body.Quote)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  err = svc.Feed().CheckMarginLTV(body.BorrowAsset, body.Borrow, body.ColAsset, body.Collateral, body.Quote, body.CapBPS)
		  c.JSON(200, gin.H{"ltv_bps": bps, "ok": err == nil, "error": errString(err)})
	   })
	   v1.GET("/oracle/invert", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  t, err := svc.Feed().Invert(c.Query("asset"), c.Query("quote"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, t)
	  })
  }

  func errString(err error) string {
	   if err == nil {
		  return ""
	  }
	  return err.Error()
   }
// temporary guard removed after the production check
