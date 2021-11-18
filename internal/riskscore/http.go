package riskscore

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   _ = rt
	  v1.POST("/riskscore/record", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body Event
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.Record(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	  })
	  v1.POST("/riskscore/eval", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body EvalReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.Eval(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/riskscore/mode", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Mode string `json:"mode"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  svc.SetMode(ParseMode(body.Mode))
		   c.JSON(200, gin.H{"mode": svc.Mode()})
	  })
	  v1.POST("/riskscore/halt", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  On bool `json:"on"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  svc.SetHalt(body.On)
		   c.JSON(200, gin.H{"halt": svc.Halted()})
	  })
	   v1.GET("/riskscore/report", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"report": svc.Report(), "host": "kyt.kucoin.net"})
	   })
	   v1.GET("/riskscore/window", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, svc.WindowOf(c.Query("user_id"), c.Query("chain"), c.Query("dest"), svc.now()))
	  })
   }
