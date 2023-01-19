package vipfee

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		  svc.BindSQL(rt.SQL)
	  }
	  v1.GET("/vipfee/schedules", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"schedules": svc.List(), "host": "query-gateway.kucoin.net"})
	   })
	   v1.POST("/vipfee/schedules", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body Schedule
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if err := svc.Put(body); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, body)
	  })
	   v1.POST("/vipfee/resolve", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body ResolveReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.Resolve(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	  v1.POST("/vipfee/volume", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body VolumeReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  got,
			  err := svc.RecordVolume(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   registerMore(v1, svc)
   }

   func registerMore(v1 *gin.RouterGroup, svc *Service) {
	   v1.POST("/vipfee/pin", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body PinReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Pin(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.POST("/vipfee/pin-clear", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  UserID string `json:"user_id"`
			  Actor  string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := svc.ClearPin(body.UserID, body.Actor); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"cleared": true})
	  })
	  v1.POST("/vipfee/withdraw-quote", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   UserID string `json:"user_id"`
			   Asset string `json:"asset"`
			   Gross string `json:"gross"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.QuoteWithdraw(body.UserID, body.Asset, body.Gross)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	  v1.GET("/vipfee/volume", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   sum, err := svc.WindowOf(c.Query("user_id"), c.Query("asset"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"volume": sum, "fills": svc.Fills(c.Query("user_id"), 50)})
	  })
   }
