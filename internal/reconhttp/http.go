package reconhttp

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/internal/subacct"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/xferloop"
	"github.com/gin-gonic/gin"
)

type snapCap struct{ svc *snap.Service }

   func (s snapCap) Shot(ctx context.Context, userID string) (recon.Shot, error) {
	   if s.svc == nil {
		  return recon.Shot{UserID: userID},
			   nil
	   }
	   got, err := s.svc.Capture(ctx, userID)
	  if err != nil {
		   return recon.Shot{}, err
	   }
	  out := recon.Shot{UserID: got.UserID, Positions: make([]recon.ShotPos, 0, len(got.Positions))}
	   for _, p := range got.Positions {
		   out.Positions = append(out.Positions, recon.ShotPos{
			  Book: p.Book, Asset: p.Asset, Available: p.Available, Frozen: p.Frozen, Total: p.Total,
		   })
	  }
	  return out, nil
   }

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  kcfund.BindFundsRecon(rt)
	   p := recon.NewPlatformWith(rt.Ledger,
		  snapCap{svc: snap.New(rt.Ledger, rt.Store)})
	  if rt.FundSurf != nil {
		  p.Halt = custodyhalt.EngageOnly{Gate: rt.FundSurf.HaltGate(), Hist: custodyhalt.SharedHistory()}
	   }
	  v1.POST("/recon/run", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
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
		   uid, err = validate.UserID(body.UserID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   if _, err := validate.Actor(actor.ID); err != nil {
			   mw.WriteProblem(c, recon.ErrActor)
			   return
		   }
		  rep, err := p.Run(c.Request.Context(), actor.ID, uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"report": rep, "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
	  v1.GET("/recon/tickets", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   uid := c.Query("user_id")
		   c.JSON(200, gin.H{"tickets": p.Tickets().List(uid, validate.Limit(0, 50, 200))})
	   })
	  v1.POST("/recon/hops/recover", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   svc := subacct.NewWithKV(rt.KV)
		   if rt != nil && !rt.Mem && rt.SQL != nil {
			   svc.BindSQL(rt.SQL)
		  }
		  reps, err := kcfund.RecoverOpenHops(c.Request.Context(), rt.Ledger, svc, 200)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"recovered": reps, "host": "ledger-transfer-freeze-reconciliation.kucoin.net"})
	  })
	  v1.GET("/recon/xfer-loop/board", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   b := xferloop.Live()
		   if b == nil {
			   c.JSON(200, gin.H{"board": xferloop.Board{}, "policies": xferloop.Policies(), "status": xferloop.StatusCatalog()})
			   return
		  }
		  c.JSON(200, gin.H{"board": b.Board(), "policies": xferloop.Policies(), "status": xferloop.StatusCatalog()})
	   })
	   v1.GET("/recon/xfer-loop", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  b := xferloop.Live()
		  if b == nil {
			  c.JSON(200, gin.H{"cases": []xferloop.Case{}})
			  return
		   }
		   var breached *bool
		  switch c.Query("breached") {
		  case "1", "true":
			   on := true
			  breached = &on
		  case "0", "false":
			   off := false
			  breached = &off
		   }
		   c.JSON(200, gin.H{"cases": b.ListQuery(xferloop.Query{
			   Kind: xferloop.Kind(c.Query("kind")), State: xferloop.State(c.Query("state")),
			  Asset: c.Query("asset"), UserID: c.Query("user_id"), Breached: breached, Limit: 50,
		  })})
	  })
	   v1.POST("/recon/xfer-loop/:id/assign", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  b := xferloop.Live()
		   if b == nil {
			  mw.WriteProblem(c, xferloop.ErrNotFound)
			  return
		   }
		   var body struct {
			   Actor string `json:"actor"`
		   }
		  if c.Request.ContentLength > 0 && !mw.BindJSON(c, &body) {
			   return
		   }
		  who := body.Actor
		  if who == "" {
			   who = actor.ID
		   }
		  got, err := b.Assign(c.Request.Context(), c.Param("id"), who)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/recon/xfer-loop/:id/retry", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   b := xferloop.Live()
		   if b == nil {
			  mw.WriteProblem(c, xferloop.ErrNotFound)
			   return
		  }
		   got, err := b.Retry(c.Request.Context(), c.Param("id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	  v1.POST("/recon/triparty", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   Observe []hotinv.Snapshot `json:"observe"`
		   }
		  if c.Request.ContentLength > 0 {
			   if !mw.BindJSON(c, &body) {
				   return
			   }
		   }
		  if _, err := validate.Actor(actor.ID); err != nil {
			  mw.WriteProblem(c, recon.ErrActor)
			   return
		   }
		  hot := hotinv.New(nil)
		  if rt.Cfg != nil {
			  hot = hotinv.FromHotMax(rt.Cfg.Transfer.HotMax)
		   }
		  rep, err := recon.RunTriparty(c.Request.Context(), rt.Ledger, rt.Store, rt.Durable.Recon, actor.ID, hot, body.Observe)
		   if err != nil && len(rep.Drifts) == 0 {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, rep)
	   })
	   v1.GET("/recon/triparty/classes", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, gin.H{"classes": recon.AssetClasses()})
	  })
	  v1.GET("/recon/triparty", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, recon.LastTriparty())
	   })
   }
