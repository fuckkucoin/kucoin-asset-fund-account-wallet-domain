// HTTP 面：对外路由与请求编排。

// Author: Cason
// Date: 2021.08.10

package leddiff

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil {
		  svc.BindLedger(rt.Ledger)
	  }
	   v1.POST("/leddiff/capture", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   UserID string `json:"user_id"`
			   Label string `json:"label"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   body.UserID = uid
		  got, err := svc.Capture(c.Request.Context(), body.UserID, body.Label)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/leddiff/put", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Positions []Position `json:"positions"`
			   Label     string     `json:"label"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.Put(body.Positions, body.Label)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	  v1.POST("/leddiff/diff", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   LeftID  string `json:"left_id"`
			   RightID string `json:"right_id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.Diff(body.LeftID, body.RightID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"report": got, "host": "query-gateway.kucoin.net"})
	   })
	  registerMore(v1, svc)
   }

  func registerMore(v1 *gin.RouterGroup, svc *Service) {
	   v1.GET("/leddiff/list", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"snaps": svc.List(50), "reports": svc.ListReports(50)})
	   })
	   v1.GET("/leddiff/snap/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  got, err := svc.Get(c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	   v1.GET("/leddiff/report/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  got, err := svc.ReportOf(c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  v1.POST("/leddiff/capture-many", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Users []string `json:"users"`
			  Label string `json:"label"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.CaptureMany(c.Request.Context(), body.Users, body.Label)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"snaps": got})
	  })
  }
