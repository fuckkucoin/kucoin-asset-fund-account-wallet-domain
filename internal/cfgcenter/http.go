package cfgcenter

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  st := NewStore()
	  if rt != nil && !rt.Mem && rt.SQL != nil {
		   st.BindSQL(rt.SQL)
	  }
	  v1.GET("/config/packs", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"names": st.Names(), "killed": st.Killed(), "host": "iam-gateway.kucoin.net"})
	   })
	   v1.GET("/config/packs/:name", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  app, err := st.Applied(c.Param("name"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"applied": app, "history": st.History(c.Param("name"))})
	   })
	  v1.POST("/config/packs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body LimitPack
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := st.Put(body); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, body)
	   })
	  v1.POST("/config/packs/:name/pin", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Version int `json:"version"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := st.Pin(c.Param("name"), body.Version); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"ok": true})
	   })
	   v1.POST("/config/kill", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  Name string `json:"name"`
			   Resume bool `json:"resume"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  if body.Resume {
			  st.Resume(body.Name)
		  } else {
			  st.Kill(body.Name)
		   }
		   c.JSON(200, gin.H{"killed": st.Killed()})
	  })
	   v1.POST("/config/eval", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body Request
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   dec, err := st.Eval(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, dec)
	  })
  }
