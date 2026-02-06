package warroom

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New()
	  if rt != nil {
		  svc.BindLedger(rt.Ledger)
	  }
	  v1.POST("/warroom/freeze-all", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body FreezeReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.FreezeAll(c.Request.Context(), body.Actor, body.Reason)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	  v1.POST("/warroom/freeze-user", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body FreezeReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := svc.FreezeUser(c.Request.Context(), body); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"frozen": true, "user_id": body.UserID})
	   })
	   v1.POST("/warroom/unfreeze-open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body TicketReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  got,
			  err := svc.OpenUnfreeze(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	   v1.POST("/warroom/unfreeze-approve", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   ID string `json:"id"`
			  Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.Approve(body.ID, body.Actor)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	   registerMore(v1, svc)
   }

  func registerMore(v1 *gin.RouterGroup, svc *Service) {
	   v1.POST("/warroom/unfreeze-apply", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  ID    string `json:"id"`
			   Actor string `json:"actor"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.Apply(c.Request.Context(), body.ID, body.Actor)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	  v1.POST("/warroom/unfreeze-deny", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   ID string `json:"id"`
			   Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.Deny(body.ID, body.Actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	   v1.GET("/warroom/switch", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"switch": svc.Current(), "frozen": svc.FrozenUsers(), "host": "account-book.kucoin.net"})
	   })
	   v1.GET("/warroom/tickets", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"tickets": svc.Tickets(c.Query("status"))})
	  })
	   v1.GET("/warroom/timeline", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"events": svc.Timeline(200)})
	   })
	   v1.GET("/warroom/export", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  if c.Query("fmt") == "csv" {
			   txt, err := svc.ExportCSV()
			  if err != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			  c.String(200, txt)
			   return
		   }
		   ex, err := svc.Export()
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, ex)
	   })
   }
