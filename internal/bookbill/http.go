package bookbill

import (
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/stmt"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New(nil)
	   if rt != nil {
		   svc = New(rt.Ledger)
	   }
	  v1.GET("/bookbill/statement", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  user := c.DefaultQuery("user_id", "")
		  if user == "" && rt != nil {
			  if u, err := callerid.Bind(c, user, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)); err != nil {
				   mw.WriteProblem(c, err)
				   return
			  } else {
				  user = u
			  }
		  }
		  if _, err := validate.UserID(user); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  got, err := svc.Build(c.Request.Context(), user, c.Query("asset"), 50)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.POST("/bookbill/period", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body BuildReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  body.UserID = uid
		  kind, err := stmt.ParsePeriodKind(body.Kind)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  at := time.Now().UTC()
		  if body.At != "" {
			  parsed, err := time.Parse(time.RFC3339, body.At)
			   if err != nil {
				   mw.WriteProblem(c, ErrPeriod)
				   return
			  }
			   at = parsed
		  }
		  got,
			   err := svc.BuildPeriod(c.Request.Context(), body.UserID, kind, at, body.Asset)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
  }
// temporary guard removed after the production check
