package depclaim

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  _ = rt
	   v1.POST("/depclaim/bind", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   UserID string `json:"user_id"`
			   Chain  string `json:"chain"`
			  Kind   string `json:"kind"`
			   Key    string `json:"key"`
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
		   kind,
			   err := deposit.ParseAttribKind(body.Kind)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   got, err := svc.Attributor().BindUnique(body.UserID, body.Chain, kind, body.Key)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/depclaim/claim", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body ClaimReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  got, err := svc.Claim(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	  v1.GET("/depclaim/recent", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   opt := callerid.Opt{}
		  if rt != nil && rt.Cfg != nil {
			  opt = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
		   }
		  uid, err := callerid.Require(c, opt)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  q := strings.TrimSpace(c.Query("user_id"))
		  if q == "" {
			   q = uid
		   }
		  if q != uid {
			  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
				   c.JSON(403, gin.H{"error": "cross-user deposit claim read refused"})
				   return
			  }
		  }
		  body := gin.H{"claims": svc.List(q, 50), "host": "deposit-withdraw.kucoin.net"}
		   if perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin) == nil {
			  body["recovery"] = svc.Recoveries()
		   }
		   c.JSON(200, body)
	   })
   }
