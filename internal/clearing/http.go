package clearing

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundsafety"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/portfolio"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
	"time"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  v1.POST("/clearing/batches", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  Day string `json:"day"`
			   Actor string `json:"actor"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   actor := perm.FromRequest(c.Request, "").ID
		   if actor == "" {
			  actor = body.Actor
		   }
		   bat, err := svc.Open(body.Day, actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"batch": bat, "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
	   v1.POST("/clearing/items", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  BatchID string `json:"batch_id"`
			  UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
			   MovementID string `json:"movement_id"`
			  Offset  int    `json:"offset"`
			   TradeAt string `json:"trade_at"`
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
		   trade := time.Now().UTC()
		   if body.TradeAt != "" {
			   t, err := time.Parse(time.RFC3339, body.TradeAt)
			  if err != nil {
				   mw.WriteProblem(c, ErrBatch)
				  return
			   }
			  trade = t
		  }
		  it, err := svc.Enqueue(body.BatchID, body.UserID, body.Asset, body.Amount, body.MovementID, body.Offset, trade)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, it)
	   })
	   v1.POST("/clearing/tick", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"ready": svc.Tick(time.Time{})})
	  })
	   v1.POST("/clearing/items/:id/release", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Epoch int64  `json:"epoch"`
			  Token string `json:"token"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  actor := perm.FromRequest(c.Request, "").ID
		  if _, err := validate.Actor(actor); err != nil {
			   mw.WriteProblem(c, ErrActor)
			   return
		  }
		  it, err := svc.Release(c.Param("id"), actor, body.Epoch, body.Token)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, it)
	   })
	   v1.POST("/clearing/items/:id/park", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  Reason string `json:"reason"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   ex, err := svc.Park(c.Param("id"), perm.FromRequest(c.Request, "").ID, body.Reason)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, ex)
	  })
	   v1.POST("/clearing/batches/:id/close", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   Epoch int64 `json:"epoch"`
			  Token string `json:"token"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   bat, err := svc.Close(c.Param("id"), perm.FromRequest(c.Request, "").ID, body.Epoch, body.Token)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, bat)
	  })
	   v1.POST("/clearing/items/:id/apply", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   Epoch int64 `json:"epoch"`
			   Token  string `json:"token"`
			  UserID string `json:"user_id"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  actor := perm.FromRequest(c.Request, "").ID
		   if _, err := validate.Actor(actor); err != nil {
			   mw.WriteProblem(c, ErrActor)
			  return
		  }
		  led := portfolio.NewMem().Ledger()
		  pos, err := svc.Apply(led, c.Param("id"), actor, body.Epoch, body.Token)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"position": pos, "applied": svc.Book().Applied(c.Param("id"))})
	   })
	  v1.GET("/clearing/exceptions", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"exceptions": svc.Book().OpenExceptions()})
	  })
	  v1.GET("/clearing/holidays", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"days": svc.Book().Holidays().List(), "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
	  v1.POST("/clearing/holidays", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Day string `json:"day"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  if err := svc.Book().Holidays().Add(body.Day); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"days": svc.Book().Holidays().List()})
	   })
	   registerSafety(v1, rt, svc)
   }

  func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   gate := fundsafety.New()
	  _ = rt
	  v1.POST("/clearing/partial", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
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
		  c.JSON(200, gin.H{"ok": true, "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
	   v1.GET("/clearing/open", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"exceptions": svc.Book().OpenExceptions(), "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	   })
  }
