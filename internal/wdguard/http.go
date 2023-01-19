package wdguard

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		   if b := svc.Book(); b != nil {
			  b.BindSQL(rt.SQL)
		   }
	   }
	  RegisterService(v1, svc)
   }

   func RegisterService(v1 *gin.RouterGroup, svc *Service) {
	   if svc == nil {
		   svc = New()
	  }
	   v1.POST("/wdguard/preflight", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body Check
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   if body.Actor == "" {
			  body.Actor = perm.FromRequest(c.Request, "").ID
		  }
		  got, err := svc.Preflight(c.Request.Context(), body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	  v1.POST("/wdguard/halt", func(c *gin.Context) {
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
	   v1.GET("/wdguard/recent", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"recent": svc.Recent(50), "host": "deposit-withdraw.kucoin.net"})
	  })
   }
