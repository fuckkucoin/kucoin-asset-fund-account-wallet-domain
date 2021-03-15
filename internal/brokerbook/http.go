package brokerbook

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/perm"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   _ = rt
	  v1.POST("/broker/master", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body OpenMasterReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.OpenMaster(body)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	  v1.POST("/broker/sub", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body OpenSubReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.OpenSub(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	  v1.POST("/broker/alloc", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body AllocReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.Actor == "" {
			   body.Actor = perm.FromRequest(c.Request, "ops-broker").ID
		  }
		   got, err := svc.Alloc(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/broker/fence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  SubID string `json:"sub_id"`
			   Actor string `json:"actor"`
			  Privs []Priv `json:"privileges"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Fence(body.SubID, body.Actor, body.Privs)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	  v1.GET("/broker/subs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"subs": svc.ListSubs(c.Query("master_id")), "host": "account-book.kucoin.net"})
	   })
	   registerMore(v1, svc)
	   v1.GET("/broker/rollup", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   got, err := svc.Rollup(c.Query("master_id"), c.Query("asset"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
   }
