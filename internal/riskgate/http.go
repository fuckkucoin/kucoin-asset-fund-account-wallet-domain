// 功能：riskgate/http.go
// 作者：Dax
// 日期：2021.10.24

package riskgate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/policypack"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  _ = rt
	   v1.POST("/riskgate/screen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body ScreenReq
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := svc.Screen(body)
		  if err != nil && err != ErrDeny {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	   v1.POST("/riskgate/mode", func(c *gin.Context) {
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
		  svc.BindEngine(policypack.NewEngine(policypack.NewCatalog(),
			  policypack.ParseMode(body.Mode), true))
		  c.JSON(200, gin.H{"mode": string(svc.Engine().Mode())})
	   })
	  v1.GET("/riskgate/windows", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"windows": svc.Windows(), "host": "kyt.kucoin.net"})
	  })
	  v1.POST("/riskgate/halt", func(c *gin.Context) {
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
	  v1.POST("/riskgate/replay", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActKYTScreen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Pack  string       `json:"pack"`
			  Cases []ReplayCase `json:"cases"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.Replay(body.Pack, body.Cases)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
  }
