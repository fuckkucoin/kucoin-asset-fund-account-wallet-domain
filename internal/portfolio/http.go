package portfolio

import (
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundsafety"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New(account.New(rt.Ledger))
	   v1.GET("/portfolio", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   uid, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  if _, err := svc.LoadFromLedger(c.Request.Context(), uid); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  snap, err := svc.Snapshot(uid)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"portfolio": snap, "hops": LegalHops(), "host": "account-book.kucoin.net"})
	  })
	  v1.POST("/portfolio/hop", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  UserID string `json:"user_id"`
			   From string `json:"from"`
			  To   string `json:"to"`
			  Asset string `json:"asset"`
			   Amount string `json:"amount"`
			   Idem   string `json:"idempotency_key"`
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
		  from, err := ParseBook(body.From)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   to, err := ParseBook(body.To)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  if _, err := svc.LoadFromLedger(c.Request.Context(), body.UserID); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  if body.Idem != "" && svc.acct != nil {
			   tr, err := svc.MoveOnLedger(c.Request.Context(), body.UserID, from, to, body.Asset, body.Amount, body.Idem)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(200, gin.H{"transfer": ledgerView(tr)})
			   return
		  }
		  got, err := svc.Hop(body.UserID, from, to, body.Asset, body.Amount)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	  v1.POST("/portfolio/plan", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   UserID  string   `json:"user_id"`
			   Asset   string   `json:"asset"`
			   Targets []Target `json:"targets"`
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
		  if _, err := svc.LoadFromLedger(c.Request.Context(), body.UserID); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  steps, err := svc.Plan(body.UserID, body.Asset, body.Targets)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"steps": steps})
	  })
	   v1.POST("/portfolio/batch-hop", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   UserID string      `json:"user_id"`
			   Items  []BatchItem `json:"items"`
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
		   if _, err := svc.LoadFromLedger(c.Request.Context(), body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  got, err := svc.BatchHop(body.UserID, body.Items)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	  v1.GET("/portfolio/journal", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  uid, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"journal": svc.Ledger().Journal(uid), "host": "account-book.kucoin.net"})
	  })
	   registerSafety(v1, rt, svc)
  }

   func ledgerView(tr interface{}) interface{} { return tr }

  func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  gate := fundsafety.New()
	  if rt != nil {
		  gate.BindLedger(rt.Ledger)
	  }
	  v1.POST("/portfolio/refuse-frozen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   UserID string `json:"user_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   uid,
			  err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  body.UserID = uid
		   if err := gate.RefuseFrozen(c.Request.Context(), body.UserID); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"ok": true, "host": "account-book.kucoin.net"})
	   })
	  v1.POST("/portfolio/partial", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body fundsafety.Partial
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := gate.CheckPartial(body); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true})
	   })
	   _ = svc
   }
