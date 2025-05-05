package webhook

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   d := New(nil)
	   _ = rt
	  v1.GET("/webhooks/kinds", func(c *gin.Context) {
		   c.JSON(200, gin.H{"kinds": AllKinds(), "host": "deposit-withdraw.kucoin.net"})
	   })
	   v1.POST("/webhooks/endpoints", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body Endpoint
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   if err := d.Registry().Put(body); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"ok": true, "id": body.ID})
	   })
	   v1.GET("/webhooks/endpoints", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"endpoints": d.Registry().List()})
	  })
	  v1.POST("/webhooks/dispatch", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Kind string `json:"kind"`
			  Body string `json:"body"`
			   Nonce string `json:"nonce"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   rows,
			   err := d.Dispatch(c.Request.Context(), body.Kind, body.Body, body.Nonce)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"deliveries": rows, "dead": d.Dead().Len()})
	   })
	   v1.GET("/webhooks/dlq", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"dead": d.Dead().List()})
	  })
	  v1.POST("/webhooks/dlq/replay", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  Index int `json:"index"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  del, err := d.Replay(c.Request.Context(), body.Index)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"delivery": del, "dead": d.Dead().Len()})
	   })
   }
