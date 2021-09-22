package obsdash

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	   if rt != nil {
		  svc.Bind(rt.Ledger, rt.Store, rt.Cfg.SeedUserID)
	   }
	   v1.POST("/obs/inc", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Name  string `json:"name"`
			  Label string `json:"label"`
			   N int64 `json:"n"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if err := svc.Counters().Inc(body.Name, body.Label, body.N); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true})
	   })
	  v1.POST("/obs/slo", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   Name string `json:"name"`
			   Failed bool `json:"failed"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := svc.Burns().Observe(body.Name, body.Failed); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  got, err := svc.Burns().Alert(body.Name)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	  v1.GET("/obs/dash", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  got, err := svc.Snapshot(c.Request.Context())
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"dash": got, "host": "query-gateway.kucoin.net"})
	   })
	   v1.GET("/obs/slo", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"burns": svc.Burns().List(), "documented": Documented()})
	   })
  }
